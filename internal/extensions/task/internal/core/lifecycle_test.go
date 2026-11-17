// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A RUN SAYS WHAT HAPPENS TO IT — every transition of a run is one event on its
// lifecycle topic, in the order it happens, with what the transition needs
// (what it waits on, what woke it, how long it queued, why it failed).

// lifecycleOf records the lifecycle events svc publishes.
type lifecycleLog struct {
	mu     sync.Mutex
	events []engine.RunLifecycleEvent
}

func lifecycleOf(svc *Service) *lifecycleLog {
	l := &lifecycleLog{}
	svc.channel.Bus().SubscribeAny(func(topic string, payload any) {
		if e, ok := payload.(*engine.RunLifecycleEvent); ok && engine.IsRunLifecycleTopic(topic) && topic == engine.RunLifecycleTopic(e.RunID) {
			l.mu.Lock()
			l.events = append(l.events, *e)
			l.mu.Unlock()
		}
	})
	return l
}

func (l *lifecycleLog) of(runID string) []engine.RunLifecycleEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []engine.RunLifecycleEvent
	for _, e := range l.events {
		if e.RunID == runID {
			out = append(out, e)
		}
	}
	return out
}

func (l *lifecycleLog) transitions(runID string) []engine.RunTransition {
	var out []engine.RunTransition
	for _, e := range l.of(runID) {
		out = append(out, e.Transition)
	}
	return out
}

func (l *lifecycleLog) last(runID string, t engine.RunTransition) (engine.RunLifecycleEvent, bool) {
	evs := l.of(runID)
	for i := len(evs) - 1; i >= 0; i-- {
		if evs[i].Transition == t {
			return evs[i], true
		}
	}
	return engine.RunLifecycleEvent{}, false
}

func (l *lifecycleLog) has(runID string, t engine.RunTransition) bool {
	_, ok := l.last(runID, t)
	return ok
}

// UC-130: a run that queues, starts, waits, is woken and completes says so, in
// that order, each transition once, with the status it leads to.
func TestRun_StatusTransitions(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	log := lifecycleOf(svc)
	release := make(chan struct{})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	useStubborn(svc, release)
	holder(t, svc, "g")
	ctx := context.Background()

	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "queued", func() bool { return log.has(r.RunID, engine.RunQueued) })
	time.Sleep(20 * time.Millisecond)
	once.Do(func() { close(release) }) // the holder ends: the waiter gets the slot
	eventually(t, "waiting", func() bool { return log.has(r.RunID, engine.RunWaiting) })
	wake(t, svc, r.RunID)
	eventually(t, "completed", func() bool { return log.has(r.RunID, engine.RunCompleted) })

	want := []engine.RunTransition{engine.RunQueued, engine.RunStarted, engine.RunWaiting, engine.RunWoken, engine.RunCompleted}
	if got := log.transitions(r.RunID); !reflect.DeepEqual(got, want) {
		t.Fatalf("transitions = %v, want %v", got, want)
	}
	statuses := map[engine.RunTransition]string{engine.RunQueued: "queued", engine.RunStarted: "running", engine.RunWaiting: "waiting", engine.RunCompleted: "completed"}
	for _, e := range log.of(r.RunID) {
		if want, ok := statuses[e.Transition]; ok && e.Status != want {
			t.Errorf("%s: status %q, want %q", e.Transition, e.Status, want)
		}
		if e.TaskName != "waiter" || e.Group != "g" || e.At.IsZero() {
			t.Errorf("%s: %+v; want its task, group and time", e.Transition, e)
		}
	}
	started, _ := log.last(r.RunID, engine.RunStarted)
	if started.QueuedForMs < 20 {
		t.Errorf("started after queueing %d ms; want how long it queued", started.QueuedForMs)
	}
	waiting, _ := log.last(r.RunID, engine.RunWaiting)
	if !reflect.DeepEqual(waiting.Waiting, []string{"wake." + r.RunID}) {
		t.Errorf("waiting on %v; want what it waits for", waiting.Waiting)
	}
	if woken, _ := log.last(r.RunID, engine.RunWoken); woken.WokenBy == "" {
		t.Error("woken by nothing; want what woke it")
	}
}

// A run stopped while it waits says stopping, then cancelled; one stopped while
// it queues says so too, and why.
func TestAStoppedRunSaysStoppingThenCancelled(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	log := lifecycleOf(svc)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	ctx := context.Background()

	w, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return log.has(w.RunID, engine.RunWaiting) })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: w.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled", func() bool { return log.has(w.RunID, engine.RunCancelled) })
	got := log.transitions(w.RunID)
	if got[len(got)-2] != engine.RunStopping {
		t.Errorf("transitions = %v; want stopping before cancelled", got)
	}

	holder(t, svc, "g")
	q, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "queued", func() bool { return log.has(q.RunID, engine.RunQueued) })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: q.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled while queued", func() bool { return log.has(q.RunID, engine.RunCancelled) })
	if e, _ := log.last(q.RunID, engine.RunCancelled); !strings.Contains(e.Reason, "queued") {
		t.Errorf("reason %q; want it to say it was stopped while queued", e.Reason)
	}
}

// A run that does not end once stopped is killed, and says why.
func TestAKilledRunSaysSo(t *testing.T) {
	old := KillAfter
	KillAfter = 30 * time.Millisecond
	t.Cleanup(func() { KillAfter = old })
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	log := lifecycleOf(svc)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	ctx := context.Background()
	before := stubbornActions.Load()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "its action running", func() bool { return stubbornActions.Load() > before })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "killed", func() bool { return log.has(r.RunID, engine.RunKilled) })
	if e, _ := log.last(r.RunID, engine.RunKilled); e.Reason == "" || e.Status != string(taskapi.RunStatusKilled) {
		t.Errorf("killed = %+v; want its status and why", e)
	}
}

// failer fails at its first step; timer waits 10 ms for nothing, then ends.
type failer struct{}

func (failer) Name() string                           { return "failer" }
func (failer) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (failer) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) { return nil, errors.New("the disk is full") }, nil
}

type timer struct{}

func (timer) Name() string                           { return "timer" }
func (timer) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (timer) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	if st.Name == "init" {
		return func(context.Context) (*engine.StepResult, error) {
			return &engine.StepResult{NextState: "waited", Wait: &engine.WaitSpec{Mode: engine.WaitAny,
				Conditions: []engine.WaitCondition{{Topic: "never", Label: "the never event"}}, Timeout: 10 * time.Millisecond}}, nil
		}, nil
	}
	return func(context.Context) (*engine.StepResult, error) {
		return &engine.StepResult{NextState: "done", Output: map[string]any{}}, nil
	}, nil
}

type machineOf struct{ m engine.Machine }

func (f machineOf) Name() string { return f.m.Name() }
func (f machineOf) CreateMachine(context.Context, any) (engine.Machine, error) {
	return f.m, nil
}

func useMachine(svc *Service, m engine.Machine) {
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: m.Name()}, Impl: machineOf{m}})
	svc.tasks[m.Name()] = &taskapi.Task{Name: m.Name(), Provider: "test", Type: m.Name()}
}

// A failed run says why; a wait that times out says so, with its deadline.
func TestAFailureAndATimeoutAreTold(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	log := lifecycleOf(svc)
	useMachine(svc, failer{})
	useMachine(svc, timer{})
	ctx := context.Background()

	f, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "failer"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed", func() bool { return log.has(f.RunID, engine.RunFailed) })
	if e, _ := log.last(f.RunID, engine.RunFailed); !strings.Contains(e.Reason, "the disk is full") {
		t.Errorf("failed with %q; want the reason", e.Reason)
	}
	if res, _ := svc.Result(ctx, taskapi.ResultRequest{RunID: f.RunID}); !strings.Contains(res.Error, "the disk is full") {
		t.Errorf("Result = %+v; want the step's error (it was empty)", res)
	}

	tm, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "timer"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "completed", func() bool { return log.has(tm.RunID, engine.RunCompleted) })
	if !log.has(tm.RunID, engine.RunTimedOut) {
		t.Errorf("transitions = %v; want timed-out", log.transitions(tm.RunID))
	}
	if e, _ := log.last(tm.RunID, engine.RunWaiting); e.Deadline == nil || !reflect.DeepEqual(e.Waiting, []string{"the never event"}) {
		t.Errorf("waiting = %+v; want its deadline and what it waited for", e)
	}
}

// failingStore loses every checkpoint.
type failingStore struct{ engine.StateStore }

func (failingStore) Save(string, engine.Checkpoint) error { return errors.New("no space left") }

// A checkpoint that cannot be saved is told: the run goes on, but a restart
// would not find it where it is.
func TestACheckpointNotSavedIsTold(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	log := lifecycleOf(svc)
	store := tellingStore{StateStore: failingStore{}, s: svc}
	if err := store.Save("r-1", engine.Checkpoint{}); err == nil {
		t.Fatal("the store's error was swallowed")
	}
	eventually(t, "told", func() bool { return log.has("r-1", engine.RunCheckpointFailed) })
	if e, _ := log.last("r-1", engine.RunCheckpointFailed); !strings.Contains(e.Reason, "no space left") {
		t.Errorf("reason %q", e.Reason)
	}
}

// A wait inside a tool call (a person answering at the UI, a child's question
// suspended) says what it waits for and that it was woken.
func TestAWaitInsideAToolIsTold(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	log := lifecycleOf(svc)
	if err := svc.suspend.WhileWaiting(context.Background(), "r-2", func() {}); err != nil {
		t.Fatal(err)
	}
	want := []engine.RunTransition{engine.RunWaiting, engine.RunWoken}
	eventually(t, "told", func() bool { return reflect.DeepEqual(log.transitions("r-2"), want) })

	ch, err := svc.suspend.Suspend("r-3", "agent:!question/q-1/answered")
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := log.last("r-3", engine.RunWaiting); !ok || !reflect.DeepEqual(e.Waiting, []string{"agent:!question/q-1/answered"}) {
		t.Errorf("waiting = %+v; want what it waits for", e)
	}
	_ = ch
	if err := svc.suspend.Resume("r-3", []byte(`"ok"`)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "woken", func() bool { return log.has("r-3", engine.RunWoken) })
}

// After a restart a waiting run says resumed, a working one restarted, and one
// that cannot go on interrupted — with why.
func TestARestoreTellsWhatItDidToEachRun(t *testing.T) {
	ctx := context.Background()

	dir := scratchDir(t)
	m, _ := leftRun(t, dir)
	second := durableService(t, dir)
	log := lifecycleOf(second)
	second.RestoreRuns(ctx)
	eventually(t, "resumed", func() bool { return log.has(m.RunID, engine.RunResumed) })
	wake(t, second, m.RunID)

	release := resetStepper(t)
	dir = scratchDir(t)
	first := durableService(t, dir)
	useStepper(t, first)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working", func() bool { return stepped("work2") == 1 })
	orphan(t, first.runs, r.RunID)
	third := durableService(t, dir)
	useStepper(t, third)
	log = lifecycleOf(third)
	third.RestoreRuns(ctx)
	eventually(t, "restarted", func() bool { return log.has(r.RunID, engine.RunRestarted) })
	release()

	dir = scratchDir(t)
	stubborn := make(chan struct{})
	t.Cleanup(func() { close(stubborn) })
	fourth := durableService(t, dir)
	useStubborn(fourth, stubborn)
	s, err := fourth.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "running", func() bool { return groupLoad(t, fourth, "g").Running == 1 })
	began(t, fourth.runs, s.RunID)
	orphan(t, fourth.runs, s.RunID)
	fifth := durableService(t, dir)
	useStubborn(fifth, stubborn)
	log = lifecycleOf(fifth)
	fifth.RestoreRuns(ctx)
	eventually(t, "interrupted", func() bool { return log.has(s.RunID, engine.RunInterrupted) })
	if e, _ := log.last(s.RunID, engine.RunInterrupted); e.Reason == "" {
		t.Error("interrupted with no reason")
	}
}
