// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A run that works but takes no step for too long is reported stuck — once,
// in Runs and as an event — and not killed; a waiting run is not stuck.
func TestARunThatNoLongerMovesIsReportedStuck(t *testing.T) {
	oldAfter := stuckAfter
	stuckAfter = 50 * time.Millisecond
	t.Cleanup(func() { stuckAfter = oldAfter })
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
	w, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working", func() bool { return stubbornActions.Load() > before })
	eventually(t, "waiting", func() bool { return status(t, svc, w.RunID) == taskapi.RunStatusWaiting })
	time.Sleep(100 * time.Millisecond)
	svc.watchStuck()
	svc.watchStuck() // told once per stall

	if got := runInfo(t, svc, r.RunID); got.StuckSince == nil || got.Status != taskapi.RunStatusRunning {
		t.Errorf("run = %+v; want it running and stuck since its last step", got)
	}
	n := 0
	for _, e := range log.of(r.RunID) {
		if e.Transition == engine.RunStuck {
			n++
		}
	}
	if n != 1 {
		t.Errorf("stuck told %d times; want once", n)
	}
	if runInfo(t, svc, w.RunID).StuckSince != nil || log.has(w.RunID, engine.RunStuck) {
		t.Error("a waiting run was reported stuck")
	}
}

// Its next step clears it; the watchdog looks by itself, every watchdogEvery.
func TestAStuckRunMovingAgainIsNoLongerStuck(t *testing.T) {
	oldAfter, oldEvery := stuckAfter, watchdogEvery
	stuckAfter, watchdogEvery = 30*time.Millisecond, 20*time.Millisecond
	t.Cleanup(func() { stuckAfter, watchdogEvery = oldAfter, oldEvery })
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	log := lifecycleOf(svc)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.runWatchdog(ctx)

	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "told stuck by the watchdog", func() bool { return log.has(r.RunID, engine.RunStuck) })
	svc.stepped(r.RunID)
	if runInfo(t, svc, r.RunID).StuckSince != nil {
		t.Error("still stuck after a step")
	}
}
