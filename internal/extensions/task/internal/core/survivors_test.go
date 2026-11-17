// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// The behaviours a mutation audit found no test for: each would have been
// broken silently.

// A run WORKING at a crash whose task changed since is not restarted on the
// new definition: it is reported interrupted, and says why.
func TestAWorkingRunWhoseTaskChangedIsNotRestarted(t *testing.T) {
	release := resetStepper(t)
	defer release()
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useStepper(t, first)
	first.tasks["stepper"].Body = "version 1"
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working in its last step", func() bool { return stepped("work2") == 1 })
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	useStepper(t, second)
	second.tasks["stepper"].Body = "version 2"
	rep := second.RestoreRuns(ctx)
	if len(rep.Restarted) != 0 || len(rep.Interrupted) != 1 {
		t.Fatalf("restore = %+v; want it interrupted, its task changed", rep)
	}
	if e, _ := second.reg.get(r.RunID); e == nil || !strings.Contains(e.err.Error(), "changed") {
		t.Errorf("its error does not say its task changed")
	}
}

// A kept-alive task's backoff doubles with each end (30 s, 60 s, …).
func TestAKeptAliveTasksBackoffDoubles(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	svc.useKeepAlive([]taskapi.KeepAliveTask{{Task: "failer"}})
	info := taskapi.RunInfo{TaskName: "failer", Origin: taskapi.RunOrigin{Kind: taskapi.OriginKeepAlive}}
	var got []time.Duration
	for i := 0; i < 3; i++ {
		svc.keepAliveEnded(info, engine.RunFailed)
		svc.keepAlive.mu.Lock()
		got = append(got, svc.keepAlive.state["failer"].backoff)
		svc.keepAlive.mu.Unlock()
	}
	first := svc.keepAlive.first
	if got[0] != first || got[1] != 2*first || got[2] != 4*first {
		t.Fatalf("backoffs %v; want %v doubling", got, first)
	}
}

// A run the restore interrupts tells its end on its run's topic: a parent
// waiting on it (as a machine) is woken.
func TestAnInterruptedRunTellsItsEndToItsParent(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	child, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g", ParentRunID: "run-parent"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, child.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, child.RunID)

	second := durableService(t, dir)
	told := make(chan any, 1)
	unsub := second.channel.Bus().Subscribe(engine.RunEndedTopic(child.RunID), func(_ string, p any) { told <- p })
	defer unsub()
	if rep := second.RestoreRuns(ctx); len(rep.Interrupted) != 1 {
		t.Fatalf("restore = %+v; want the child interrupted", rep)
	}
	select {
	case <-told:
	case <-time.After(2 * time.Second):
		t.Fatal("the interrupted child's end was not told on its run's topic")
	}
	wake(t, first, child.RunID)
}

// The serving process looks again while it serves: a run a process left after
// the first sweep is restored by a later one.
func TestTheServingProcessSweepsAgain(t *testing.T) {
	old := restoreSweepEvery
	restoreSweepEvery = 50 * time.Millisecond
	t.Cleanup(func() { restoreSweepEvery = old })
	dir := scratchDir(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serving := durableService(t, dir)
	swept := make(chan struct{})
	go serving.ServeRestores(ctx, func() { close(swept) })
	<-swept

	cli := durableService(t, dir) // a CLI run that dies after the first sweep
	r, err := cli.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, cli, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, cli.runs, r.RunID)
	eventually(t, "picked up by a later sweep", func() bool {
		info, _, ok := serving.reg.snapshot(r.RunID)
		return ok && info.Status == taskapi.RunStatusWaiting
	})
	// Registered as waiting before its wait is subscribed again: an event
	// published in between is not kept (OD-020) — wake it until it hears.
	rctx, cancelR := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelR()
	go func() {
		for rctx.Err() == nil {
			_ = serving.channel.Bus().Publish("wake."+r.RunID, map[string]any{})
			time.Sleep(20 * time.Millisecond)
		}
	}()
	res, err := serving.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("the run picked up by the sweep: %+v (%v); want it to go on and complete", res, err)
	}
	_, _ = cli.Stop(context.Background(), taskapi.StopRequest{RunID: r.RunID}) // the dead process's copy
}

// What was kept for a run while it ran goes when it ends: its suspension, its
// children's undrained notifications — and none is queued for it after.
func TestWhatARunKeptGoesWhenItEnds(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	if _, err := svc.suspend.Suspend(r.RunID, "agent:!question/q/answered"); err != nil {
		t.Fatal(err)
	}
	_ = svc.suspend.Resume(r.RunID, []byte(`"yes"`))
	bg := svc.bgMgr.(*bgManager)
	bg.enqueueNotification(r.RunID, taskapi.TaskNotification{TaskID: "child-1"})
	wake(t, svc, r.RunID)
	e, _ := svc.reg.get(r.RunID)
	<-e.done
	eventually(t, "released", func() bool {
		svc.suspend.mu.RLock()
		_, kept := svc.suspend.suspensions[r.RunID]
		svc.suspend.mu.RUnlock()
		bg.mu.RLock()
		_, queued := bg.notifications[r.RunID]
		bg.mu.RUnlock()
		return !kept && !queued
	})
	bg.enqueueNotification(r.RunID, taskapi.TaskNotification{TaskID: "child-late"})
	if n := len(bg.Notifications(r.RunID)); n != 0 {
		t.Errorf("%d notifications queued for a parent that has ended", n)
	}
}

// A run that ended and left the registry (a restart, its retention) still
// answers Status from its kept result — a parent rechecking its child does not
// take it for gone.
func TestStatusAnswersForAnEndedRun(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useMachine(first, eoDone{})
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "eodone", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Result(ctx, taskapi.ResultRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	second := durableService(t, dir)
	st, err := second.Status(ctx, taskapi.StatusRequest{RunID: r.RunID})
	if err != nil || st.Status != taskapi.RunStatusCompleted {
		t.Fatalf("Status after a restart: %+v (%v); want completed", st, err)
	}
}

// A killed run tells its end once, when it is killed — a parent waiting on it
// learns it then — and not again when its goroutine, let go, returns.
func TestAKilledRunTellsItsEndOnce(t *testing.T) {
	old := KillAfter
	KillAfter = 50 * time.Millisecond
	t.Cleanup(func() { KillAfter = old })
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	release := make(chan struct{})
	var actions atomic.Int64
	useMachine(svc, eoStuck{name: "eostuck-kill", actions: &actions, release: release})
	ctx := context.Background()
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-kill"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working", func() bool { return actions.Load() == 1 })
	var ends atomic.Int32
	unsub := svc.channel.Bus().Subscribe(engine.RunEndedTopic(r.RunID), func(string, any) { ends.Add(1) })
	defer unsub()
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "its end told at the kill", func() bool { return ends.Load() == 1 })
	close(release) // its goroutine returns, late
	e, _ := svc.reg.get(r.RunID)
	<-e.done
	time.Sleep(50 * time.Millisecond)
	if n := ends.Load(); n != 1 {
		t.Fatalf("its end told %d times; once", n)
	}
}
