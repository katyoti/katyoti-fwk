// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// STOP, KILL AND THE QUEUE — stopping always settles: callers get an answer,
// what a run held is let go, and a run is never refused or dropped wrongly.

// A waiting run restored after a restart is not refused by its group's queue
// limit: it takes no queue place — it waits for an event, not for a slot.
func TestARestoredRunIsNotRefusedByAFullQueue(t *testing.T) {
	stubbornRelease = make(chan struct{})
	t.Cleanup(func() { close(stubbornRelease) })
	dir := scratchDir(t)
	m, first := leftRun(t, dir)

	second := durableService(t, dir)
	second.useGroups(newGroupScheduler(taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 1, QueueLimit: 1}}}))
	useStubborn(second, stubbornRelease)
	ctx := context.Background()
	if _, err := second.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the slot taken", func() bool { return groupLoad(t, second, "g").Running == 1 })
	if _, err := second.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the queue full", func() bool { return groupLoad(t, second, "g").Queued == 1 })

	rep := second.RestoreRuns(ctx)
	if len(rep.Resumed) != 1 || len(rep.Interrupted) != 0 {
		t.Errorf("restore into a full group = %+v; want the waiting run resumed", rep)
	}
	wake(t, first, m.RunID)
}

// A killed run's Result answers — killed — at once: a delegating parent
// waiting on it is not left blocked until the stuck goroutine ends.
func TestAKilledRunsResultAnswers(t *testing.T) {
	old := KillAfter
	KillAfter = 50 * time.Millisecond
	t.Cleanup(func() { KillAfter = old })
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	ctx := context.Background()
	before := stubbornActions.Load()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "its action running", func() bool { return stubbornActions.Load() > before })
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID})
	eventually(t, "killed", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusKilled })

	rctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusKilled) {
		t.Errorf("Result of a killed run = %+v, %v; want killed, at once", res, err)
	}
}

// Stopping a run while it is still queued lets go of what it held: its
// session recorder's subscriptions on the bus.
func TestAStoppedQueuedRunLetsGoOfItsRecorder(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	bus, ok := svc.channel.Bus().(*engine.MemoryEventBus)
	if !ok {
		t.Fatalf("bus %T", svc.channel.Bus())
	}
	ctx := context.Background()
	if _, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the slot taken", func() bool { return groupLoad(t, svc, "g").Running == 1 })
	before := bus.TotalSubscribers()
	q, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "queued", func() bool { return groupLoad(t, svc, "g").Queued == 1 })
	if bus.TotalSubscribers() <= before {
		t.Skip("no session recorder in this environment: nothing to let go")
	}
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: q.RunID})
	eventually(t, "its recorder's subscriptions gone", func() bool { return bus.TotalSubscribers() == before })
}

// A run waiting inside a tool call (the suspension manager) that ends without
// an answer leaves no goroutine behind.
func TestAnUnansweredInToolWaitLeavesNoGoroutine(t *testing.T) {
	g := newGroupScheduler(taskapi.TaskGroupsConfig{})
	if err := g.admit("r1", "g", 0); err != nil {
		t.Fatal(err)
	}
	if err := g.acquire(context.Background(), "r1"); err != nil {
		t.Fatal(err)
	}
	m := newSuspensionManager()
	m.slots = g
	if _, err := m.Suspend("r1", "cwm:!workspace/object/q/updated"); err != nil {
		t.Fatal(err)
	}
	if suspendGoroutines() == 0 {
		t.Fatal("no waiting goroutine to begin with: the test sees nothing")
	}
	g.done("r1") // the run ends (stopped) — no answer ever comes
	eventually(t, "the waiting goroutine gone", func() bool { return suspendGoroutines() == 0 })
}

// suspendGoroutines counts the goroutines a suspension started.
func suspendGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "created by github.com/katyoti/katyoti-fwk/internal/extensions/task/internal/core.(*suspensionManager).Suspend")
}

// bgParent starts a background child (a waiter), then waits for ever.
type bgParent struct{}

var bgParentState struct {
	svc   *Service
	child chan string
}

func (bgParent) Name() string                           { return "bgparent" }
func (bgParent) InitialState(engine.Event) engine.State { return engine.State{Name: "init"} }
func (bgParent) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	return func(context.Context) (*engine.StepResult, error) {
		if st.Name == "init" {
			id, err := bgParentState.svc.bgMgr.Register(st.RunID, taskapi.TaskRunRequest{TaskName: "waiter", MaxDurationS: 60})
			if err != nil {
				return nil, err
			}
			bgParentState.child <- id
		}
		return &engine.StepResult{NextState: "waiting", Wait: &engine.WaitSpec{Mode: engine.WaitAny,
			Conditions: []engine.WaitCondition{{Topic: "never." + st.RunID}}}}, nil
	}, nil
}

type bgParentFactory struct{}

func (bgParentFactory) Name() string { return "bgparent" }
func (bgParentFactory) CreateMachine(context.Context, any) (engine.Machine, error) {
	return bgParent{}, nil
}

// Stopping a run stops the background children it started: they do not go on
// spending (an agent's tokens) for up to their whole time budget.
func TestStoppingAParentStopsItsBackgroundChildren(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	_ = engine.MachineDomain.Types.Contribute(strategies.TypeDef[engine.MachineFactory]{DefMeta: strategies.DefMeta{Name: "bgparent"}, Impl: bgParentFactory{}})
	svc.tasks["bgparent"] = &taskapi.Task{Name: "bgparent", Provider: "test", Type: "bgparent"}
	bgParentState.svc = svc
	bgParentState.child = make(chan string, 1)
	ctx := context.Background()
	p, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "bgparent"})
	if err != nil {
		t.Fatal(err)
	}
	var child string
	select {
	case child = <-bgParentState.child:
	case <-time.After(3 * time.Second):
		t.Fatal("the parent started no child")
	}
	eventually(t, "the child running", func() bool {
		rec, err := svc.bgMgr.TaskStatus(child)
		return err == nil && rec.Status == taskapi.TaskRunning
	})
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: p.RunID})
	eventually(t, "the child stopped with its parent", func() bool {
		rec, err := svc.bgMgr.TaskStatus(child)
		return err == nil && rec.Status != taskapi.TaskRunning && rec.Status != taskapi.TaskPending
	})
}

// A stopped run that is still ending is not dropped from the registry by the
// retention bounds: it is not ended yet.
func TestAStoppedRunStillEndingIsKept(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	svc.SetRetentionTTL(time.Millisecond)
	ctx := context.Background()
	before := stubbornActions.Load()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn"})
	if err != nil {
		t.Fatal(err)
	}
	// Its action running (not only its slot granted: stopped before its step,
	// a run ends at once — and is pruned).
	eventually(t, "its action running", func() bool { return stubbornActions.Load() > before })
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID})
	time.Sleep(20 * time.Millisecond)
	if _, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "uc-130-task"}); err != nil { // prunes on insert
		t.Fatal(err)
	}
	if _, err := svc.Status(ctx, taskapi.StatusRequest{RunID: r.RunID}); err != nil {
		t.Errorf("a stopped run still ending was dropped: %v", err)
	}
}
