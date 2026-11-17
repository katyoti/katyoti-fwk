// SPDX-License-Identifier: Apache-2.0

package core

// The verbs that manage features over the CWM Object API: a criterion is an
// acceptance-criterion object under its feature; the goals a feature serves are
// its serves edges; its state is the `feature` workflow's. Nothing is stored
// here.

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
)

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// get loads an object in full by id and refuses one of another type (typ
// empty: any type), with the revision read: an update hands it back, so it
// never overwrites an object written since.
func get(ctx context.Context, cwm cwmapi.CWMService, id, typ string) (cwmapi.Object, string, string, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, "", "", err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, "", "", err
	}
	if typ != "" && got.Object.Type != typ {
		return cwmapi.Object{}, "", "", fmt.Errorf("%w: %s is a %q, not a %s", cwmapi.ErrInvalidInput, id, got.Object.Type, typ)
	}
	return got.Object, module, got.Revision, nil
}

// stateOf is the feature's lifecycle state; none is the workflow's initial.
func stateOf(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok && l.State != "" {
		return l.State
	}
	return api.StateInitial
}

// live: not dropped — retired and split features are dropped, the reason in
// their lifecycle history.
func live(o cwmapi.Object) bool { return stateOf(o) != api.StateDropped }

// drop moves a feature to dropped through its workflow, the note saying why.
func drop(ctx context.Context, cwm cwmapi.CWMService, id string, actor cwmapi.Actor, note string) (cwmapi.Object, error) {
	if actor.ID == "" {
		return cwmapi.Object{}, fmt.Errorf("%w: features: actor required — who retires or splits is recorded", cwmapi.ErrInvalidInput)
	}
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: api.StateDropped, By: actor, Note: note})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

// setWant writes the title a feature is read by and its reachability: a
// feature is always closable.
func setWant(o *cwmapi.Object, title string) {
	o.Name = title
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
}

// Add creates a feature serving the goals it names, under the module or under
// the object named (a feature, for a sub-feature).
func (s *Service) Add(ctx context.Context, in api.AddInput) (api.AddOutput, error) {
	if in.Module == "" || strings.TrimSpace(in.Title) == "" {
		return api.AddOutput{}, fmt.Errorf("%w: features.Add: module and title required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AddOutput{}, err
	}
	for _, g := range in.Serves {
		if _, _, _, err := get(ctx, cwm, g, "goal"); err != nil {
			return api.AddOutput{}, fmt.Errorf("features.Add: serves %s: %w", g, err)
		}
	}
	parent := in.Parent
	if parent == "" {
		parent = in.Module
	}
	o := cwmapi.Object{Type: api.FeatureType}
	o.Parent = parent
	setWant(&o, in.Title)
	for _, g := range in.Serves {
		o.Edges = append(o.Edges, cwmapi.ObjectEdge{Kind: api.EdgeServes, To: g})
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: o})
	if err != nil {
		return api.AddOutput{}, err
	}
	return api.AddOutput{Feature: toFeature(in.Module, out.Object), Path: out.Path}, nil
}

// Split replaces a feature with several through ObjectSplit, which writes the
// trail and moves the children each successor adopts (criteria, sub-features).
// Each successor is made a feature in full — closable, in the initial state —
// and serves every goal the source served, so no goal loses coverage by the
// split. The source stays, dropped, as the record of what was replaced. The
// count of successors and the reason are ObjectSplit's own checks.
func (s *Service) Split(ctx context.Context, in api.SplitInput) (api.SplitOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.SplitOutput{}, err
	}
	src, _, _, err := get(ctx, cwm, in.ID, api.FeatureType)
	if err != nil {
		return api.SplitOutput{}, err
	}
	if !live(src) {
		return api.SplitOutput{}, fmt.Errorf("%w: features.Split: %s is no longer live", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	if in.Actor.ID == "" {
		return api.SplitOutput{}, fmt.Errorf("%w: features.Split: actor required — who splits is recorded", cwmapi.ErrInvalidInput)
	}
	targets := make([]cwmapi.ObjectSplitTarget, len(in.Successors))
	for i, succ := range in.Successors {
		targets[i] = cwmapi.ObjectSplitTarget{Type: api.FeatureType, Title: succ.Title, Children: succ.Children}
	}
	split, err := cwm.ObjectSplit(ctx, cwmapi.ObjectSplitInput{ID: in.ID, Successors: targets, Reason: in.Reason})
	if err != nil {
		return api.SplitOutput{}, err
	}
	var serves []cwmapi.ObjectEdge
	for _, e := range src.Edges {
		if e.Kind == api.EdgeServes {
			serves = append(serves, e)
		}
	}
	out := api.SplitOutput{Orphans: split.Orphans}
	for i, id := range split.Successors {
		o, m, rev, err := get(ctx, cwm, id, "")
		if err != nil {
			return api.SplitOutput{}, err
		}
		setWant(&o, in.Successors[i].Title)
		o.Edges = append(o.Edges, serves...)
		up, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: m, Object: o, Revision: rev})
		if err != nil {
			return api.SplitOutput{}, err
		}
		out.Successors = append(out.Successors, toFeature(m, up.Object))
	}
	if _, err := drop(ctx, cwm, in.ID, in.Actor, "split: "+in.Reason); err != nil {
		return api.SplitOutput{}, err
	}
	return out, nil
}

// Retire drops a feature. It stays, so what it once wanted and why it stopped
// is still readable; it leaves Uncovered and takes no new criterion.
func (s *Service) Retire(ctx context.Context, in api.RetireInput) (api.RetireOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RetireOutput{}, err
	}
	o, module, rev, err := get(ctx, cwm, in.ID, api.FeatureType)
	if err != nil {
		return api.RetireOutput{}, err
	}
	if !live(o) {
		return api.RetireOutput{}, fmt.Errorf("%w: features.Retire: %s is already retired or split", cwmapi.ErrTypeRuleViolation, in.ID)
	}
	if in.Actor.ID == "" {
		return api.RetireOutput{}, fmt.Errorf("%w: features.Retire: actor required — who retires is recorded", cwmapi.ErrInvalidInput)
	}
	if r := strings.TrimSpace(in.Reason); r != "" {
		o.Body = appendProse(o.Body, "## Retired\n\n"+r)
		// The body first, then the state: the other order would make this
		// write carry the stale state.
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: rev}); err != nil {
			return api.RetireOutput{}, err
		}
	}
	dropped, err := drop(ctx, cwm, in.ID, in.Actor, "retired: "+strings.TrimSpace(in.Reason))
	if err != nil {
		return api.RetireOutput{}, err
	}
	return api.RetireOutput{Feature: toFeature(module, dropped)}, nil
}

// AttachCriterion creates an acceptance-criterion under a live feature. With
// no probe the criterion is declared unmeasurable (`checkable.kind: none`)
// rather than given a probe that could only pass vacuously.
func (s *Service) AttachCriterion(ctx context.Context, in api.AttachCriterionInput) (api.AttachCriterionOutput, error) {
	if strings.TrimSpace(in.Title) == "" {
		return api.AttachCriterionOutput{}, fmt.Errorf("%w: features.AttachCriterion: title required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AttachCriterionOutput{}, err
	}
	f, module, _, err := get(ctx, cwm, in.Feature, api.FeatureType)
	if err != nil {
		return api.AttachCriterionOutput{}, err
	}
	if !live(f) {
		return api.AttachCriterionOutput{}, fmt.Errorf("%w: features.AttachCriterion: %s is no longer live", cwmapi.ErrTypeRuleViolation, in.Feature)
	}
	check := map[string]any{"kind": "none"}
	if strings.TrimSpace(in.Probe) != "" {
		sev := in.Severity
		if sev == "" {
			sev = "core"
		}
		check = map[string]any{"kind": "command", "probe": in.Probe, "severity": sev}
	}
	o := cwmapi.Object{Type: api.CriterionType}
	o.Parent = in.Feature
	o.Name = in.Title
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	_ = o.SetCapability(cwmapi.CapCheckable, check)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		return api.AttachCriterionOutput{}, err
	}
	return api.AttachCriterionOutput{Criterion: toCriterion(out.Object), Path: out.Path}, nil
}

// Uncovered lists the live features nothing checks. A feature is recursive: it
// is checked when a criterion hangs under it or under one of its live
// sub-features, at any depth — a feature proven by its sub-features is not
// reported, the sub-feature that has no criterion is.
func (s *Service) Uncovered(ctx context.Context, in api.UncoveredInput) (api.UncoveredOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.UncoveredOutput{}, err
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Type: api.FeatureType})
	if err != nil {
		return api.UncoveredOutput{}, err
	}
	memo := map[string]bool{}
	out := []api.Feature{}
	for _, o := range list.Objects {
		if !live(o) {
			continue
		}
		checked, err := checkedUnder(ctx, cwm, o.ID, memo, map[string]bool{})
		if err != nil {
			return api.UncoveredOutput{}, err
		}
		if !checked {
			m, _ := cwmapi.ModuleOfObjectID(o.ID)
			out = append(out, toFeature(m, o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return api.UncoveredOutput{Features: out}, nil
}

// checkedUnder: a criterion hangs under the feature id, or under one of its
// live sub-features (recursively). memo keeps each answer; path stops a cycle.
func checkedUnder(ctx context.Context, cwm cwmapi.CWMService, id string, memo, path map[string]bool) (bool, error) {
	if v, ok := memo[id]; ok {
		return v, nil
	}
	if path[id] {
		return false, nil
	}
	path[id] = true
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: id})
	if err != nil {
		return false, err
	}
	checked := false
	for _, k := range kids.Objects {
		if k.Type == api.CriterionType {
			checked = true
			break
		}
	}
	for _, k := range kids.Objects {
		if checked {
			break
		}
		if k.Type != api.FeatureType || !live(k) {
			continue
		}
		if checked, err = checkedUnder(ctx, cwm, k.ID, memo, path); err != nil {
			return false, err
		}
	}
	memo[id] = checked
	return checked, nil
}

// Goals lists the objects a feature serves, read from its serves edges.
func (s *Service) Goals(ctx context.Context, in api.GoalsInput) (api.GoalsOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.GoalsOutput{}, err
	}
	if _, _, _, err := get(ctx, cwm, in.ID, api.FeatureType); err != nil {
		return api.GoalsOutput{}, err
	}
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: in.ID, Kind: cwmapi.EdgeType(api.EdgeServes), Direction: cwmapi.DirectionOut})
	if err != nil {
		return api.GoalsOutput{}, err
	}
	out := api.GoalsOutput{Goals: []api.Served{}}
	for _, e := range edges.Edges {
		if e.From != in.ID {
			continue
		}
		served := api.Served{ID: e.To}
		if o, _, _, err := get(ctx, cwm, e.To, ""); err == nil {
			served.Type, served.Title = o.Type, o.DisplayTitle()
		}
		out.Goals = append(out.Goals, served)
	}
	return out, nil
}

func toFeature(module string, o cwmapi.Object) api.Feature {
	f := api.Feature{ID: o.ID, Module: module, Title: o.DisplayTitle(), Parent: o.ParentID(), Status: stateOf(o)}
	for _, e := range o.Edges {
		if e.Kind == api.EdgeServes {
			f.Serves = append(f.Serves, e.To)
		}
	}
	return f
}

func toCriterion(o cwmapi.Object) api.Criterion {
	cr := api.Criterion{ID: o.ID, Feature: o.ParentID(), Title: o.DisplayTitle()}
	if p, ok := o.Capabilities[cwmapi.CapCheckable]["probe"].(string); ok {
		cr.Probe = p
	}
	if s, ok := o.Capabilities[cwmapi.CapCheckable]["severity"].(string); ok {
		cr.Severity = s
	}
	return cr
}

// appendProse adds a section to a feature's markdown body.
func appendProse(prose, section string) string {
	if prose != "" {
		prose = strings.TrimRight(prose, "\n") + "\n\n"
	}
	return prose + section + "\n"
}
