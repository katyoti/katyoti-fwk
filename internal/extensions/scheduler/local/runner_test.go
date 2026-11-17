// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedcore "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/core"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// THE INTERNAL SCHEDULER'S WORLD — the scheduler and a fake task service that
// records the runs started; a test sets how each ends.

type fakeTasks struct {
	mu      sync.Mutex
	started []taskapi.TaskRunRequest
	status  map[string]taskapi.RunStatus
	groups  []taskapi.GroupLoad
	active  []taskapi.RunInfo
	refuse  string
}

func (f *fakeTasks) Run(_ context.Context, in taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refuse != "" {
		return taskapi.TaskRunResponse{}, errors.New(f.refuse)
	}
	f.started = append(f.started, in)
	id := fmt.Sprintf("run-%d", len(f.started))
	f.status[id] = taskapi.RunStatusRunning
	return taskapi.TaskRunResponse{RunID: id, TaskName: in.TaskName, Status: taskapi.RunStatusRunning, SessionID: "session-" + id}, nil
}

func (f *fakeTasks) Status(_ context.Context, in taskapi.StatusRequest) (taskapi.StatusResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.status[in.RunID]
	if !ok {
		return taskapi.StatusResponse{}, errors.New("no run " + in.RunID)
	}
	out := taskapi.StatusResponse{RunID: in.RunID, Status: st}
	for _, r := range f.active {
		if r.RunID == in.RunID {
			out.StartedAt = r.StartedAt
		}
	}
	return out, nil
}

func (f *fakeTasks) runs() []taskapi.TaskRunRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]taskapi.TaskRunRequest{}, f.started...)
}

// work is the work-todo runs started, and their ids.
func (f *fakeTasks) work() ([]taskapi.TaskRunRequest, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var runs []taskapi.TaskRunRequest
	var ids []string
	for i, r := range f.started {
		if r.TaskName == api.WorkTodoTask {
			runs, ids = append(runs, r), append(ids, fmt.Sprintf("run-%d", i+1))
		}
	}
	return runs, ids
}

func (f *fakeTasks) end(run string, st taskapi.RunStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status[run] = st
}

func (f *fakeTasks) Result(context.Context, taskapi.ResultRequest) (taskapi.ResultResponse, error) {
	return taskapi.ResultResponse{}, nil
}
func (f *fakeTasks) Stop(context.Context, taskapi.StopRequest) (taskapi.StopResponse, error) {
	return taskapi.StopResponse{}, nil
}
func (f *fakeTasks) RunStages(context.Context, taskapi.RunStagesRequest) (taskapi.RunStagesResponse, error) {
	return taskapi.RunStagesResponse{}, nil
}
func (f *fakeTasks) Runs(context.Context, taskapi.ListRunsRequest) (taskapi.ListRunsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return taskapi.ListRunsResponse{Runs: append([]taskapi.RunInfo{}, f.active...)}, nil
}
func (f *fakeTasks) List(context.Context, taskapi.ListTasksRequest) (taskapi.ListTasksResponse, error) {
	return taskapi.ListTasksResponse{}, nil
}
func (f *fakeTasks) Get(_ context.Context, in taskapi.GetTaskRequest) (taskapi.TaskInfo, error) {
	return taskapi.TaskInfo{Name: in.Name}, nil
}
func (f *fakeTasks) BackgroundManager() taskapi.BackgroundTaskManager { return nil }
func (f *fakeTasks) Repeat(context.Context, taskapi.RepeatRequest) (taskapi.TaskRunResponse, error) {
	return taskapi.TaskRunResponse{}, nil
}
func (f *fakeTasks) SuspensionManager() taskapi.SuspensionManager { return nil }
func (f *fakeTasks) Groups(context.Context, taskapi.GroupsRequest) (taskapi.GroupsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return taskapi.GroupsResponse{Groups: append([]taskapi.GroupLoad{}, f.groups...)}, nil
}
func (f *fakeTasks) SetGroup(_ context.Context, in taskapi.SetGroupRequest) (taskapi.GroupLoad, error) {
	return taskapi.GroupLoad{Name: in.Group}, nil
}
func (f *fakeTasks) ClearGroup(context.Context, taskapi.ClearGroupRequest) (taskapi.ClearGroupResponse, error) {
	return taskapi.ClearGroupResponse{}, nil
}

func runnerWorld(t *testing.T) (context.Context, api.TodoService, *fakeTasks, *collected) {
	t.Helper()
	tasks := &fakeTasks{status: map[string]taskapi.RunStatus{}}
	ctx, _, svc, got := world(t, application.DeclareSimpleService[taskapi.TaskService](func(context.Context) (taskapi.TaskService, error) {
		return tasks, nil
	}))
	return ctx, svc, tasks, got
}

func mode(t *testing.T, ctx context.Context, svc api.TodoService, module, m string) {
	t.Helper()
	if _, err := svc.Mode(ctx, api.ModeInput{Module: module, Mode: m, By: person}); err != nil {
		t.Fatal(err)
	}
}

// The internal scheduler runs the auto agent to-dos of the auto modules — no
// manual start, no person's, nothing of a manual module — each pulled for the
// work-todo task's run, which is recorded on its claim; a run that ends without
// a report is reported failed for it, and its owner collects that.
func TestTheInternalSchedulerRunsTheAutoToDos(t *testing.T) {
	ctx, svc, tasks, got := runnerWorld(t)
	mode(t, ctx, svc, "billing", api.ModeAuto)
	auto := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", Group: "build", Priority: "P1", Target: "billing.RQ-001"})
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "by hand", Start: api.StartManual})
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", For: api.ForPerson})
	put(t, ctx, svc, api.Todo{Module: "shipping", Title: "pack"})

	rep, err := svc.Tick(ctx, api.TickInput{})
	if err != nil {
		t.Fatal(err)
	}
	runs, ids := tasks.work()
	if len(rep.Started) != 1 || len(runs) != 1 {
		t.Fatalf("started %v (runs %+v); want the auto implement alone", rep.Started, runs)
	}
	r := runs[0]
	if r.TaskName != api.WorkTodoTask || r.Input["todo"] != auto.ID || r.Group != "build" || r.Target != "billing.RQ-001" || r.Priority != 2 {
		t.Errorf("the run: %+v; want work-todo on the implement, in its group, on its target, at P1's priority", r)
	}
	held, _ := svc.Get(ctx, api.GetInput{ID: auto.ID})
	if c := held.Todo.Claim; held.Todo.State != api.StateClaimed || c == nil || c.By != api.Worker || c.Run != ids[0] || c.Session != "session-"+ids[0] {
		t.Fatalf("the implement once started: %+v; want held by work-todo, its run and session recorded", held.Todo)
	}

	if again, _ := svc.Tick(ctx, api.TickInput{}); len(again.Started)+len(again.GivenBack) != 0 {
		t.Errorf("a second tick while it runs: %+v; want nothing", again)
	}
	tasks.end(ids[0], taskapi.RunStatusCompleted)
	back, err := svc.Tick(ctx, api.TickInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.GivenBack) != 1 || !strings.HasPrefix(back.GivenBack[0], auto.ID) {
		t.Fatalf("given back %v; want the implement", back.GivenBack)
	}
	failed, _ := svc.Get(ctx, api.GetInput{ID: auto.ID})
	if res := failed.Todo.Result; res == nil || res.Status != api.StatusFail || !strings.Contains(res.Summary, "without a report") {
		t.Errorf("the implement given back: %+v; want reported failed, saying why", failed.Todo.Result)
	}
	if ids := got.seen(); len(ids) != 1 || ids[0] != auto.ID {
		t.Errorf("collected %v; want the implement", ids)
	}
}

// A task group's window bounds what starts: a paused group starts nothing and
// says so; a group of one starts one.
func TestTheInternalSchedulerKeepsToItsGroupsWindows(t *testing.T) {
	ctx, svc, tasks, _ := runnerWorld(t)
	mode(t, ctx, svc, "billing", api.ModeAuto)
	mode(t, ctx, svc, "shipping", api.ModeAuto)
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "one"})
	put(t, ctx, svc, api.Todo{Module: "shipping", Title: "two"})

	tasks.groups = []taskapi.GroupLoad{{Name: api.DefaultGroup, On: false, MaxRunning: 2}}
	rep, _ := svc.Tick(ctx, api.TickInput{})
	if len(rep.Started) != 0 || len(rep.Waiting) != 2 || !strings.Contains(rep.Waiting[0], "paused") {
		t.Fatalf("a paused group: %+v; want nothing started, both waiting", rep)
	}
	tasks.groups = []taskapi.GroupLoad{{Name: api.DefaultGroup, On: true, MaxRunning: 1}}
	rep, _ = svc.Tick(ctx, api.TickInput{})
	if len(rep.Started) != 1 || len(rep.Waiting) != 1 {
		t.Fatalf("a group of one: %+v; want one started, one waiting", rep)
	}
}

// Run works a ready agent to-do now, in any mode but off; a person's is
// refused; a run that does not start gives the to-do back.
func TestRunWorksOneToDoNow(t *testing.T) {
	ctx, svc, tasks, _ := runnerWorld(t)
	manual := put(t, ctx, svc, api.Todo{Module: "billing", Title: "by hand", Start: api.StartManual})
	ask := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", For: api.ForPerson})
	if _, err := svc.Run(ctx, api.RunInput{ID: ask.ID, By: person}); err == nil {
		t.Error("a person's to-do was run")
	}
	tasks.refuse = "no such task"
	if _, err := svc.Run(ctx, api.RunInput{ID: manual.ID, By: person}); err == nil {
		t.Fatal("a run that did not start was accepted")
	}
	if back, _ := svc.Get(ctx, api.GetInput{ID: manual.ID}); back.Todo.State != api.StateReady {
		t.Fatalf("after a run that did not start: %s; want ready again", back.Todo.State)
	}
	tasks.refuse = ""
	out, err := svc.Run(ctx, api.RunInput{ID: manual.ID, By: person})
	if err != nil || out.Todo.Claim == nil || out.Todo.Claim.Run != "run-1" {
		t.Fatalf("Run: %+v, %v; want it held by its run", out.Todo, err)
	}
	if r := tasks.runs(); r[0].Origin == nil || r[0].Origin.Kind != taskapi.OriginPerson {
		t.Errorf("the run's origin: %+v; want the person who asked", r[0].Origin)
	}
	mode(t, ctx, svc, "shipping", api.ModeOff)
	off := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "pack"})
	if _, err := svc.Run(ctx, api.RunInput{ID: off.ID, By: person}); err == nil {
		t.Error("a to-do of a module off was run")
	}
}

// Setting a module auto starts the internal scheduler's run, once.
func TestAnAutoModuleStartsTheInternalScheduler(t *testing.T) {
	ctx, svc, tasks, _ := runnerWorld(t)
	mode(t, ctx, svc, "billing", api.ModeManual)
	if len(tasks.runs()) != 0 {
		t.Fatalf("a manual module started %+v", tasks.runs())
	}
	mode(t, ctx, svc, "billing", api.ModeAuto)
	runs := tasks.runs()
	if len(runs) != 1 || runs[0].TaskDef["name"] != api.SchedulerTask {
		t.Fatalf("runs %+v; want the internal scheduler's", runs)
	}
	tasks.active = []taskapi.RunInfo{{RunID: "run-1", TaskName: api.SchedulerTask}}
	mode(t, ctx, svc, "shipping", api.ModeAuto)
	if len(tasks.runs()) != 1 {
		t.Errorf("a second auto module started another scheduler: %+v", tasks.runs())
	}
}

// The internal scheduler's machine ticks, then waits for a move or its
// interval, rechecked on resume; with nothing to run it ends; beside an older
// run it stops.
func TestTheInternalSchedulersMachine(t *testing.T) {
	ctx, svc, tasks, _ := runnerWorld(t)
	_, create := schedcore.MachineStrategyDef.Strategy.NewTypeConfig()
	m, err := create(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mach, err := m.CreateMachine(ctx, &taskapi.TaskExecRequest{Config: map[string]any{"interval_seconds": 7}})
	if err != nil {
		t.Fatal(err)
	}
	st := mach.InitialState(engine.Event{})
	step := func(self string) *engine.StepResult {
		t.Helper()
		c := engine.WithRunID(ctx, self)
		action, err := mach.Step(c, &st, engine.Event{})
		if err != nil {
			t.Fatal(err)
		}
		res, err := action(c)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	if res := step("run-9"); res.NextState != "done" {
		t.Fatalf("no module auto: %+v; want done", res)
	}
	mode(t, ctx, svc, "billing", api.ModeAuto)
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement"})
	res := step("run-9")
	if res.NextState != "tick" || res.Wait == nil || res.Wait.Timeout != 7*time.Second || !res.Wait.RecheckOnResume ||
		len(res.Wait.Conditions) != 1 || res.Wait.Conditions[0].Topic != engine.PlanChangedTopic() || len(res.Emit) != 1 {
		t.Fatalf("a step: %+v; want a tick that started one, then a wait on a move or 7s", res)
	}
	now := time.Now()
	tasks.mu.Lock()
	tasks.status["run-9"] = taskapi.RunStatusRunning
	tasks.active = []taskapi.RunInfo{{RunID: "run-0", TaskName: api.SchedulerTask, StartedAt: now.Add(-time.Minute)}, {RunID: "run-9", TaskName: api.SchedulerTask, StartedAt: now}}
	tasks.mu.Unlock()
	if res := step("run-9"); res.NextState != "done" {
		t.Fatalf("beside an older scheduler: %+v; want done", res)
	}
}

// A module awaits what it asked another, once per thing asked, until a
// deadline; past it the internal scheduler flags the wait once; the answer
// clears it.
func TestAModuleAwaitsWhatItAsked(t *testing.T) {
	ctx, svc, _, _ := runnerWorld(t)
	waits, err := application.UseService[api.WaitService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	w, err := waits.Add(ctx, api.AddWaitInput{Module: "billing", On: "shipping.RQ-001", From: "shipping", What: "the parcel weights", By: person})
	if err != nil {
		t.Fatal(err)
	}
	since, _ := time.Parse(time.RFC3339Nano, w.Since)
	deadline, _ := time.Parse(time.RFC3339Nano, w.Deadline)
	if deadline.Sub(since) != 7*24*time.Hour || w.Overdue {
		t.Fatalf("the wait: %+v; want seven days to the deadline", w)
	}
	if again, _ := waits.Add(ctx, api.AddWaitInput{Module: "billing", On: "shipping.RQ-001", From: "shipping", What: "again", By: person}); again.ID != w.ID {
		t.Errorf("a second wait on the same ask: %s; want %s", again.ID, w.ID)
	}
	short, _ := waits.Add(ctx, api.AddWaitInput{Module: "billing", On: "shipping.RQ-002", From: "shipping", What: "the labels", Days: 1, By: person})
	if over, _ := waits.List(ctx, api.ListWaitsInput{Overdue: true}); len(over.Waits) != 0 {
		t.Fatalf("overdue before any deadline: %+v", over.Waits)
	}
	if rep, _ := svc.Tick(ctx, api.TickInput{}); len(rep.Reminded) != 0 {
		t.Fatalf("reminded before any deadline: %v", rep.Reminded)
	}

	// A day later.
	restore := schedcore.SetClock(func() time.Time { return time.Now().Add(25 * time.Hour) })
	defer restore()
	over, _ := waits.List(ctx, api.ListWaitsInput{Overdue: true})
	if len(over.Waits) != 1 || over.Waits[0].ID != short.ID || !over.Waits[0].Overdue {
		t.Fatalf("overdue a day later: %+v; want the labels", over.Waits)
	}
	rep, _ := svc.Tick(ctx, api.TickInput{})
	if len(rep.Reminded) != 1 || !strings.HasPrefix(rep.Reminded[0], short.ID) {
		t.Fatalf("reminded %v; want the labels", rep.Reminded)
	}
	if rep, _ := svc.Tick(ctx, api.TickInput{}); len(rep.Reminded) != 0 {
		t.Errorf("reminded twice: %v", rep.Reminded)
	}

	if out, err := waits.Clear(ctx, api.ClearWaitInput{On: "shipping.RQ-001"}); err != nil || out.Cleared != 1 {
		t.Fatalf("clearing the answered ask: %+v, %v", out, err)
	}
	left, _ := waits.List(ctx, api.ListWaitsInput{Module: "billing"})
	if len(left.Waits) != 1 || left.Waits[0].ID != short.ID || left.Waits[0].Reminded == "" {
		t.Fatalf("left %+v; want the labels, reminded", left.Waits)
	}
	if _, err := waits.Clear(ctx, api.ClearWaitInput{ID: w.ID}); err == nil {
		t.Error("a wait cleared twice")
	}
}

// A to-do reported while its process stopped (its collect cut) is collected by
// the next tick; one reported just now is left to the collect its report
// started.
func TestATickCollectsWhatARestartLeftReported(t *testing.T) {
	ctx, svc, _, got := runnerWorld(t)
	at := func(ago time.Duration) string { return time.Now().Add(-ago).UTC().Format(time.RFC3339Nano) }
	cut := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", State: api.StateReported,
		Result: &api.Result{Status: api.StatusSuccess, Summary: "built", By: person, At: at(30 * time.Minute)}})
	fresh := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", State: api.StateReported,
		Result: &api.Result{Status: api.StatusSuccess, Summary: "fine", By: person, At: at(5 * time.Second)}})
	rep, err := svc.Tick(ctx, api.TickInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Collected) != 1 || rep.Collected[0] != cut.ID {
		t.Fatalf("collected by the tick: %v; want only %s", rep.Collected, cut.ID)
	}
	got.mu.Lock()
	ids := append([]string(nil), got.ids...)
	got.mu.Unlock()
	if len(ids) != 1 || ids[0] != cut.ID {
		t.Fatalf("its owner collected %v; want %s only", ids, cut.ID)
	}
	if g, err := svc.Get(ctx, api.GetInput{ID: fresh.ID}); err != nil || g.Todo.State != api.StateReported {
		t.Fatalf("the one reported just now: %+v, %v; want still reported", g.Todo, err)
	}
}

// What a stopped process left reported is collected when the next one serves,
// whatever its age — no tick runs in a universe whose modules are all manual.
func TestServingCollectsWhatAStoppedProcessLeftReported(t *testing.T) {
	ctx, svc, _, got := runnerWorld(t)
	left := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", State: api.StateReported,
		Result: &api.Result{Status: api.StatusSuccess, Summary: "built", By: person, At: time.Now().Add(-5 * time.Second).UTC().Format(time.RFC3339Nano)}})
	if collected := schedcore.CollectLeftReported(ctx, time.Now()); len(collected) != 1 || collected[0] != left.ID {
		t.Fatalf("collected at serve: %v; want %s", collected, left.ID)
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	if len(got.ids) != 1 || got.ids[0] != left.ID {
		t.Fatalf("its owner collected %v", got.ids)
	}
}
