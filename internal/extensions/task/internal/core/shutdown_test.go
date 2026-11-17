// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A clean stop of the serving process pauses its runs: each keeps its
// manifest and checkpoint, and the next start takes it up — a working run
// restarts from its last step (nothing before it done again), a waiting run
// waits again.
func TestACleanStopPausesTheServingProcessesRuns(t *testing.T) {
	release := resetStepper(t)
	defer release()
	dir := scratchDir(t)
	ctx := context.Background()
	first := durableService(t, dir)
	first.servingProc.on.Store(true) // the serving process (ServeRestores marks it)
	useStepper(t, first)
	working, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stepper", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "working in its last step", func() bool { return stepped("work2") == 1 })
	waiting, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g2"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, waiting.RunID) == taskapi.RunStatusWaiting })

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{working.RunID, waiting.RunID} {
		m, err := first.runs.readManifest(id)
		if err != nil || m.Ended != "" || m.Stopped != "" {
			t.Fatalf("manifest of %s after a clean stop: %+v (%v); want it kept, not ended", id, m, err)
		}
	}

	second := durableService(t, dir)
	useStepper(t, second)
	rep := second.RestoreRuns(ctx)
	if len(rep.Restarted) != 1 || rep.Restarted[0] != working.RunID || len(rep.Resumed) != 1 || rep.Resumed[0] != waiting.RunID {
		t.Fatalf("restore %+v; want the working run restarted and the waiting one resumed", rep)
	}
	eventually(t, "the last step again", func() bool { return stepped("work2") == 2 })
	if stepped("init") != 1 || stepped("work1") != 1 {
		t.Errorf("steps init=%d work1=%d; a paused run restarts from its last step", stepped("init"), stepped("work1"))
	}
	wake(t, second, waiting.RunID)
}

// Quitting a process that is not the serving one (a CLI run, a TUI) stops its
// runs: they end cancelled and the serving process never restarts them.
func TestQuittingAProcessStopsItsRuns(t *testing.T) {
	dir := scratchDir(t)
	ctx := context.Background()
	cli := durableService(t, dir)
	r, err := cli.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, cli, r.RunID) == taskapi.RunStatusWaiting })
	if err := cli.Close(); err != nil {
		t.Fatal(err)
	}
	if got := status(t, cli, r.RunID); got != taskapi.RunStatusCancelled {
		t.Errorf("status after quitting %q; cancelled", got)
	}

	serving := durableService(t, dir)
	rep := serving.RestoreRuns(ctx)
	if len(rep.Resumed)+len(rep.Restarted) > 0 {
		t.Fatalf("restore %+v; a run whose process quit was taken up again", rep)
	}
}

// A closing task service refuses new runs; closing twice is harmless.
func TestAClosingServiceRefusesNewRuns(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("closing twice: %v", err)
	}
	if _, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter"}); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Run after Close: %v; want ErrShuttingDown", err)
	}
}

func init() { shutdownGrace = 3 * time.Second } // the tests' runs end at once

// A run still queued is ended or paused like any run: paused by a clean stop
// (the next start takes it up), failed by its deadline, and when stopped its
// end is kept and told on its run's topic (a parent waiting on it hears it).
func TestAQueuedRunEndsLikeAnyRun(t *testing.T) {
	ctx := context.Background()
	queued := func(t *testing.T, svc *Service, in taskapi.TaskRunRequest) string {
		t.Helper()
		release := make(chan struct{})
		t.Cleanup(func() { close(release) })
		useStubborn(svc, release)
		holder(t, svc, "g")
		in.TaskName, in.Group = "waiter", "g"
		r, err := svc.Run(ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		eventually(t, "queued", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusQueued })
		return r.RunID
	}

	t.Run("paused by a clean stop", func(t *testing.T) {
		dir := scratchDir(t)
		first := durableService(t, dir)
		first.servingProc.on.Store(true)
		id := queued(t, first, taskapi.TaskRunRequest{})
		if err := first.Close(); err != nil {
			t.Fatal(err)
		}
		if m, err := first.runs.readManifest(id); err != nil || m.Ended != "" || m.Stopped != "" {
			t.Fatalf("manifest after a clean stop: %+v (%v); want it kept, not ended", m, err)
		}
		second := durableService(t, dir)
		rep := second.RestoreRuns(ctx)
		taken := false
		for _, r := range append(rep.Restarted, rep.Resumed...) {
			taken = taken || r == id
		}
		if !taken {
			t.Errorf("restore %+v; want the queued run taken up", rep)
		}
	})

	t.Run("failed by its deadline", func(t *testing.T) {
		svc := durableService(t, scratchDir(t))
		id := queued(t, svc, taskapi.TaskRunRequest{MaxDurationS: 1})
		eventually(t, "ended", func() bool { return status(t, svc, id) == taskapi.RunStatusFailed })
		info, _, _ := svc.reg.snapshot(id)
		if !strings.Contains(info.Error, ErrDeadlineExceeded.Error()) {
			t.Errorf("error %q; want it out of time", info.Error)
		}
	})

	t.Run("stopped: its end kept and told", func(t *testing.T) {
		svc := durableService(t, scratchDir(t))
		id := queued(t, svc, taskapi.TaskRunRequest{})
		told := make(chan struct{}, 1)
		unsub := svc.channel.Bus().Subscribe(engine.RunEndedTopic(id), func(string, any) { told <- struct{}{} })
		defer unsub()
		if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: id}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-told:
		case <-time.After(3 * time.Second):
			t.Fatal("a queued run's stop was not told on its run's topic")
		}
		if res, err := svc.runs.loadResult(id); err != nil || res.Status != string(taskapi.RunStatusCancelled) {
			t.Errorf("kept result %+v (%v); want cancelled", res, err)
		}
	})
}

// Result says how a stopped run ended as Status and Runs do: cancelled — a
// delegating parent reading it is not told the run failed.
func TestResultOfAStoppedRunIsCancelled(t *testing.T) {
	ctx := context.Background()
	svc := durableService(t, scratchDir(t))
	r, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	res, err := svc.Result(rctx, taskapi.ResultRequest{RunID: r.RunID})
	if err != nil || res.Status != string(taskapi.RunStatusCancelled) {
		t.Errorf("Result of a stopped run: %+v (%v); want cancelled", res, err)
	}
}
