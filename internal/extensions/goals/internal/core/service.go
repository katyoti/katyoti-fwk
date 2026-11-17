// SPDX-License-Identifier: Apache-2.0

// Package core implements the goals service over the CWM Object API. It
// reads and writes goal objects only through CWMService; nothing is stored here.
package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
)

// Service implements api.GoalsService.
type Service struct{}

// New returns the goals service.
func New() *Service { return &Service{} }

var _ api.GoalsService = (*Service)(nil)

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	return application.UseService[cwmapi.CWMService](ctx)
}

// getGoal loads an object by id and refuses anything that is not a goal.
func getGoal(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, string, error) {
	o, module, _, err := getGoalRev(ctx, cwm, id)
	return o, module, err
}

// getGoalRev is getGoal with the revision read: an update hands it back, so
// it never overwrites a goal written since.
func getGoalRev(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, string, string, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, "", "", err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, "", "", err
	}
	if got.Object.Type != api.GoalType {
		return cwmapi.Object{}, "", "", fmt.Errorf("%w: %s is a %q, not a goal", cwmapi.ErrInvalidInput, id, got.Object.Type)
	}
	return got.Object, module, got.Revision, nil
}

// checkParent accepts the module itself or a goal of the same module.
func checkParent(ctx context.Context, cwm cwmapi.CWMService, module, parent string) error {
	if parent == module {
		return nil
	}
	pm, err := cwmapi.ModuleOfObjectID(parent)
	if err != nil {
		return err
	}
	if pm != module {
		return fmt.Errorf("%w: parent %s is in module %s, the goal in %s — a goal stays in its module", cwmapi.ErrInvalidInput, parent, pm, module)
	}
	_, _, err = getGoal(ctx, cwm, parent)
	return err
}

// Add creates a goal under the module or under another goal of the module.
func (s *Service) Add(ctx context.Context, in api.AddInput) (api.AddOutput, error) {
	if in.Module == "" || strings.TrimSpace(in.Title) == "" {
		return api.AddOutput{}, fmt.Errorf("%w: goals.Add: module and title required", cwmapi.ErrInvalidInput)
	}
	// A goal is always asymptotic: a want a test can declare done is a feature.
	reach := in.Reachability
	switch reach {
	case "":
		reach = "asymptotic"
	case "asymptotic":
	default:
		return api.AddOutput{}, fmt.Errorf("%w: goals.Add: a goal is always asymptotic, not %q — something a test can declare done is a feature", cwmapi.ErrInvalidInput, reach)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AddOutput{}, err
	}
	parent := in.Parent
	if parent == "" {
		parent = in.Module
	}
	if err := checkParent(ctx, cwm, in.Module, parent); err != nil {
		return api.AddOutput{}, err
	}
	obj := cwmapi.Object{Type: api.GoalType}
	obj.Parent = parent
	obj.Name = in.Title
	_ = obj.SetCapability(cwmapi.CapWant, map[string]any{"reachability": reach})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: in.Module, Object: obj})
	if err != nil {
		return api.AddOutput{}, err
	}
	return api.AddOutput{Goal: toGoal(in.Module, out.Object), Path: out.Path}, nil
}

// Remove deletes a goal. It refuses a goal with child goals (they would be
// stranded) and a goal something serves (the serves edges would dangle).
func (s *Service) Remove(ctx context.Context, in api.RemoveInput) (api.RemoveOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.RemoveOutput{}, err
	}
	_, module, err := getGoal(ctx, cwm, in.ID)
	if err != nil {
		return api.RemoveOutput{}, err
	}
	kids, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: in.ID, Type: api.GoalType})
	if err != nil {
		return api.RemoveOutput{}, err
	}
	if len(kids.Objects) > 0 {
		return api.RemoveOutput{}, fmt.Errorf("%w: goals.Remove: %s has %d child goal(s); reparent them first", cwmapi.ErrTypeRuleViolation, in.ID, len(kids.Objects))
	}
	servers, err := servesEdgesInto(ctx, cwm, in.ID)
	if err != nil {
		return api.RemoveOutput{}, err
	}
	if len(servers) > 0 {
		return api.RemoveOutput{}, fmt.Errorf("%w: goals.Remove: %s is served by %s; unlink them first", cwmapi.ErrTypeRuleViolation, in.ID, strings.Join(servers, ", "))
	}
	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: module, ID: in.ID}); err != nil {
		return api.RemoveOutput{}, err
	}
	return api.RemoveOutput{ID: in.ID}, nil
}

// List returns a module's goals, grouped by parent, in their order.
func (s *Service) List(ctx context.Context, in api.ListInput) (api.ListOutput, error) {
	if in.Module == "" {
		return api.ListOutput{}, fmt.Errorf("%w: goals.List: module required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListOutput{}, err
	}
	goals, err := listGoals(ctx, cwm, in.Module)
	if err != nil {
		return api.ListOutput{}, err
	}
	return api.ListOutput{Goals: goals}, nil
}

// Reorder writes each goal's position. The ids must be exactly the goals under
// one parent: a partial list would leave the rest with stale positions.
func (s *Service) Reorder(ctx context.Context, in api.ReorderInput) (api.ReorderOutput, error) {
	if len(in.IDs) == 0 {
		return api.ReorderOutput{}, fmt.Errorf("%w: goals.Reorder: ids required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReorderOutput{}, err
	}
	objs := make([]cwmapi.Object, len(in.IDs))
	revs := make([]string, len(in.IDs))
	module, parent := "", ""
	for i, id := range in.IDs {
		o, m, rev, err := getGoalRev(ctx, cwm, id)
		if err != nil {
			return api.ReorderOutput{}, err
		}
		revs[i] = rev
		if i == 0 {
			module, parent = m, o.ParentID()
		} else if o.ParentID() != parent {
			return api.ReorderOutput{}, fmt.Errorf("%w: goals.Reorder: %s sits under %s, not %s — order one parent's goals at a time", cwmapi.ErrInvalidInput, id, o.ParentID(), parent)
		}
		objs[i] = o
	}
	siblings, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Parent: parent, Type: api.GoalType})
	if err != nil {
		return api.ReorderOutput{}, err
	}
	given := map[string]bool{}
	for _, id := range in.IDs {
		if given[id] {
			return api.ReorderOutput{}, fmt.Errorf("%w: goals.Reorder: %s listed twice", cwmapi.ErrInvalidInput, id)
		}
		given[id] = true
	}
	for _, sib := range siblings.Objects {
		if !given[sib.ID] {
			return api.ReorderOutput{}, fmt.Errorf("%w: goals.Reorder: %s also sits under %s and is missing from the order", cwmapi.ErrInvalidInput, sib.ID, parent)
		}
	}
	out := make([]api.Goal, len(objs))
	for i, o := range objs {

		if o.Parent == "" {
			o.Parent = parent
		}
		o.Order = i + 1
		up, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: revs[i]})
		if err != nil {
			return api.ReorderOutput{}, err
		}
		out[i] = toGoal(module, up.Object)
	}
	return api.ReorderOutput{Goals: out}, nil
}

// Reparent hangs a goal under another goal of its module, or under the module.
// A move within a module is an update of parent; the position is
// cleared because it belonged to the old siblings.
func (s *Service) Reparent(ctx context.Context, in api.ReparentInput) (api.ReparentOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ReparentOutput{}, err
	}
	o, module, rev, err := getGoalRev(ctx, cwm, in.ID)
	if err != nil {
		return api.ReparentOutput{}, err
	}
	parent := in.NewParent
	if parent == "" {
		parent = module
	}
	if err := checkParent(ctx, cwm, module, parent); err != nil {
		return api.ReparentOutput{}, err
	}
	// No cycle: the new parent may not be the goal or one of its descendants.
	for p := parent; p != module; {
		if p == in.ID {
			return api.ReparentOutput{}, fmt.Errorf("%w: goals.Reparent: %s is %s itself or under it", cwmapi.ErrTypeRuleViolation, parent, in.ID)
		}
		po, _, err := getGoal(ctx, cwm, p)
		if err != nil {
			return api.ReparentOutput{}, err
		}
		p = po.ParentID()
	}
	o.Parent = parent
	o.Order = 0 // a new place: the old position among the old siblings goes
	up, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o, Revision: rev})
	if err != nil {
		return api.ReparentOutput{}, err
	}
	return api.ReparentOutput{Goal: toGoal(module, up.Object)}, nil
}

// LinkServer writes the serves edge on the server, which owns its outgoing edges.
func (s *Service) LinkServer(ctx context.Context, in api.LinkServerInput) (api.LinkServerOutput, error) {
	if in.Goal == "" || in.Server == "" {
		return api.LinkServerOutput{}, fmt.Errorf("%w: goals.LinkServer: goal and server required", cwmapi.ErrInvalidInput)
	}
	if in.Goal == in.Server {
		return api.LinkServerOutput{}, fmt.Errorf("%w: goals.LinkServer: a goal does not serve itself", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.LinkServerOutput{}, err
	}
	if _, _, err := getGoal(ctx, cwm, in.Goal); err != nil {
		return api.LinkServerOutput{}, err
	}
	sm, err := cwmapi.ModuleOfObjectID(in.Server)
	if err != nil {
		return api.LinkServerOutput{}, err
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: sm, ID: in.Server}); err != nil {
		return api.LinkServerOutput{}, err
	}
	out, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: in.Server, To: in.Goal, Kind: cwmapi.EdgeType(api.EdgeServes)})
	if err != nil {
		return api.LinkServerOutput{}, err
	}
	return api.LinkServerOutput{From: out.Edge.From, To: out.Edge.To, Kind: string(out.Edge.Kind)}, nil
}

// Uncovered lists the goals no object serves.
func (s *Service) Uncovered(ctx context.Context, in api.UncoveredInput) (api.UncoveredOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.UncoveredOutput{}, err
	}
	goals, err := listGoals(ctx, cwm, in.Module)
	if err != nil {
		return api.UncoveredOutput{}, err
	}
	out := []api.Goal{}
	for _, g := range goals {
		servers, err := serversOf(ctx, cwm, g.ID)
		if err != nil {
			return api.UncoveredOutput{}, err
		}
		if len(servers) == 0 {
			out = append(out, g)
		}
	}
	return api.UncoveredOutput{Goals: out}, nil
}

// serversOf returns what serves id: the objects with a serves edge into it,
// but a request that is not itself a want — work asked in the goal's name (a
// fix, a build pointing at it) advances it, it does not serve it. A feature
// carrying a request still serves.
func serversOf(ctx context.Context, cwm cwmapi.CWMService, id string) ([]string, error) {
	edges, err := servesEdgesInto(ctx, cwm, id)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, from := range edges {
		if workOnly(ctx, cwm, from) {
			continue
		}
		out = append(out, from)
	}
	return out, nil
}

// servesEdgesInto returns every object with a serves edge into id, requests
// included: what an edit of the goal must account for. A qualified id
// (<key>:<module>.<local>) answers its project's edges, the servers named in
// that project.
func servesEdgesInto(ctx context.Context, cwm cwmapi.CWMService, id string) ([]string, error) {
	edges, err := cwm.EdgeList(ctx, cwmapi.EdgeListInput{ObjectID: id, Kind: cwmapi.EdgeType(api.EdgeServes), Direction: cwmapi.DirectionIn})
	if err != nil {
		return nil, err
	}
	key, bare, err := cwmapi.SplitReference(id)
	if err != nil {
		key, bare = "", id
	}
	var out []string
	for _, e := range edges.Edges {
		if e.To != id && e.To != bare {
			continue
		}
		from := e.From
		if key != "" && !strings.Contains(from, ":") {
			from = cwmapi.QualifiedID(key, from)
		}
		out = append(out, from)
	}
	return out, nil
}

// capRequest is planification's request capability: work asked for.
const capRequest = "request"

// workOnly reports an object carrying a request (planification's) that is not
// itself a want: a request of its own, never a feature with work on it.
func workOnly(ctx context.Context, cwm cwmapi.CWMService, id string) bool {
	eff, err := cwm.ObjectEffectiveCapabilities(ctx, cwmapi.ObjectEffectiveCapabilitiesInput{ID: id})
	if err != nil {
		return false
	}
	request, want := false, false
	for _, c := range eff.Capabilities {
		switch c.ID {
		case capRequest:
			request = true
		case cwmapi.CapWant:
			want = true
		}
	}
	return request && !want
}

func listGoals(ctx context.Context, cwm cwmapi.CWMService, module string) ([]api.Goal, error) {
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Type: api.GoalType})
	if err != nil {
		return nil, err
	}
	goals := make([]api.Goal, 0, len(list.Objects))
	for _, o := range list.Objects {
		m, _ := cwmapi.ModuleOfObjectID(o.ID)
		goals = append(goals, toGoal(m, o))
	}
	sort.SliceStable(goals, func(i, j int) bool {
		a, b := goals[i], goals[j]
		if a.Parent != b.Parent {
			return a.Parent < b.Parent
		}
		if a.Order != b.Order {
			// Ordered goals first; an unordered one (0) goes after them.
			if a.Order == 0 || b.Order == 0 {
				return b.Order == 0
			}
			return a.Order < b.Order
		}
		return a.ID < b.ID
	})
	return goals, nil
}

func toGoal(module string, o cwmapi.Object) api.Goal {
	g := api.Goal{ID: o.ID, Module: module, Title: o.DisplayTitle(), Parent: o.ParentID()}
	if r, ok := o.Capabilities[cwmapi.CapWant]["reachability"].(string); ok {
		g.Reachability = r
	}
	g.Order = o.Order // a position under a parent is a tree fact: the object core's
	return g
}
