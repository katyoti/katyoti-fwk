// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Plans is the plan service: the one way an object's plan becomes ready
// (Plan), the ways it ends (Finish, Block), and what is read of it (Get,
// List). Its steps are its to-dos (Work); the plan records where the work
// stands and what each step came to.
type Plans struct{}

// NewPlans returns the plan service.
func NewPlans() *Plans { return &Plans{} }

var _ api.PlanService = (*Plans)(nil)

// Plan makes an object's plan ready, with its priority: an object without a
// plan gets one; a plan that ended is not planned again (refile it). A
// blocked request resumed with all its to-dos ended tries its end again: it
// ends, or is blocked again saying why - never ready with nothing to do and in
// no queue.
func (p *Plans) Plan(ctx context.Context, in api.PlanNextInput) (api.PlannedOutput, error) {
	was := ""
	if cwm, err := cwmOf(ctx); err == nil {
		if o, err := loadObject(ctx, cwm, in.ID); err == nil && hasRequest(o) {
			was = planState(o)
		}
	}
	out, err := p.plan(ctx, in)
	if err != nil || was != api.PlanBlocked {
		return out, err
	}
	// Resumed, what its block held is offered again.
	if err := settleRequest(ctx, in.ID, in.By); err != nil {
		return out, err
	}
	// A round an agent reviewer sent back past its cap is planned now, as the
	// person's: one more round.
	if err := resumeSentBack(ctx, in.ID, in.By); err != nil {
		return out, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out, err
	}
	if err := endRequest(ctx, cwm, in.ID, in.By); err != nil {
		return out, err
	}
	o, err := loadObject(ctx, cwm, in.ID)
	if err != nil {
		return out, err
	}
	return api.PlannedOutput{Plan: toPlanned(o)}, nil
}

// plan makes an object's plan ready: Plan without the resume of a blocked
// request's end - the intake of a request filed or answered plans with it.
func (p *Plans) plan(ctx context.Context, in api.PlanNextInput) (api.PlannedOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.PlannedOutput{}, err
	}
	if in.Priority != "" && !slices.Contains(api.Priorities, in.Priority) {
		return api.PlannedOutput{}, invalid("Plan: priority %q is not one of %v", in.Priority, api.Priorities)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	o, err := loadObject(ctx, cwm, in.ID)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	f, has := planOf(o)
	if has && planEnded(f.State) {
		return api.PlannedOutput{}, invalid("Plan: %s's plan is %s", in.ID, f.State)
	}
	// Who planned it, and when, is in the trail (the plan transition's).
	f.Note, f.Block = "", ""
	if in.Priority != "" {
		f.Priority = in.Priority
		// A request shows the priority it is taken by: the two stay one.
		if has && hasRequest(o) {
			rf := fieldsOf(o)
			if rf.Priority != in.Priority {
				rf.Priority = in.Priority
				if err := o.SetCapability(api.CapRequest, rf); err != nil {
					return api.PlannedOutput{}, err
				}
			}
		}
	} else if f.Priority == "" {
		f.Priority = api.DefaultPriority
	}
	if o, err = writePlan(ctx, cwm, o, f, has); err != nil {
		return api.PlannedOutput{}, err
	}
	if in.Priority != "" {
		if err := reprioritise(ctx, in.ID, in.Priority); err != nil {
			return api.PlannedOutput{}, err
		}
	}
	// Always a plan transition — ready to ready included: the trail is the one
	// record of who planned it.
	if o, err = planTransition(ctx, cwm, in.ID, api.PlanReady, "plan", in.By, in.Note); err != nil {
		return api.PlannedOutput{}, err
	}
	return api.PlannedOutput{Plan: toPlanned(o)}, nil
}

// reprioritise moves a request's to-dos not started to its new priority: the
// queue orders by the to-do's, so it follows the request's at once. One a
// worker holds keeps its own.
func reprioritise(ctx context.Context, request, priority string) error {
	todos, err := todosOf(ctx, request)
	if err != nil {
		return err
	}
	for _, t := range todos {
		if !notStarted(t) || t.Priority == priority {
			continue
		}
		t.Priority = priority
		if _, err := put(ctx, t); err != nil {
			return err
		}
	}
	return nil
}

// Finish ends a plan: done, or dropped.
func (p *Plans) Finish(ctx context.Context, in api.FinishInput) (api.PlannedOutput, error) {
	out, after, err := p.finishWrites(ctx, in)
	if err == nil {
		after(ctx)
	}
	return out, err
}

// finishWrites is Finish's CWM writes - its checks, its plan's move - and,
// apart, what follows its end (afterEnd): the request's end writes the first
// in its change set and runs the second once it is committed, since what
// follows lists objects, and a list never sees what a set staged
func (p *Plans) finishWrites(ctx context.Context, in api.FinishInput) (api.PlannedOutput, func(context.Context), error) {
	if err := checkActor(in.By); err != nil {
		return api.PlannedOutput{}, nil, err
	}
	to, name := api.PlanDone, "finish"
	switch in.Outcome {
	case "", api.PlanDone:
	case api.PlanDropped:
		to, name = api.PlanDropped, "drop"
	default:
		return api.PlannedOutput{}, nil, invalid("Finish: outcome %q is neither done nor dropped", in.Outcome)
	}
	if to == api.PlanDone && in.By.ID == IntakeTask {
		// The intake reads, assesses and orders; it does none of the work, so
		// it cannot say the work is done. Ending its own step is `complete`.
		return api.PlannedOutput{}, nil, invalid("Finish: the intake ends a request only as dropped (a duplicate) — to end your own step, complete: the recipe plans the next one")
	}
	if to == api.PlanDone {
		if err := p.checkDeliverables(ctx, in.ID); err != nil {
			return api.PlannedOutput{}, nil, err
		}
	}
	out, o, err := p.moveWrites(ctx, in.ID, to, name, in.By, in.Note, nil)
	if err != nil {
		return out, nil, err
	}
	after := func(ctx context.Context) {
		p.afterMove(ctx, in.ID, o, to, in.By, in.Note)
		if out.Plan.SplitFrom != "" {
			endSplits(ctx, in.By) // the demand it was split from ends with its last part
		}
	}
	return out, after, nil
}

// checkDeliverables refuses done for a request whose deliverables are not all
// verified, and for a question not yet answered. A split demand's deliverables
// are carried by its parts, never checked on it: it ends when they have
// (endSplits), whatever its own list says.
func (p *Plans) checkDeliverables(ctx context.Context, id string) error {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return err
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil || !hasRequest(o) {
		return err
	}
	if open := openFollowUps(ctx, cwm, id); len(open) > 0 {
		return invalid("Finish: %s waits on its follow-ups %s: they end first (done or dropped)", id, strings.Join(open, ", "))
	}
	f := fieldsOf(o)
	if f.Kind == api.KindQuestion && f.Answer == nil {
		return invalid("Finish: %s is a question — it is done when answered (Answer)", id)
	}
	if planState(o) == api.PlanSplit {
		return nil
	}
	for _, d := range f.Deliverables {
		if !d.Verified {
			return invalid("Finish: %s is not done — deliverable %q is not verified: run its check (requests check), correct it (requests revise), or a person verifies it by hand (requests verify)", id, d.Title)
		}
	}
	return nil
}

// Block blocks a plan with what blocks it: by hand (api.BlockManual).
func (p *Plans) Block(ctx context.Context, in api.BlockInput) (api.PlannedOutput, error) {
	return p.blockAs(ctx, in, api.BlockManual)
}

// blockAs blocks a plan for a typed cause (api.Block*): its readers choose
// its actions by the kind, never by its note's words.
func (p *Plans) blockAs(ctx context.Context, in api.BlockInput, kind string) (api.PlannedOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.PlannedOutput{}, err
	}
	if strings.TrimSpace(in.Note) == "" {
		return api.PlannedOutput{}, invalid("Block: say what blocks it (note)")
	}
	note := in.Note
	out, err := p.move(ctx, in.ID, api.PlanBlocked, "block", in.By, in.Note, func(f *api.PlanFields) { f.Note, f.Block = note, kind })
	if err != nil || kind != api.BlockManual {
		return out, err
	}
	// Blocked by hand, its work is no agent's until it is resumed: its
	// agents' to-dos wait, saying why.
	return out, settleRequest(ctx, in.ID, in.By)
}

// blockKind is why a plan is blocked; "" when it is not blocked. A blocked
// plan that names no kind was blocked by hand.
func blockKind(f api.PlanFields) string {
	if f.State != api.PlanBlocked {
		return ""
	}
	return orDefault(f.Block, api.BlockManual)
}

// move writes what changes with a move (set, nil for nothing), then makes it.
func (p *Plans) move(ctx context.Context, id, to, name string, by cwmapi.Actor, note string, set func(*api.PlanFields)) (api.PlannedOutput, error) {
	out, o, err := p.moveWrites(ctx, id, to, name, by, note, set)
	if err != nil {
		return out, err
	}
	p.afterMove(ctx, id, o, to, by, note)
	return out, nil
}

// moveWrites is move's CWM writes: what changes with it, and the move.
func (p *Plans) moveWrites(ctx context.Context, id, to, name string, by cwmapi.Actor, note string, set func(*api.PlanFields)) (api.PlannedOutput, cwmapi.Object, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PlannedOutput{}, cwmapi.Object{}, err
	}
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return api.PlannedOutput{}, cwmapi.Object{}, err
	}
	f, has := planOf(o)
	if !has {
		return api.PlannedOutput{}, cwmapi.Object{}, invalid("%s carries no plan", id)
	}
	if st := planState(o); planEnded(st) && to != api.PlanReady {
		// Ended: only filed again — a late block never writes over its end.
		return api.PlannedOutput{}, cwmapi.Object{}, invalid("%s has ended (%s)", id, st)
	}
	if set != nil {
		set(&f)
		if o, err = writePlan(ctx, cwm, o, f, true); err != nil {
			return api.PlannedOutput{}, cwmapi.Object{}, err
		}
	}
	if o, err = planTransition(ctx, cwm, id, to, name, by, note); err != nil {
		return api.PlannedOutput{}, cwmapi.Object{}, err
	}
	return api.PlannedOutput{Plan: toPlanned(o)}, o, nil
}

// afterMove is what follows a written move that ended a plan: its to-dos
// archived, its origin's end tried, what waited on it released. It reads the
// scheduler and lists objects: it runs once the move is written.
func (p *Plans) afterMove(ctx context.Context, id string, o cwmapi.Object, to string, by cwmapi.Actor, note string) {
	if !planEnded(to) {
		return
	}
	// Its work ends with it — done by hand or dropped: nothing of it stays
	// offered (a request ended by its last step has none left).
	if err := EndTodos(ctx, id, to, note); err != nil {
		slog.Warn("planification: the to-dos of an ended request were not archived", "request", id, "error", err)
	}
	// The request a follow-up was filed by may end now.
	EndOrigin(ctx, o, by)
	if !collecting(ctx) {
		// What waited on it may go now: every to-do is settled again (a result
		// being collected settles once, at its end).
		if err := settleAll(ctx, by); err != nil {
			slog.Warn("planification: the work waiting on an ended request was not released", "request", id, "error", err)
		}
	}
}

// Get reads one plan.
func (p *Plans) Get(ctx context.Context, in api.GetPlanInput) (api.PlannedOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	o, err := loadObject(ctx, cwm, in.ID)
	if err != nil {
		return api.PlannedOutput{}, err
	}
	if _, has := planOf(o); !has {
		return api.PlannedOutput{}, invalid("%s carries no plan", in.ID)
	}
	return api.PlannedOutput{Plan: toPlanned(o)}, nil
}

// List lists the plans by priority, then id.
func (p *Plans) List(ctx context.Context, in api.ListPlansInput) (api.ListPlansOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListPlansOutput{}, err
	}
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: in.Module, Capability: api.CapPlan})
	if err != nil {
		return api.ListPlansOutput{}, err
	}
	out := api.ListPlansOutput{Plans: []api.PlannedObject{}}
	for _, o := range got.Objects {
		pl := toPlanned(o)
		if in.State != "" && pl.State != in.State {
			continue
		}
		out.Plans = append(out.Plans, pl)
	}
	sortPlans(out.Plans)
	return out, nil
}

// sortPlans orders plans by priority, then id.
func sortPlans(ps []api.PlannedObject) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if ra, rb := priorityRank(a.Priority), priorityRank(b.Priority); ra != rb {
			return ra < rb
		}
		return a.ID < b.ID
	})
}

// ownState is an object's own lifecycle state — not its plan's; "" when it has
// none (a request, an object not yet moved).
func ownState(o cwmapi.Object) string {
	if l, ok := o.Lifecycle(); ok {
		return l.State
	}
	return ""
}

// loadObject reads any object by its id.
func loadObject(ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, error) {
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	return getObject(ctx, cwm, module, id)
}

// planOf is an object's plan and whether it carries one.
func planOf(o cwmapi.Object) (api.PlanFields, bool) {
	if _, ok := o.Capabilities[api.CapPlan]; !ok {
		return api.PlanFields{}, false
	}
	f, _, _ := cwmapi.DecodeCapability[api.PlanFields](o, api.CapPlan)
	return f, true
}

// planState is a plan's state; none is the workflow's initial one.
func planState(o cwmapi.Object) string {
	// Read the one field, not the whole plan and its trail: views ask every
	// request's state many times.
	if st, _ := o.Capabilities[api.CapPlan]["state"].(string); st != "" {
		return st
	}
	return api.PlanIdle
}

func planEnded(state string) bool { return state == api.PlanDone || state == api.PlanDropped }

// writePlan writes a plan's fields: adding the capability when the object has
// none (new), updating it otherwise. The state is left to the transitions.
func writePlan(ctx context.Context, cwm cwmapi.CWMService, o cwmapi.Object, f api.PlanFields, has bool) (cwmapi.Object, error) {
	if !has {
		f.State, f.Trail = "", nil
		fields, err := capabilityMap(f)
		if err != nil {
			return cwmapi.Object{}, err
		}
		out, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: o.ID, Capability: api.CapPlan, Fields: fields})
		if err != nil {
			return cwmapi.Object{}, err
		}
		return out.Object, nil
	}
	if err := o.SetCapability(api.CapPlan, f); err != nil {
		return cwmapi.Object{}, err
	}
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	out, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: o})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

// planTransition moves a plan by its workflow.
func planTransition(ctx context.Context, cwm cwmapi.CWMService, id, to, name string, by cwmapi.Actor, note string) (cwmapi.Object, error) {
	out, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, Transition: name, By: by, Note: note, Capability: api.CapPlan})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return out.Object, nil
}

func toPlanned(o cwmapi.Object) api.PlannedObject {
	module, _ := cwmapi.ModuleOfObjectID(o.ID)
	f, _ := planOf(o)
	f.State = planState(o)
	return api.PlannedObject{
		ID: o.ID, Type: o.Type, Module: module, Title: titleOf(o),
		DependsOn: dependsOn(o), SplitFrom: o.EdgeTo(api.EdgeSplitFrom), FollowsUp: o.EdgeTo(api.EdgeFollowsUp), PlanFields: f,
	}
}
