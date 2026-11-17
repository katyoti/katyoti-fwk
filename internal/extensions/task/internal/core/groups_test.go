// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TASK GROUPS — a group limits how many of its runs RUN at once; a run holds
// a slot from wake to the next wait, waiting holds none.

func one(n int) taskapi.TaskGroup { return taskapi.TaskGroup{MaxRunning: n} }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	// A wait on a condition, never a pause: the ceiling only bounds a test that
	// fails - a loaded go test ./... can take seconds to deliver an end.
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("20s and still not: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// The limit holds; a slot given back goes to the queue's best waiter: higher
// priority first, then submit order.
func TestAGroupRunsAtMostItsLimit(t *testing.T) {
	g := newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	ctx := context.Background()
	for _, r := range []struct {
		id   string
		prio int
	}{{"a", 0}, {"b", 0}, {"c", 5}} {
		if err := g.admit(r.id, "g", r.prio); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.acquire(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 2)
	for _, id := range []string{"b", "c"} {
		go func(id string) {
			if g.acquire(ctx, id) == nil {
				got <- id
			}
		}(id)
	}
	eventually(t, "b and c queued", func() bool { _, q := g.load("g"); return q == 2 })
	if running, _ := g.load("g"); running != 1 {
		t.Fatalf("running = %d, want 1", running)
	}
	next := func() string {
		select {
		case id := <-got:
			return id
		case <-time.After(5 * time.Second):
			t.Fatal("no waiter was served")
			return ""
		}
	}
	g.done("a")
	first := next()
	if first != "c" {
		t.Errorf("first served = %s, want c (priority 5)", first)
	}
	g.done(first)
	if then := next(); then != "b" && first == "c" {
		t.Errorf("then = %s, want b", then)
	}
}

// Waiting gives the slot back; a woken run queues for it like a new one.
func TestWaitingHoldsNoSlot(t *testing.T) {
	g := newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	ctx := context.Background()
	_ = g.admit("a", "g", 0)
	_ = g.admit("b", "g", 0)
	_ = g.acquire(ctx, "a")
	g.release("a") // a waits
	if err := g.acquire(ctx, "b"); err != nil {
		t.Fatalf("b while a waits: %v", err)
	}
	woke := make(chan struct{})
	go func() { _ = g.acquire(ctx, "a"); close(woke) }() // a's event arrives
	select {
	case <-woke:
		t.Fatal("a ran again while b holds the only slot")
	case <-time.After(50 * time.Millisecond):
	}
	g.done("b")
	<-woke
}

// A full queue refuses a new run, with the group's load.
func TestAFullGroupRefusesARun(t *testing.T) {
	g := newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 1, QueueLimit: 1}}})
	ctx := context.Background()
	_ = g.admit("a", "g", 0)
	_ = g.acquire(ctx, "a")
	_ = g.admit("b", "g", 0)
	go func() { _ = g.acquire(ctx, "b") }()
	eventually(t, "b queued", func() bool { _, q := g.load("g"); return q == 1 })
	err := g.admit("c", "g", 0)
	var full *taskapi.GroupFullError
	if !errors.As(err, &full) || !errors.Is(err, taskapi.ErrGroupFull) || full.Running != 1 || full.Queued != 1 {
		t.Fatalf("admit c = %v; want ErrGroupFull with running 1, queued 1", err)
	}
}

// A paused group queues, starts nothing.
func TestAPausedGroupStartsNothing(t *testing.T) {
	off := false
	g := newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 3, On: &off}}})
	_ = g.admit("a", "g", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := g.acquire(ctx, "a"); err == nil {
		t.Fatal("a run started in a paused group")
	}
	if _, q := g.load("g"); q != 0 {
		t.Errorf("queued = %d after giving up, want 0", q)
	}
}

// An in-tool wait (SuspensionManager) gives the slot back too, and the waiter
// continues only once it has a slot again.
func TestASuspendedRunHoldsNoSlot(t *testing.T) {
	g := newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	m := newSuspensionManager()
	m.slots = g
	ctx := context.Background()
	_ = g.admit("a", "g", 0)
	_ = g.admit("b", "g", 0)
	_ = g.acquire(ctx, "a")
	ch, err := m.Suspend("a", "inbox:wait")
	if err != nil {
		t.Fatal(err)
	}
	if running, _ := g.load("g"); running != 0 {
		t.Fatalf("running = %d while a is suspended, want 0", running)
	}
	_ = g.acquire(ctx, "b")
	if err := m.Resume("a", json.RawMessage(`"yes"`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ch:
		t.Fatal("a continued while b holds the only slot")
	case <-time.After(50 * time.Millisecond):
	}
	g.done("b")
	select {
	case p := <-ch:
		if string(p) != `"yes"` {
			t.Errorf("payload = %s", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a never continued")
	}
}

// waiterMachine waits for `wake.<run>` then ends: a run that waits through the
// executor. waiterStarts counts its first steps: a run resumed where it waited
// does not take one again.
type waiterMachine struct{}

var waiterStarts atomic.Int64

func (waiterMachine) Name() string                           { return "waiter" }
func (waiterMachine) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (waiterMachine) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	if st.Name == "init" {
		return func(context.Context) (*engine.StepResult, error) {
			waiterStarts.Add(1)
			return &engine.StepResult{NextState: "waiting", Wait: &engine.WaitSpec{
				Conditions: []engine.WaitCondition{{Topic: "wake." + st.RunID}}, Mode: engine.WaitAny}}, nil
		}, nil
	}
	return func(context.Context) (*engine.StepResult, error) {
		return &engine.StepResult{NextState: "done", Output: map[string]any{"woke": true}}, nil
	}, nil
}

type waiterFactory struct{}

func (waiterFactory) Name() string { return "waiter" }
func (waiterFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return waiterMachine{}, nil
}

func groupsService(t *testing.T, cfg taskapi.TaskGroupsConfig) *Service {
	t.Helper()
	svc, _ := newServiceForLifecycleTest(t)
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "waiter"}, Impl: waiterFactory{}})
	svc.tasks["waiter"] = &taskapi.Task{Name: "waiter", Provider: "test", Type: "waiter"}
	svc.suspend = newSuspensionManager()
	svc.useGroups(newGroupScheduler(cfg))
	svc.sessionsDir = sessionsDir(t)
	return svc
}

func status(t *testing.T, svc *Service, id string) taskapi.RunStatus {
	t.Helper()
	st, err := svc.Status(context.Background(), taskapi.StatusRequest{RunID: id})
	if err != nil {
		t.Fatal(err)
	}
	return st.Status
}

// End to end: in a group of one, a run waiting on its event lets another run
// start and finish; when its event comes it runs again and finishes. The run
// statuses follow: running, waiting, completed; the other queued then running.
func TestARunWaitingThroughTheExecutorLetsAnotherRun(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	ctx := context.Background()
	a, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "a waiting", func() bool { return status(t, svc, a.RunID) == taskapi.RunStatusWaiting })
	b, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "uc-130-task", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if res, err := svc.Result(ctx, taskapi.ResultRequest{RunID: b.RunID}); err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("b while a waits: %+v, %v; want completed", res, err)
	}
	_ = svc.channel.Bus().Publish("wake."+a.RunID, map[string]any{})
	if res, err := svc.Result(ctx, taskapi.ResultRequest{RunID: a.RunID}); err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("a after its event: %+v, %v; want completed", res, err)
	}
	if running, queued := svc.groups.load("g"); running != 0 || queued != 0 {
		t.Errorf("group after both: running %d queued %d, want 0 0", running, queued)
	}
}

// A run awaiting a child's result gives its slot back: a parent and its child
// in one group of one do not deadlock. The child inherits the group.
func TestAParentAwaitingItsChildDoesNotDeadlock(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	ctx := context.Background()
	parent := "run-parent"
	if err := svc.groups.admit(parent, "g", 0); err != nil {
		t.Fatal(err)
	}
	if err := svc.groups.acquire(ctx, parent); err != nil {
		t.Fatal(err)
	}
	child, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "uc-130-task", ParentRunID: parent})
	if err != nil {
		t.Fatal(err)
	}
	if g := svc.groups.groupOf(child.RunID); g != "" && g != "g" {
		t.Errorf("child group = %q, want the parent's (g)", g)
	}
	pctx, cancel := context.WithTimeout(engine.WithRunID(ctx, parent), 5*time.Second)
	defer cancel()
	res, err := svc.Result(pctx, taskapi.ResultRequest{RunID: child.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("parent awaiting its child: %+v, %v; want completed (no deadlock)", res, err)
	}
	if running, _ := svc.groups.load("g"); running != 1 {
		t.Errorf("running after = %d, want 1 (the parent has its slot again)", running)
	}
}

// A run naming no group and no parent is the default group's: a person's.
func TestARunWithNoGroupIsTheUsers(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	if g := svc.groups.groupOf(r.RunID); g != taskapi.DefaultGroup {
		t.Errorf("group = %q, want %q", g, taskapi.DefaultGroup)
	}
	eventually(t, "the run waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	_ = svc.channel.Bus().Publish("wake."+r.RunID, map[string]any{})
	_, _ = svc.Result(context.Background(), taskapi.ResultRequest{RunID: r.RunID})
}
