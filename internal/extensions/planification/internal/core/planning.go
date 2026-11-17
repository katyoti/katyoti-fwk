// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Planning implements api.PlanningService over the Object API. A release is
// the plan: its steps are its children, in its planning space; the requests
// targeting it and its steps are its network, ordered by `depends-on`. How
// many runs run at once is the task groups' (TaskGroups.yaml).
type Planning struct{}

// NewPlanning returns the planning service.
func NewPlanning() *Planning { return &Planning{} }

var _ api.PlanningService = (*Planning)(nil)

func loadTyped(ctx context.Context, cwm cwmapi.CWMService, id, typ string) (cwmapi.Object, error) {
	o, _, err := loadAny(ctx, cwm, id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	if o.Type != typ {
		return cwmapi.Object{}, invalid("%s is a %q, not a %s", id, o.Type, typ)
	}
	return o, nil
}

func edgesTo(o cwmapi.Object, kind string) []string {
	var out []string
	for _, e := range o.Edges {
		if e.Kind == kind {
			out = append(out, e.To)
		}
	}
	return out
}

// withEdge replaces every edge of a kind by one to `to` ("" removes them).
func withEdge(o cwmapi.Object, kind, to string) cwmapi.Object {
	edges := o.Edges[:0:0]
	for _, e := range o.Edges {
		if e.Kind != kind {
			edges = append(edges, e)
		}
	}
	if to != "" {
		edges = append(edges, cwmapi.ObjectEdge{Kind: kind, To: to})
	}
	o.Edges = edges
	return o
}

func update(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object) (cwmapi.Object, error) {
	module, err := cwmapi.ModuleOfObjectID(o.ID)
	if err != nil {
		return cwmapi.Object{}, err
	}
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

// childrenOf lists the children of an object, of a type, in the object's module.
func childrenOf(ctx context.Context, cwm cwmapi.CWMService, parent, typ string) ([]cwmapi.Object, error) {
	module, err := cwmapi.ModuleOfObjectID(parent)
	if err != nil {
		return nil, err
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Type: typ, Parent: parent})
	if err != nil {
		return nil, err
	}
	sort.Slice(list.Objects, func(i, j int) bool { return list.Objects[i].ID < list.Objects[j].ID })
	return list.Objects, nil
}

func toStep(o cwmapi.Object) api.Step {
	f, _, _ := cwmapi.DecodeCapability[api.StepFields](o, api.CapStep)
	return api.Step{ID: o.ID, Release: o.ParentID(), Name: o.Name, State: nodeState(o),
		After: dependsOn(o), Ships: edgeTo(o, api.EdgeShips), StepFields: f}
}

// readPlan is a release's plan: its steps and the requests targeting it.
func readPlan(ctx context.Context, cwm cwmapi.CWMService, rel string) (api.Plan, error) {
	plan := api.Plan{Release: rel}
	nodes, err := networkNodes(ctx, cwm, rel)
	if err != nil {
		return plan, err
	}
	for _, n := range nodes {
		if n.Type == api.StepType {
			plan.Steps = append(plan.Steps, toStep(n))
		} else {
			plan.Requests = append(plan.Requests, n.ID)
		}
	}
	return plan, nil
}

// GetPlan reads a release's plan.
func (p *Planning) GetPlan(ctx context.Context, in api.GetInput) (api.PlanOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PlanOutput{}, err
	}
	if _, err := loadRelease(ctx, cwm, in.ID); err != nil {
		return api.PlanOutput{}, err
	}
	plan, err := readPlan(ctx, cwm, in.ID)
	return api.PlanOutput{Plan: plan}, err
}

// openPlan refuses a release whose plan takes no more work.
func openPlan(o cwmapi.Object) error {
	if st := releaseState(o); st == api.ReleaseReleased || st == api.ReleaseDropped {
		return invalid("%s is %s and takes no more work", o.ID, st)
	}
	return nil
}

// inPlan says whether a network node belongs to a release's plan: a step by
// its parent, a request by its target.
func inPlan(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, rel string) bool {
	if o.Type == api.StepType {
		return o.ParentID() == rel
	}
	return takerOf(ctx, cwm, o) == rel
}

// AddStep adds a step to the plan of an open release, after nodes of that plan.
func (p *Planning) AddStep(ctx context.Context, in api.AddStepInput) (api.StepOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || in.Release == "" {
		return api.StepOutput{}, invalid("AddStep: release and name required")
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.StepOutput{}, err
	}
	rel, err := loadRelease(ctx, cwm, in.Release)
	if err != nil {
		return api.StepOutput{}, err
	}
	if err := openPlan(rel); err != nil {
		return api.StepOutput{}, err
	}
	o := cwmapi.Object{Type: api.StepType}
	o.Parent = in.Release
	o.Name = name
	if err := o.SetCapability(api.CapStep, api.StepFields{Produces: in.Produces}); err != nil {
		return api.StepOutput{}, err
	}
	for _, a := range in.After {
		n, err := loadNode(ctx, cwm, a)
		if err != nil {
			return api.StepOutput{}, fmt.Errorf("planning.AddStep: after %s: %w", a, err)
		}
		if !inPlan(ctx, cwm, n, in.Release) {
			return api.StepOutput{}, invalid("AddStep: %s is not in %s's plan; a step joins its own release's work", a, in.Release)
		}
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: a})
	}
	if in.Ships != "" {
		if _, err := loadRelease(ctx, cwm, in.Ships); err != nil {
			return api.StepOutput{}, err
		}
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeShips, To: in.Ships})
	}
	module, _ := cwmapi.ModuleOfObjectID(rel.ID)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.StepOutput{}, err
	}
	return api.StepOutput{Step: toStep(out.Object), Path: out.Path}, nil
}

// withinModule says whether module is `root` or one of its descendants — a
// parent module groups its children (a module view includes theirs).
func withinModule(ctx context.Context, cwm cwmapi.CWMService, module, root string) (bool, error) {
	seen := map[string]bool{}
	for m := module; m != "" && !seen[m]; {
		if m == root {
			return true, nil
		}
		seen[m] = true
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: m})
		if err != nil {
			return false, err
		}
		m = got.Object.ParentID()
	}
	return false, nil
}

// Advance moves a step or a work item through its workflow.
func (p *Planning) Advance(ctx context.Context, in api.AdvanceInput) (api.AdvanceOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.AdvanceOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AdvanceOutput{}, err
	}
	o, _, err := loadAny(ctx, cwm, in.ID)
	if err != nil {
		return api.AdvanceOutput{}, err
	}
	if o.Type != api.StepType {
		return api.AdvanceOutput{}, invalid("Advance: %s is a %q; requests (their plans) and releases move through their own services", in.ID, o.Type)
	}
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: in.ID, To: in.To, By: in.By, Note: in.Note})
	if err != nil {
		return api.AdvanceOutput{}, err
	}
	return api.AdvanceOutput{ID: in.ID, From: out.From, State: out.To}, nil
}

// CheckTransition is the on_transition hook of steps, given the step as it is
// about to be: it is reached only when everything it waits on is done.
func (p *Planning) CheckTransition(ctx context.Context, in api.HookInput) (api.CheckReadyOutput, error) {
	if in.Object == nil {
		return api.CheckReadyOutput{Accept: true}, nil
	}
	o := *in.Object
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.CheckReadyOutput{}, err
	}
	var open []string
	switch {
	case o.Type == api.StepType && nodeState(o) == api.StepReached:
		for _, d := range dependsOn(o) {
			n, err := loadNode(ctx, cwm, d)
			if err != nil {
				return api.CheckReadyOutput{}, err
			}
			if !nodeDone(n) {
				open = append(open, d+" ("+nodeState(n)+")")
			}
		}
	default:
		return api.CheckReadyOutput{Accept: true}, nil
	}
	if len(open) > 0 {
		slices.Sort(open)
		return api.CheckReadyOutput{Reason: "waits on " + strings.Join(open, ", ")}, nil
	}
	return api.CheckReadyOutput{Accept: true}, nil
}
