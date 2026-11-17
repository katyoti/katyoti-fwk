// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"os"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// DURABLE WAITS — a run waiting when its process ends is resumed by the next
// one; any other run it left is reported interrupted.

// durableService is a groups service whose runs are kept in dir.
func durableService(t *testing.T, dir string) *Service {
	t.Helper()
	svc := groupsService(t, taskapi.TaskGroupsConfig{Groups: map[string]taskapi.TaskGroup{"g": one(1)}})
	st, err := newRunStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.close) // its owner lock open would keep the folder
	t.Cleanup(st.close)
	svc.useRuns(st)
	return svc
}

// orphan makes a run's manifest look left by a process that is gone (an
// owner with no lease).
func orphan(t *testing.T, st *runStore, runID string) {
	t.Helper()
	b, err := atomicio.ReadAtomic(st.manifestPath(runID)) // the run may be rewriting it (its end)
	if err != nil {
		t.Fatal(err)
	}
	var m runManifest
	if err := readSealedJSON(b, &m); err != nil {
		t.Fatal(err)
	}
	m.Owner, m.PID = "gone", 0
	// Written as the dead process left it (saveManifest refuses a run that
	// ended in this process).
	if err := writeSealedJSON(st.manifestPath(runID), m); err != nil {
		t.Fatal(err)
	}
}

// began waits until a run's manifest says it got its slot (no longer queued):
// a run orphaned before that never ran, and is taken up from its beginning.
func began(t *testing.T, st *runStore, runID string) {
	t.Helper()
	eventually(t, "begun", func() bool {
		m, err := st.readManifest(runID)
		return err == nil && !m.Queued
	})
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

// A run waiting on its event when its process ends is rebuilt by the next
// process under the same id, waits again (holding no slot), and finishes when
// its event comes; what was kept for it is then removed.
func TestAWaitingRunSurvivesARestart(t *testing.T) {
	dir := scratchDir(t)
	first := durableService(t, dir)
	ctx := context.Background()
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g", Target: "mod.obj"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	if !exists(first.runs.manifestPath(r.RunID)) || !exists(first.runs.checkpointPath(r.RunID)) {
		t.Fatal("a waiting run keeps its manifest and its checkpoint")
	}
	orphan(t, first.runs, r.RunID) // the first process is gone

	starts := waiterStarts.Load()
	second := durableService(t, dir)
	rep := second.RestoreRuns(ctx)
	if len(rep.Resumed) != 1 || rep.Resumed[0] != r.RunID || len(rep.Interrupted) != 0 {
		t.Fatalf("restore = %+v, want %s resumed", rep, r.RunID)
	}
	if st := status(t, second, r.RunID); st != taskapi.RunStatusWaiting {
		t.Fatalf("restored run = %s, want waiting", st)
	}
	if l := groupLoad(t, second, "g"); l.Running != 0 || l.Waiting != 1 {
		t.Errorf("restored run holds a slot: %+v", l)
	}
	runs, _ := second.Runs(ctx, taskapi.ListRunsRequest{})
	if got := runsByID(runs.Runs)[r.RunID]; got.Target != "mod.obj" || got.Group != "g" {
		t.Errorf("restored run lost what it was: %+v", got)
	}

	// Restored, the run already listens: its event, published once, is heard.
	_ = second.channel.Bus().Publish("wake."+r.RunID, map[string]any{})
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	res, err := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCompleted) {
		t.Fatalf("after its event: %+v, %v; want completed", res, err)
	}
	if n := waiterStarts.Load() - starts; n != 0 {
		t.Errorf("the restored run started over (%d first steps); want it resumed where it waited", n)
	}
	eventually(t, "its files removed", func() bool {
		return !exists(second.runs.manifestPath(r.RunID)) && !exists(second.runs.checkpointPath(r.RunID))
	})
}

// A run that was running (not waiting) is not run again: it is reported
// failed, interrupted by the restart, and its Result says so at once.
func TestARunningRunIsReportedInterrupted(t *testing.T) {
	dir := scratchDir(t)
	first := durableService(t, dir)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(first, release)
	ctx := context.Background()
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "running", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusRunning })
	began(t, first.runs, r.RunID) // its manifest written: an orphan before that is rewritten by its own process
	if exists(first.runs.checkpointPath(r.RunID)) {
		t.Error("a running run keeps no checkpoint")
	}
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	rep := second.RestoreRuns(ctx)
	if len(rep.Interrupted) != 1 || len(rep.Resumed) != 0 {
		t.Fatalf("restore = %+v, want it interrupted", rep)
	}
	if st := status(t, second, r.RunID); st != taskapi.RunStatusFailed {
		t.Errorf("status = %s, want failed", st)
	}
	rctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	res, err := second.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || !strings.HasPrefix(res.Error, ErrInterruptedByRestart.Error()+": ") {
		t.Errorf("result = %+v, %v; want the restart error at once, with its cause", res, err)
	}
	if exists(second.runs.manifestPath(r.RunID)) {
		t.Error("an interrupted run's manifest stays")
	}
}

// Not restored: an agent's child (its parent, restored, decides) and a run
// whose process is still alive.
func TestWhatARestartDoesNotResume(t *testing.T) {
	dir := scratchDir(t)
	first := durableService(t, dir)
	ctx := context.Background()

	child, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", ParentRunID: "run-gone"})
	if err != nil {
		t.Fatal(err)
	}
	alive, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{child.RunID, alive.RunID} {
		eventually(t, "waiting", func() bool { return status(t, first, id) == taskapi.RunStatusWaiting })
	}
	orphan(t, first.runs, child.RunID)
	// alive keeps this process's owner: it lives.

	second := durableService(t, dir)
	rep := second.RestoreRuns(ctx)
	if len(rep.Resumed) != 0 {
		t.Errorf("resumed %v, want none", rep.Resumed)
	}
	if len(rep.Interrupted) != 1 || rep.Interrupted[0] != child.RunID {
		t.Errorf("interrupted %v, want the agent's child only, not the live one", rep.Interrupted)
	}
	if !exists(second.runs.manifestPath(alive.RunID)) {
		t.Error("a live process's run was taken")
	}
	wake(t, first, alive.RunID, child.RunID)
}

// A wait's timeout counts from when it was saved: what is left after a
// restart, an expired one at once. A step that does not wait drops the
// checkpoint.
func TestAWaitKeepsItsDeadline(t *testing.T) {
	st, err := newRunStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.close) // its owner lock open would keep the folder
	save := func(id string, ago time.Duration) {
		cp := engine.Checkpoint{State: engine.State{Name: "w", RunID: id}, Wait: &engine.WaitSpec{Mode: engine.WaitAny, Timeout: time.Hour,
			Conditions: []engine.WaitCondition{{Topic: "x"}}}}
		if err := writeJSONAtomic(st.checkpointPath(id), savedCheckpoint{SavedAt: time.Now().Add(-ago), Checkpoint: cp}); err != nil {
			t.Fatal(err)
		}
	}
	save("half", 30*time.Minute)
	save("gone", 2*time.Hour)
	if cp, _ := st.Load("half"); cp.Wait.Timeout > 31*time.Minute || cp.Wait.Timeout < 29*time.Minute {
		t.Errorf("half: timeout %v, want ~30m left", cp.Wait.Timeout)
	}
	if cp, _ := st.Load("gone"); cp.Wait.Timeout != time.Nanosecond {
		t.Errorf("gone: timeout %v, want 1ns (expired)", cp.Wait.Timeout)
	}

	if err := st.Save("r", engine.Checkpoint{Wait: &engine.WaitSpec{Mode: engine.WaitAny, Conditions: []engine.WaitCondition{{Topic: "x"}}}}); err != nil {
		t.Fatal(err)
	}
	if !exists(st.checkpointPath("r")) {
		t.Fatal("a waiting step keeps its checkpoint")
	}
	if err := st.Save("r", engine.Checkpoint{}); err != nil {
		t.Fatal(err)
	}
	if exists(st.checkpointPath("r")) {
		t.Error("a step that does not wait drops the checkpoint")
	}
}

// ServeRestores says when its first sweep is done — the other serving hooks
// (the planner) start then, never on runs not yet restored.
func TestServeRestoresSaysWhenItsFirstSweepIsDone(t *testing.T) {
	dir := scratchDir(t)
	m, _ := leftRun(t, dir)
	second := durableService(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	swept := make(chan bool, 1)
	go second.ServeRestores(ctx, func() {
		_, _, known := second.reg.snapshot(m.RunID)
		swept <- known
	})
	select {
	case known := <-swept:
		if !known {
			t.Error("said done before the left run was restored")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("never said its first sweep was done")
	}
}
