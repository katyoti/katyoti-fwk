// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// DEMANDS — what was asked and how far it is, as trees: every planned object
// (a request, a feature, a goal) with each part under the demand it was split
// from, wherever the part lives. A module's view roots at its demands whose
// parent is not in it; the parts follow, so a requester follows a demand to
// the modules it went to.

// stateRank orders open demands: what needs a person first, then what moves.
var stateRank = map[string]int{
	api.PlanBlocked: 0, api.PlanIdle: 1, api.PlanReady: 2, api.PlanSplit: 3,
	api.PlanDone: 4, api.PlanDropped: 5,
}

// Demands answers the demands of a module (every module when none) as trees:
// open (the default), a person's open ones (mine), or the ended ones (history,
// the latest first).
func (s *Service) Demands(ctx context.Context, in api.DemandsInput) (api.DemandsOutput, error) {
	// A view carrying the read cache asks the same demands from several of
	// its parts (the deck and its « Needs a person »): read once.
	if c, ok := ctx.Value(readCacheKey{}).(*readCache); ok {
		key := in.Module + "|" + in.View + "|" + in.Person + "|" + in.Split + "|" + demandScopeOf(ctx)
		c.mu.Lock()
		r, hit := c.demands[key]
		c.mu.Unlock()
		if hit {
			return r.out, r.err
		}
		out, err := s.demands(ctx, in)
		c.mu.Lock()
		if c.demands == nil {
			c.demands = map[string]demandsRead{}
		}
		c.demands[key] = demandsRead{out: out, err: err}
		c.mu.Unlock()
		return out, err
	}
	// One listing, one queue and one hold reader for every demand.
	return s.demands(withReadCache(ctx), in)
}

// demandScopeKey carries the object « À faire » is scoped to
// : Demands then builds only the demands whose items
// can be of it.
type demandScopeKey struct{}

// withDemandScope is ctx with Demands scoped to the object id.
func withDemandScope(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, demandScopeKey{}, id)
}

// demandScopeOf is the object Demands is scoped to in ctx, empty when none.
func demandScopeOf(ctx context.Context) string {
	s, _ := ctx.Value(demandScopeKey{}).(string)
	return s
}

// inDemandScope keeps the planned objects whose items can be of the scope -
// an item of a demand is about the demand itself and is placed in the
// increment and the release its work goes to (placeItems: releaseOfWork) -
// and every demand above one of them, so each tree roots and reads as it does
// unscoped.
func inDemandScope(ctx context.Context, cwm cwmapi.CWMService, objects []cwmapi.Object, scope string) []cwmapi.Object {
	byID := map[string]int{}
	for i, o := range objects {
		byID[o.ID] = i
	}
	keep := make([]bool, len(objects))
	for i, o := range objects {
		if keep[i] {
			continue
		}
		if o.ID != scope {
			if inc, rel := releaseOfWork(ctx, cwm, o); inc != scope && rel != scope {
				continue
			}
		}
		// It and the demands above it.
		for j, ok := i, true; ok && !keep[j]; {
			keep[j] = true
			r := objects[j]
			p := r.EdgeTo(api.EdgeSplitFrom)
			if p == "" {
				p = r.EdgeTo(api.EdgeFollowsUp)
			}
			j, ok = byID[p]
		}
	}
	var out []cwmapi.Object
	for i, o := range objects {
		if keep[i] {
			out = append(out, o)
		}
	}
	return out
}

// demandsRead is one Demands answer a read-only view keeps.
type demandsRead struct {
	out api.DemandsOutput
	err error
}

func (s *Service) demands(ctx context.Context, in api.DemandsInput) (api.DemandsOutput, error) {
	view := in.View
	switch view {
	case "":
		view = api.DemandsOpen
	case api.DemandsOpen, api.DemandsHistory:
	case api.DemandsMine:
		if in.Person == "" {
			return api.DemandsOutput{}, invalid("Demands: the mine view names the person")
		}
	default:
		return api.DemandsOutput{}, invalid("Demands: view %q is not open or mine or history", in.View)
	}
	if in.Split != "" && in.Split != "demands" && in.Split != "work" {
		return api.DemandsOutput{}, invalid("Demands: split %q is not demands or work", in.Split)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.DemandsOutput{}, err
	}
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapPlan})
	if err != nil {
		return api.DemandsOutput{}, err
	}
	// Why a demand is not moving is read from its to-dos: what blocks it or
	// whom it waits on, else why its ready to-dos wait in the queue.
	// No scheduler here: each demand reads as its plan says.
	// Several projects served: every demand and the to-dos joining it are
	// keyed by the qualified id.
	got.Objects = qualifyListed(got.Objects, got.Projects)
	refs := newProjectRefs(ctx, len(got.Projects) > 0)
	todos, _ := todosOf(ctx, "")
	held := todoReasons(ctx, todos, refs)
	byRequest := map[string][]api.WorkTodo{}
	for _, t := range todos {
		r := refs.todo(t)
		byRequest[r] = append(byRequest[r], t)
	}

	questioned := openQuestions(ctx, refs)
	demandOf := func(o cwmapi.Object) *api.Demand {
		if view != api.DemandsHistory && endedAlone(o) {
			d := endedDemand(o)
			return &d
		}
		d := toDemand(o, held)
		standing(&d, toPlanned(o), byRequest[d.ID], held, questioned[d.ID], viewStepHold(ctx, o.ID, o.Name))
		return &d
	}
	objects := got.Objects
	if scope := demandScopeOf(ctx); scope != "" {
		objects = inDemandScope(ctx, cwm, objects, scope)
	}
	nodes := map[string]*api.Demand{}
	var order []string
	for _, o := range objects {
		d := demandOf(o)
		nodes[d.ID] = d
		order = append(order, d.ID)
	}
	// A dependency left out by the scope reads as it does unscoped: its own
	// demand, built when asked.
	listed := map[string]cwmapi.Object{}
	if len(objects) < len(got.Objects) {
		for _, o := range got.Objects {
			listed[o.ID] = o
		}
	}
	depOf := func(id string) *api.Demand {
		if n := nodes[id]; n != nil {
			return n
		}
		if o, ok := listed[id]; ok {
			return demandOf(o)
		}
		return nil
	}
	for _, id := range order {
		d := nodes[id]
		if d.Plan == nil && planEnded(d.State) && view != api.DemandsHistory {
			continue // ended, never shown here: what it waited on is moot
		}
		for _, dep := range d.DependsOn {
			if !dependencyEnded(ctx, cwm, depOf(dep), dep) {
				d.WaitsOn = append(d.WaitsOn, dep)
			}
		}
	}
	children := map[string][]string{}
	for _, id := range order {
		if p := parentOf(nodes[id].Request); p != "" && nodes[p] != nil {
			children[p] = append(children[p], id)
		}
	}
	var build func(id string, seen map[string]bool) api.Demand
	build = func(id string, seen map[string]bool) api.Demand {
		d := *nodes[id]
		seen[id] = true
		for _, c := range children[id] {
			if !seen[c] {
				d.Parts = append(d.Parts, build(c, seen))
			}
		}
		sortDemands(d.Parts, false)
		return d
	}

	out := api.DemandsOutput{Demands: []api.Demand{}}
	for _, id := range order {
		d := nodes[id]
		if in.Module != "" && !ofModule(d.Module, in.Module) {
			continue
		}
		if p := parentOf(d.Request); p != "" && nodes[p] != nil && (in.Module == "" || ofModule(nodes[p].Module, in.Module)) {
			continue // a part or a follow-up: shown under its demand
		}
		if in.Split != "" && (d.Kind == api.KindDemand) != (in.Split == "demands") {
			continue // what people asked, or the modules' work
		}
		ended := planEnded(d.State)
		switch view {
		case api.DemandsHistory:
			if !ended {
				continue
			}
		case api.DemandsMine:
			if ended || d.Requester.ID != in.Person {
				continue
			}
		default:
			if ended {
				continue
			}
		}
		out.Demands = append(out.Demands, build(id, map[string]bool{}))
	}
	sortDemands(out.Demands, view == api.DemandsHistory)
	return out, nil
}

// endedAlone says a planned object's plan ended and it is no part of
// another (no split-from, no follow-up edge): an open or mine view never
// shows it - not even under a demand -, it only answers whether it ended (a
// dependency) and whether it is a parent. Most planned objects are such, and
// decoding each whole plan was most of « À faire »'s read
func endedAlone(o cwmapi.Object) bool {
	return planEnded(planState(o)) && o.EdgeTo(api.EdgeSplitFrom) == "" && o.EdgeTo(api.EdgeFollowsUp) == ""
}

// endedDemand is an ended object as an open view needs it: who it is, its
// state and its edges - no plan decoded.
func endedDemand(o cwmapi.Object) api.Demand {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	return api.Demand{Request: api.Request{ID: o.ID, Type: o.Type, Module: module, Title: titleOf(o), State: planState(o),
		DependsOn: dependsOn(o), Resolves: o.EdgeTo(api.EdgeResolves)}}
}

// ofModule says a demand's module is the module asked: the same reference,
// or - a bare module asked - the module of that name in any project.
func ofModule(module, asked string) bool {
	return module == asked || keyOf(asked) == "" && bareID(module) == asked
}

// toDemand reads one planned object as a demand: its ask, its plan, why it
// is not moving, and its activity.
func toDemand(o cwmapi.Object, held map[string]string) api.Demand {
	d := api.Demand{Request: toRequest(o)}
	if d.Plan == nil {
		return d
	}
	d.Timeline = timelineOf(*d.Plan, d.Waiver)
	switch d.State {
	case api.PlanIdle, api.PlanBlocked:
		d.Why = d.Plan.Note
	case api.PlanReady:
		d.Why = held[d.ID]
	case api.PlanDone, api.PlanDropped:
		// How it ended is its last move's: when, and the note given.
		if n := len(d.Plan.Trail); n > 0 {
			d.EndedAt, d.Why = d.Plan.Trail[n-1].At, d.Plan.Trail[n-1].Note
		}
	}
	return d
}

// dependencyEnded reports a dependency ended: its plan, else — one not listed
// — its own lifecycle. One that cannot be read is still waited on.
func dependencyEnded(ctx context.Context, cwm cwmapi.CWMService, n *api.Demand, dep string) bool {
	if n != nil {
		return planEnded(n.State)
	}
	o, err := loadObject(ctx, cwm, dep)
	return err == nil && nodeEnded(o)
}

// parentOf is the demand a request belongs to: the one it was split from, or
// the one whose step filed it (a follow-up).
func parentOf(r api.Request) string {
	if r.SplitFrom != "" {
		return r.SplitFrom
	}
	return r.FollowsUp
}

// standing sets where an open demand stands from its to-dos — the same
// reading as the deck's: its state (ready, running, waiting, blocked), why, and
// the step it is on — never the next step written when it was filed. A demand
// with no to-do (planned outside a work plan) reads as its plan says.
// questioned says an intake question of the demand is still open (a human
// task).
func standing(d *api.Demand, p api.PlannedObject, todos []api.WorkTodo, held map[string]string, questioned bool, hold stepHold) {
	if d.Plan == nil || planEnded(d.State) || d.State == api.PlanSplit || len(todos) == 0 {
		return
	}
	st := standOf(p, todos, (questioned || len(d.AskedIn) > 0) && d.State == api.PlanIdle, hold)
	for plan, label := range stateLabels {
		if label == st.state && plan != api.PlanSplit && !planEnded(plan) {
			d.State = plan
		}
	}
	d.Why = st.why
	if d.Why == "" && d.State == api.PlanReady {
		d.Why = held[d.ID] // a ready step held in the queue: why
	}
	if t := st.step; t != nil {
		s := &api.DemandStep{Step: t.Packet.Step, Phase: t.Packet.Phase, Who: t.Packet.Task, Interaction: t.Packet.Interaction, Ask: t.Packet.AskedIn}
		if t.Packet.Doer == api.DoerPerson {
			s.Who = api.DoerPerson
		}
		if t.Claim != nil && t.State == schedapi.StateClaimed {
			by := t.Claim.By
			s.HeldBy, s.Until = &by, t.Claim.Until
		}
		// Needs you once asked: a module whose scheduling is off asks no one.
		s.NeedsYou = t.For == schedapi.ForPerson && t.State == schedapi.StateReady && askedOf(*t)
		d.Step = s
	}
}

// sortDemands orders open demands by state (what needs a person first), then
// priority, then age; history the latest ended first.
func sortDemands(ds []api.Demand, history bool) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if history {
			if a.EndedAt != b.EndedAt {
				return a.EndedAt > b.EndedAt
			}
			return a.ID < b.ID
		}
		if ra, rb := stateRank[a.State], stateRank[b.State]; ra != rb {
			return ra < rb
		}
		if pa, pb := priorityOf(a), priorityOf(b); pa != pb {
			return pa < pb
		}
		return a.ID < b.ID
	})
}

func priorityOf(d api.Demand) int {
	if d.Plan == nil {
		return priorityRank(api.DefaultPriority)
	}
	return priorityRank(d.Plan.Priority)
}

// todoReasons is, per request whose work is not moving - a to-do of it
// waits, or the queue holds a ready one back - what holds it, as planning
// check names it (holdOf, holdWords): read from the
// to-dos given, one queue and one reader, never per request.
func todoReasons(ctx context.Context, all []api.WorkTodo, refs *projectRefs) map[string]string {
	out := map[string]string{}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	say := func(request string) {
		if seen[request] {
			return
		}
		seen[request] = true
		if o, err := loadObject(ctx, cwm, request); err == nil {
			if b, ok := holdOf(ctx, o); ok {
				out[request] = orDefault(b.Step, "its work") + " waits: " + holdWords(b)
			}
		}
	}
	for _, t := range all {
		if t.State == schedapi.StateWaiting {
			say(refs.todo(t))
		}
	}
	// Why a ready step waits in the queue. (Where an open demand stands is
	// read from its to-dos by standing — no rollup per request, which read
	// every to-do again for each one.)
	if q, err := queueOf(ctx, ""); err == nil {
		for _, e := range q.Queue {
			if !e.Takeable {
				say(refs.ref(e.Todo.Request, e.Todo.Repository))
			}
		}
	}
	return out
}
