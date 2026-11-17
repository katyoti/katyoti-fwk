// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// PLANNING PUTS TO-DOS — a request is expanded into its to-dos, one per step of
// its work template — intake first, unless it was assessed — each put in its
// module's queue (the scheduler's) with its work packet: what to do, copied
// from the template; the to-do is transient, the request and its object keep
// everything that lasts. A to-do waits until planification releases it (Settle).

// Work implements api.WorkService over the scheduler.
type Work struct{}

var _ api.WorkService = (*Work)(nil)

// NewWork returns planification's work service.
func NewWork() *Work { return &Work{} }

// PlanningActor is who planification acts as when no one asked it.
var PlanningActor = cwmapi.Actor{ID: api.Owner, Kind: "agent"}

// intakeInstructions is what the intake to-do does.
const intakeInstructions = "Evaluate the request: route it (or split it across modules), assess its kind, priority, estimate and depth, order it after what it waits on; when it is unclear, ask its requester one question."

// sched resolves the scheduler.
func sched(ctx context.Context) (schedapi.TodoService, error) {
	s, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return nil, fmt.Errorf("the scheduler: %w", err)
	}
	return s, nil
}

// decode reads a to-do's work packet.
func decode(t schedapi.Todo) api.WorkTodo {
	var p api.WorkPacket
	if raw, err := json.Marshal(t.Work); err == nil {
		_ = json.Unmarshal(raw, &p)
	}
	return api.WorkTodo{Todo: t, Packet: p}
}

// encode writes a work packet into a to-do.
func encode(t *schedapi.Todo, p api.WorkPacket) {
	raw, _ := json.Marshal(p)
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	t.Work = m
}

// put writes a to-do and answers it read back.
func put(ctx context.Context, w api.WorkTodo) (api.WorkTodo, error) {
	s, err := sched(ctx)
	if err != nil {
		return api.WorkTodo{}, err
	}
	t := w.Todo
	encode(&t, w.Packet)
	t.Owner = api.Owner
	// A new to-do is put in its request's project: its module qualified by
	// the request's key (several projects served), its Request and Target
	// bare - a row keeps references of its own.
	if t.ID == "" {
		t.Module = moduleIn(t.Request, t.Module)
	}
	t.Request, t.Target = bareID(t.Request), bareID(t.Target)
	// Its reads and writes are modules of its own project: the scheduler
	// locks them beside its Repository.
	t.Reads, t.Writes = bareModules(t.Reads), bareModules(t.Writes)
	out, err := s.Put(ctx, schedapi.PutInput{Todo: t})
	if err != nil {
		return api.WorkTodo{}, err
	}
	return decode(out.Todo), nil
}

// todosOf lists the scheduled to-dos (of a request, or all).
func todosOf(ctx context.Context, request string) ([]api.WorkTodo, error) {
	if c, ok := ctx.Value(readCacheKey{}).(*readCache); ok {
		return c.todosOf(ctx, request)
	}
	return listTodos(ctx, request)
}

// listTodos lists the to-dos of planification — of one request, or all — from
// the scheduler.
func listTodos(ctx context.Context, request string) ([]api.WorkTodo, error) {
	schedulerReads.Add(1)
	s, err := sched(ctx)
	if err != nil {
		return nil, err
	}
	in := schedapi.ListInput{}
	if request != "" {
		// The request's to-dos in its project: its module qualified when
		// several projects are served, its id bare as the rows keep it.
		ref, err := requestRefOf(ctx, request)
		if err != nil {
			return nil, err
		}
		in.Request = bareID(ref)
		if module, err := requestModule(ref); err == nil {
			in.Module = module
		}
	}
	got, err := s.List(ctx, in)
	if err != nil {
		return nil, err
	}
	return planTodos(got.Todos), nil
}

// planTodos are planification's own among the scheduler's to-dos, decoded, by
// request then position.
func planTodos(todos []schedapi.Todo) []api.WorkTodo {
	var out []api.WorkTodo
	for _, t := range todos {
		if t.Owner == api.Owner {
			out = append(out, decode(t))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Request != out[j].Request {
			return out[i].Request < out[j].Request
		}
		return out[i].Position < out[j].Position
	})
	return out
}

// scheduledTodos are every to-do the scheduler holds, whoever owns them -
// listed once for a view carrying the read cache: planification's own and the
// inbox's human tasks (an open intake question) are read from the same
// listing.
func scheduledTodos(ctx context.Context) ([]schedapi.Todo, error) {
	c, ok := ctx.Value(readCacheKey{}).(*readCache)
	if !ok {
		return listScheduled(ctx)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.allListed {
		c.all, c.allErr = listScheduled(ctx)
		c.allListed = true
	}
	return c.all, c.allErr
}

// listScheduled lists every to-do of every module from the scheduler.
func listScheduled(ctx context.Context) ([]schedapi.Todo, error) {
	schedulerReads.Add(1)
	s, err := sched(ctx)
	if err != nil {
		return nil, err
	}
	got, err := s.List(ctx, schedapi.ListInput{})
	return got.Todos, err
}

// doneOf lists a request's archived to-dos.
func doneOf(ctx context.Context, request string) ([]api.WorkTodo, error) {
	s, err := sched(ctx)
	if err != nil {
		return nil, err
	}
	ref := requestRef(ctx, request)
	module, err := requestModule(ref)
	if err != nil {
		return nil, err
	}
	got, err := s.List(ctx, schedapi.ListInput{Module: module, Request: bareID(ref), Done: true})
	if err != nil {
		return nil, err
	}
	var out []api.WorkTodo
	for _, t := range got.Todos {
		if t.Owner == api.Owner {
			out = append(out, decode(t))
		}
	}
	return out, nil
}

// getTodo reads one to-do.
func getTodo(ctx context.Context, id string) (api.WorkTodo, error) {
	s, err := sched(ctx)
	if err != nil {
		return api.WorkTodo{}, err
	}
	got, err := s.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil {
		return api.WorkTodo{}, err
	}
	return decode(got.Todo), nil
}

// archive moves a to-do to done.
func archive(ctx context.Context, id, note string) error {
	s, err := sched(ctx)
	if err != nil {
		return err
	}
	_, err = s.Archive(ctx, schedapi.ArchiveInput{ID: id, Note: note})
	if err != nil {
		// Archived already (a collect run again past its archive): done
		//
		if got, gerr := s.Get(ctx, schedapi.GetInput{ID: id}); gerr == nil && got.Todo.Archived != "" {
			return nil
		}
	}
	return err
}

// EndTodos archives every to-do of a request that ended — done by hand or
// dropped — not started, the one that moves its node included, with the end's
// note. A pulled one stays with its worker; its report is collected as nothing.
func EndTodos(ctx context.Context, request, outcome, note string) error {
	settling.Lock() // a settle running meanwhile would write one back
	defer settling.Unlock()
	// A request filed again since it ended - a realized feature's
	// verification filed on it as its build ends -
	// keeps its new work: an end heard late never archives it.
	if cwm, err := cwmOf(ctx); err == nil {
		if o, err := loadObject(ctx, cwm, request); err == nil && !planEnded(planState(o)) {
			return nil
		}
	}
	todos, err := todosOf(ctx, request)
	if err != nil {
		return err
	}
	why := "its request was dropped"
	if outcome == api.PlanDone {
		why = "its request ended done"
	}
	if note != "" {
		why += ": " + firstLine(note)
	}
	// Each read again just before: one pulled or archived meanwhile is left
	// alone, and one refused does not keep the others in the queue.
	var errs []error
	for _, t := range todos {
		fresh, err := getTodo(ctx, t.ID)
		if errors.Is(err, cwmapi.ErrNotFound) {
			continue // archived meanwhile
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", t.ID, err))
			continue
		}
		if !notStarted(fresh) {
			continue
		}
		if err := archive(ctx, t.ID, why); err != nil {
			errs = append(errs, fmt.Errorf("archive %s: %w", t.ID, err))
			continue
		}
		closeAsked(ctx, fresh, why)
	}
	return errors.Join(errs...)
}

// notStarted reports a to-do its owner may still change: waiting or ready.
func notStarted(t api.WorkTodo) bool {
	return t.State == schedapi.StateWaiting || t.State == schedapi.StateReady
}

// currentTodo is the open to-do a request is on: the first in position
// order; nil when none is open.
func currentTodo(ctx context.Context, request string) *api.WorkTodo {
	todos, err := todosOf(ctx, request)
	if err != nil || len(todos) == 0 {
		return nil
	}
	first := 0
	for i := range todos {
		if todos[i].Position < todos[first].Position {
			first = i
		}
	}
	return &todos[first]
}

// stepRuns says whether a step of a request runs now: one of its to-dos is
// claimed (pulled by a worker) or reported and not yet collected. Running is
// read from the to-dos, never stored on the plan.
func stepRuns(ctx context.Context, request string) bool {
	todos, err := todosOf(ctx, request)
	if err != nil {
		return false
	}
	for _, t := range todos {
		if t.State == schedapi.StateClaimed || t.State == schedapi.StateReported {
			return true
		}
	}
	return false
}

// Plan expands a request into its to-dos.
func (w *Work) Plan(ctx context.Context, in api.PlanWorkInput) (api.WorkOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.WorkOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.WorkOutput{}, err
	}
	// The request in its project: qualified when several projects are
	// served, so every read, write and to-do below is that project's.
	rid := requestRef(ctx, in.Request)
	o, err := load(ctx, cwm, rid)
	if err != nil {
		return api.WorkOutput{}, err
	}
	if !hasRequest(o) {
		return api.WorkOutput{}, invalid("Plan: %s carries no request", in.Request)
	}
	if nodeEnded(o) {
		return api.WorkOutput{}, invalid("Plan: %s has ended (%s)", in.Request, stateOf(o))
	}
	if stateOf(o) == api.PlanSplit {
		return api.WorkOutput{}, invalid("Plan: %s is split: its parts carry its work", in.Request)
	}
	existing, err := todosOf(ctx, rid)
	if err != nil {
		return api.WorkOutput{}, err
	}
	if _, has := planOf(o); !has {
		// A request with no plan - its filing lost it between its two writes -
		// gets one first: what each step comes to is recorded on it, and its
		// end can be written.
		if o, err = planIntake(ctx, cwm, rid, in.By, "planned: it carried no plan"); err != nil {
			return api.WorkOutput{}, err
		}
		if len(existing) == 0 && !in.Again {
			if ran, err := recordRan(ctx, cwm, rid, in.By); err != nil || ran {
				return api.WorkOutput{Todos: []api.WorkTodo{}}, err
			}
		}
	}
	if len(existing) > 0 {
		if !in.Again {
			return api.WorkOutput{}, invalid("Plan: %s is already planned (%d to-dos) — plan it again with again", rid, len(existing))
		}
		for _, t := range existing {
			if notStarted(t) {
				if err := archive(ctx, t.ID, "planned again"); err != nil {
					return api.WorkOutput{}, err
				}
			}
		}
	}
	tmpl, err := templateFor(ctx, o)
	if err != nil {
		return api.WorkOutput{}, err
	}
	req := fieldsOf(o)
	priority := req.Priority
	if priority == "" {
		priority = api.DefaultPriority
	}
	module, _ := cwmapi.ModuleOfObjectID(bareID(rid)) // put names it in the request's project

	type step struct {
		name          string
		after         []string
		reads, writes []string
		skill         string
		packet        api.WorkPacket
	}
	var steps []step
	if !in.NoIntake {
		steps = append(steps, step{name: api.StepIntake, reads: []string{module}, packet: api.WorkPacket{
			Doer: api.DoerTask, Task: IntakeTask, Phase: api.PhaseRequests, Instructions: intakeInstructions,
		}})
	}
	for _, st := range tmpl.Steps {
		ps, err := resolveStep(ctx, o, tmpl, st)
		if err != nil {
			return api.WorkOutput{}, err
		}
		after := st.After
		if len(after) == 0 && !in.NoIntake {
			after = []string{api.StepIntake}
		}
		steps = append(steps, step{name: st.Name, after: after, reads: ps.Reads, writes: ps.Writes, skill: st.Skill, packet: api.WorkPacket{
			Template: tmpl.Name, Doer: ps.Doer, Task: ps.Task, Interaction: ps.Interaction, Gate: ps.Gate, Phase: ps.Phase,
			Confirms: st.Confirms, Verifies: verifiedBy(st, req.Deliverables), Instructions: st.Instructions, Refines: st.Refines,
			EndsOn: st.EndsOn,
		}})
	}
	// What its end does to the node: the template's start on its first step,
	// its transition on its last.
	first, last := 0, len(steps)-1
	if !in.NoIntake {
		first = 1
	}
	if first <= last {
		steps[first].packet.Starts = tmpl.Start
		steps[last].packet.Moves = tmpl.Transition
		if e := steps[last].packet.EndsOn; e != "" {
			// The target moved itself: its end is the state it reached.
			steps[last].packet.Moves = e
		}
	}

	ids := map[string]string{}
	var out api.WorkOutput
	base := len(existing)
	for i, s := range steps {
		p := s.packet
		p.Step = s.name
		var after []string
		for _, a := range s.after {
			id, ok := ids[a]
			if !ok {
				return api.WorkOutput{}, invalid("Plan: template %s: step %s comes after %q, which is not before it", tmpl.Name, s.name, a)
			}
			after = append(after, id)
		}
		saved, err := put(ctx, api.WorkTodo{Todo: schedapi.Todo{
			Module: module, Target: rid, Request: rid, Title: s.name + " — " + titleOf(o),
			For: forOf(p.Doer), Skill: s.skill, Group: api.PlanGroup, Start: startOf(p.Doer),
			Priority: priority, State: schedapi.StateWaiting, Since: req.FiledAt, Position: base + i + 1,
			After: after, Reads: s.reads, Writes: s.writes,
		}, Packet: p})
		if err != nil {
			return api.WorkOutput{}, fmt.Errorf("Plan: put the %s to-do: %w", s.name, err)
		}
		ids[s.name] = saved.ID
		out.Todos = append(out.Todos, saved)
	}
	if tmpl.Transition == api.StateDone {
		sayEndOutOfReach(ctx, cwm, rid)
	}
	if err := settleRequest(ctx, rid, in.By); err != nil {
		return api.WorkOutput{}, err
	}
	got, err := todosOf(ctx, rid)
	if err != nil {
		return api.WorkOutput{}, err
	}
	out.Todos = got
	return out, nil
}

// recordRan records on a request just given its plan the steps its work ran
// without one - its archived to-dos, in the order they ended - then tries its
// end: its work is not planned again. It answers whether any had run.
func recordRan(ctx context.Context, cwm cwmapi.CWMService, request string, by cwmapi.Actor) (bool, error) {
	done, err := doneOf(ctx, request)
	if err != nil {
		return false, err
	}
	var ran []api.WorkTodo
	for _, t := range done {
		if t.Result != nil {
			ran = append(ran, t)
		}
	}
	if len(ran) == 0 {
		return false, nil
	}
	sort.SliceStable(ran, func(i, j int) bool { return ran[i].Result.At < ran[j].Result.At })
	for _, t := range ran {
		if err := record(ctx, cwm, t); err != nil {
			return true, err
		}
	}
	return true, endRequest(ctx, cwm, request, by)
}

// outOfReachNote is the start of the note sayEndOutOfReach writes.
const outOfReachNote = "its feature is "

// sayEndOutOfReach tells, as a feature's build is planned, that its workflow
// has no way to done from where the feature stands: the plan's note says so at
// once - its end records it realized, for a person to confirm (moveEnded) -
// never only after the work. A feature already done has nothing to reach: no
// note is written, and one written earlier is cleared.
func sayEndOutOfReach(ctx context.Context, cwm cwmapi.CWMService, id string) {
	o, err := loadObject(ctx, cwm, id)
	if err != nil || o.Type != featureType {
		return
	}
	st := ownState(o)
	if st == api.StateDone {
		clearEndOutOfReach(ctx, cwm, id)
		return
	}
	_, terr := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: api.StateDone, By: PlanningActor, DryRun: true})
	if st != "" && (terr == nil || !strings.Contains(terr.Error(), "no transition")) {
		return // done is reachable (its gates are the build's own work)
	}
	if st == "" {
		st = "no state yet"
	}
	o, err = load(ctx, cwm, id)
	if err != nil {
		return
	}
	f, has := planOf(o)
	f.Note = outOfReachNote + st + ": its workflow has no way to done from there - this build ends with it recorded realized, for a person to confirm (or accept and specify it first)"
	if _, err := writePlan(ctx, cwm, o, f, has); err != nil {
		slog.Warn("planification: a build's out-of-reach end was not noted on its plan", "request", id, "error", err)
	}
}

// clearEndOutOfReach clears the out-of-reach note a plan carries from before
// its feature was done; any other note stays.
func clearEndOutOfReach(ctx context.Context, cwm cwmapi.CWMService, id string) {
	o, err := load(ctx, cwm, id)
	if err != nil {
		return
	}
	f, has := planOf(o)
	if !has || !strings.HasPrefix(f.Note, outOfReachNote) {
		return
	}
	f.Note = ""
	if _, err := writePlan(ctx, cwm, o, f, has); err != nil {
		slog.Warn("planification: a done feature's out-of-reach note was not cleared", "request", id, "error", err)
	}
}

// List lists a request's or a module's to-dos with their packets.
func (w *Work) List(ctx context.Context, in api.ListWorkInput) (api.WorkOutput, error) {
	all, err := todosOf(ctx, in.Request)
	if err != nil {
		return api.WorkOutput{}, err
	}
	out := api.WorkOutput{Todos: []api.WorkTodo{}}
	for _, t := range all {
		if (in.Module != "" && t.Module != in.Module) || (in.State != "" && t.State != in.State) {
			continue
		}
		out.Todos = append(out.Todos, t)
	}
	return out, nil
}

// forOf is who a doer is: a person, or an agent (a task or a persona).
func forOf(doer string) string {
	if doer == api.DoerPerson {
		return schedapi.ForPerson
	}
	return schedapi.ForAgent
}

// startOf is how a step starts: a person's by hand, an agent's by itself where
// its module is auto.
func startOf(doer string) string {
	if doer == api.DoerPerson {
		return schedapi.StartManual
	}
	return schedapi.StartAuto
}

// moveNode moves a node with a lifecycle of its own to a state, from where it
// is; a node without one, or already there, is left as it is.
func moveNode(ctx context.Context, cwm cwmapi.CWMService, id, to string, by cwmapi.Actor, note string) error {
	o, err := loadObject(ctx, cwm, id)
	if err != nil {
		return err
	}
	if st := ownState(o); st == "" || st == to || decided(o) {
		return nil // a decided decision's end is its own workflow's: it is never asked to become done
	}
	_, err = cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: to, By: by, Note: note})
	return err
}

// moduleTree is each user module's containing module (the system modules
// hold no work).
func moduleTree(ctx context.Context) map[string]string {
	out := map[string]string{}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return out
	}
	mods, err := cwmapi.UserModules(ctx, cwm)
	if err != nil {
		return out
	}
	for _, o := range mods {
		if o.Parent != "" {
			out[o.ID] = o.Parent
		}
	}
	return out
}

// READ ONCE — a view over the work (the planning deck, the request panel)
// reads the scheduler once: every part of it asks the same listing and the
// same queue, answered from one read for the view's duration. Only read-only
// views carry it; anything that writes reads afresh.

// schedulerReads counts the listings and queues planification read from the
// scheduler; SchedulerReads reads it (what a test of a view's reads asks).
var schedulerReads atomic.Int64

// SchedulerReads is how many times planification read the scheduler's to-dos
// or queue so far.
func SchedulerReads() int64 { return schedulerReads.Load() }

type readCacheKey struct{}

type readCache struct {
	mu        sync.Mutex
	todos     []api.WorkTodo
	todosErr  error
	listed    bool
	all       []schedapi.Todo // every owner's, listed once (scheduledTodos)
	allErr    error
	allListed bool
	queues    map[string]queueRead     // by module ("" every module)
	objects   map[string]objectRead    // by id: each object read once for the view
	demands   map[string]demandsRead   // by input: the demands read once for the view
	incoming  map[string]incomingEdges // by object and edge kind
	takers    map[string][]string      // every takes edge, by the object taken (takersOf)
	repos     map[string]string        // project uuid by key, resolved once (repositoryOfKey)
	summaries map[string]summaryRead   // by increment: each summary counted once (Summary)
	// hold is the one reader of the view (viewReader), built once when
	// first asked; holdMu serializes it (it memoizes as it reads).
	holdMu    sync.Mutex
	hold      *holdReader
	holdBuilt bool
	// holdKeys is the projects holding a holds file (holdProjects).
	holdKeys     []string
	holdKeysRead bool
}

type objectRead struct {
	o       cwmapi.Object
	project string // the project holding it when several are served (ObjectGetOutput.Project)
	err     error
}

// getObject reads one object - once for a view carrying the read cache: a
// view asks the same request, increment or release many times. What it
// answers is only read (the view writes nothing).
func getObject(ctx context.Context, cwm cwmapi.CWMService, module, id string) (cwmapi.Object, error) {
	r := readObject(ctx, cwm, module, id)
	return r.o, r.err
}

// readObject is getObject's read, with the project the CWM names holding
// the object.
func readObject(ctx context.Context, cwm cwmapi.CWMService, module, id string) objectRead {
	c, ok := ctx.Value(readCacheKey{}).(*readCache)
	if !ok {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
		return objectRead{o: inProject(got.Object, id), project: got.Project, err: err}
	}
	c.mu.Lock()
	r, hit := c.objects[id]
	c.mu.Unlock()
	if hit {
		return r
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	r = objectRead{o: inProject(got.Object, id), project: got.Project, err: err}
	c.mu.Lock()
	if c.objects == nil {
		c.objects = map[string]objectRead{}
	}
	c.objects[id] = r
	c.mu.Unlock()
	return r
}

type queueRead struct {
	out schedapi.QueueOutput
	err error
}

// withReadCache is ctx reading the scheduler once for a read-only view.
func withReadCache(ctx context.Context) context.Context {
	if _, ok := ctx.Value(readCacheKey{}).(*readCache); ok {
		return ctx
	}
	return context.WithValue(ctx, readCacheKey{}, &readCache{})
}

func (c *readCache) todosOf(ctx context.Context, request string) ([]api.WorkTodo, error) {
	raw, rawErr := scheduledTodos(ctx)
	c.mu.Lock()
	if !c.listed {
		c.todos, c.todosErr = planTodos(raw), rawErr
		c.listed = true
	}
	all, err := c.todos, c.todosErr
	c.mu.Unlock()
	if err != nil || request == "" {
		return append([]api.WorkTodo(nil), all...), err
	}
	ref, err := requestRefOf(ctx, request)
	if err != nil {
		return nil, err // a bare id several projects hold: named, never both projects' rows
	}
	of := requestFilter(ctx, ref)
	var out []api.WorkTodo
	for _, t := range all {
		if of(t) {
			out = append(out, t)
		}
	}
	return out, nil
}

// queueOf is the scheduler's queue of the ready to-dos in pull order, of one
// module or every one — read once for a view carrying the read cache.
func queueOf(ctx context.Context, module string) (schedapi.QueueOutput, error) {
	s, err := sched(ctx)
	if err != nil {
		return schedapi.QueueOutput{}, err
	}
	c, ok := ctx.Value(readCacheKey{}).(*readCache)
	if !ok {
		schedulerReads.Add(1)
		return s.Queue(ctx, schedapi.Filter{Module: module})
	}
	// One read per filter: a to-do's takeability depends on what the queue was
	// asked for (a module's queue weighs only its own candidates), so a
	// module's view reads that module's queue — what a pull on it sees.
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.queues == nil {
		c.queues = map[string]queueRead{}
	}
	r, ok := c.queues[module]
	if !ok {
		schedulerReads.Add(1)
		r.out, r.err = s.Queue(ctx, schedapi.Filter{Module: module})
		c.queues[module] = r
	}
	return schedapi.QueueOutput{Queue: append([]schedapi.QueueEntry(nil), r.out.Queue...), TakersSince: r.out.TakersSince}, r.err
}
