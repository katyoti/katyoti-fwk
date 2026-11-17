// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A child's id is stable: started twice at once, or again after its parent's
// restart, the same id gives one run.
func TestAChildIsStartedOnceUnderItsId(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	ctx := context.Background()
	id := engine.ChildRunID("run-parent", "dlg@w.1|i=|a=0")
	if id != engine.ChildRunID("run-parent", "dlg@w.1|i=|a=0") || id == engine.ChildRunID("run-other", "dlg@w.1|i=|a=0") {
		t.Fatal("ChildRunID is not stable per parent and site")
	}
	var started atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, existing, err := svc.runAs(ctx, taskapi.TaskRunRequest{TaskName: "waiter"}, id, &spawnSite{Name: "a", Site: "dlg@w.1", By: "workflow"})
			if err != nil {
				t.Error(err)
			}
			if !existing {
				started.Add(1)
			}
		}()
	}
	wg.Wait()
	if started.Load() != 1 || len(runsOf(t, svc, "waiter")) != 1 {
		t.Fatalf("%d started, %d runs; one", started.Load(), len(runsOf(t, svc, "waiter")))
	}
	if _, _, err := svc.runAs(ctx, taskapi.TaskRunRequest{TaskName: "waiter"}, "not-an-id", nil); err == nil {
		t.Error("a malformed run id was accepted")
	}
	eventually(t, "waiting", func() bool { return status(t, svc, id) == taskapi.RunStatusWaiting })
	wake(t, svc, id)
}

// A child left on disk by a process that died, or one that ended, is found
// under its id — never started again.
func TestAChildLeftOrEndedIsNotStartedAgain(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	useMachine(first, eoDone{})
	ended := engine.ChildRunID("run-p", "ended")
	if _, existing, err := first.runAs(ctx, taskapi.TaskRunRequest{TaskName: "eodone", Group: "g"}, ended, nil); err != nil || existing {
		t.Fatalf("first start: existing=%v err=%v", existing, err)
	}
	if _, err := first.Result(ctx, taskapi.ResultRequest{RunID: ended}); err != nil {
		t.Fatal(err)
	}
	left := engine.ChildRunID("run-p", "left")
	if _, _, err := first.runAs(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g2"}, left, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, left) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, left)

	second := durableService(t, dir)
	useMachine(second, eoDone{})
	before := eoDoneSteps.Load()
	for _, id := range []string{ended, left} {
		_, existing, err := second.runAs(ctx, taskapi.TaskRunRequest{TaskName: "eodone", Group: "g"}, id, nil)
		if err != nil || !existing {
			t.Errorf("%s: existing=%v err=%v; want it found", id, existing, err)
		}
	}
	if eoDoneSteps.Load() != before {
		t.Error("a child found under its id was started again")
	}
	wake(t, first, left)
}

// A child of a run already admitted queues past a full queue — a workflow is
// not failed half-way; another run is refused.
func TestAChildQueuesPastAFullQueue(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": {MaxRunning: 1, QueueLimit: 1}}})
	ctx := context.Background()
	parent, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, parent.RunID) == taskapi.RunStatusWaiting })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var actions atomic.Int64
	useMachine(svc, eoStuck{name: "eostuck-q", actions: &actions, release: release})
	a, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-q", Group: "g"}) // running
	b, _ := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-q", Group: "g"}) // queued: full
	_, _ = a, b
	eventually(t, "queue full", func() bool { return groupLoad(t, svc, "g").Queued == 1 })
	if _, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-q", Group: "g"}); err == nil {
		t.Fatal("a run past the full queue was not refused")
	}
	child := engine.ChildRunID(parent.RunID, "c")
	if _, _, err := svc.runAs(ctx, taskapi.TaskRunRequest{TaskName: "eostuck-q", Group: "g", ParentRunID: parent.RunID}, child, nil); err != nil {
		t.Fatalf("a child of an admitted run was refused: %v", err)
	}
	_, _ = svc.Stop(ctx, taskapi.StopRequest{RunID: parent.RunID}) // its group stays full: not woken
}

// Stopping a run stops its children; a restart revives none of them.
func TestStoppingAParentStopsItsChildren(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	parent, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g2"})
	if err != nil {
		t.Fatal(err)
	}
	child := engine.ChildRunID(parent.RunID, "c")
	if _, _, err := first.runAs(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g2", ParentRunID: parent.RunID}, child, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "both waiting", func() bool {
		return status(t, first, parent.RunID) == taskapi.RunStatusWaiting && status(t, first, child) == taskapi.RunStatusWaiting
	})
	if _, err := first.Stop(ctx, taskapi.StopRequest{RunID: parent.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the child cancelled", func() bool { return status(t, first, child) == taskapi.RunStatusCancelled })
}

// A cancel request on a run's topic stops it (a terminal, another process).
func TestACancelRequestStopsARun(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	_ = svc.channel.Bus().Publish(engine.CancelRequestTopic(r.RunID), "from the terminal")
	eventually(t, "cancelled", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusCancelled })
}

// A child's end is read as soon as it is set in the registry — before its
// goroutine returns: the parent is woken by its end in between, and a child
// read as still running there leaves the parent waiting for ever.
func TestAChildIsEndedOnceItsEndIsSet(t *testing.T) {
	svc := &Service{reg: newRunRegistry()}
	e := &runEntry{info: taskapi.RunInfo{RunID: "c", Status: taskapi.RunStatusRunning}, done: make(chan struct{}), killed: make(chan struct{})}
	svc.reg.add(e)
	sp := serviceSpawner{s: svc}
	if _, ended := sp.Outcome("c"); ended {
		t.Fatal("a running child read as ended")
	}
	svc.reg.mutate("c", func(e *runEntry) {
		e.info.Status = taskapi.RunStatusCompleted
		e.result = &engine.RunResult{Next: engine.Done, Output: map[string]any{"v": 1}}
		e.terminalAt = time.Now()
	})
	o, ended := sp.Outcome("c") // its done not closed yet
	if !ended || o.Status != engine.ChildDone {
		t.Fatalf("outcome %+v ended=%v; want done once its end is set", o, ended)
	}
}

// A workflow step with no mode gives its child the worker preset's override —
// as the delegate tool does — never none (an agent child would have its
// dangerous tools denied by its fail-closed spawn gate).
func TestAModelessChildGetsTheWorkerOverride(t *testing.T) {
	o := childOverride(engine.ChildSpec{Name: "c"})
	sp, ok := o["spawnPermissions"].(map[string]any)
	if !ok {
		t.Fatalf("override %v; want the worker's spawn permissions", o)
	}
	if allowed, _ := sp["allowedTools"].([]any); len(allowed) != 1 || allowed[0] != "*" {
		t.Errorf("allowedTools %v; want the worker's [*]", sp["allowedTools"])
	}
}
