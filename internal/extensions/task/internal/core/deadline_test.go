// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A run's maximum duration is a deadline, counted from when it was submitted,
// waiting included: a run waiting for ever is ended at it, and says why.
// (TaskService.Run ignored max_duration_s: only background delegates had one.)
func TestARunEndsAtItsDeadline(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter", MaxDurationS: 1})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed at its deadline", func() bool { return runInfo(t, svc, r.RunID).Status == taskapi.RunStatusFailed })
	if e := runInfo(t, svc, r.RunID).Error; !strings.Contains(e, "max_duration_s 1") {
		t.Errorf("error = %q; want it to say it ran out of time", e)
	}
}

// A restored run keeps its deadline (from its first start); one whose deadline
// passed while the process was down is reported interrupted, and why.
func TestARestoredRunKeepsItsDeadline(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	first := durableService(t, dir)
	// Margins wide enough for a loaded machine: 6 s to live, down 2 s, so at
	// most 4 s are left at the restore — a new deadline would be 6 s.
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g", MaxDurationS: 6})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)
	time.Sleep(2 * time.Second) // the process is down a while
	second := durableService(t, dir)
	if rep := second.RestoreRuns(ctx); len(rep.Resumed) != 1 {
		t.Fatalf("restore = %+v; want it resumed", rep)
	}
	start := time.Now()
	eventually(t, "failed at its deadline", func() bool { return runInfo(t, second, r.RunID).Status == taskapi.RunStatusFailed })
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("ended %s after its restore; want at its first deadline (at most 4 s left), not a new one (6 s)", waited)
	}

	late := durableService(t, scratchDir(t))
	r2, err := late.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g", MaxDurationS: 1})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, late, r2.RunID) == taskapi.RunStatusWaiting })
	orphan(t, late.runs, r2.RunID)
	ms, _ := late.runs.manifests()
	if len(ms) != 1 {
		t.Fatalf("manifests = %d", len(ms))
	}
	ms[0].StartedAt = time.Now().Add(-time.Hour) // its deadline passed while down
	if err := late.runs.saveManifest(ms[0]); err != nil {
		t.Fatal(err)
	}
	again := durableService(t, late.runs.dir)
	if rep := again.RestoreRuns(ctx); len(rep.Interrupted) != 1 {
		t.Fatalf("restore = %+v; want it interrupted", rep)
	}
	if e := runInfo(t, again, r2.RunID).Error; !strings.Contains(e, "deadline") || !strings.Contains(e, "down") {
		t.Errorf("error = %q; want its deadline passed while the process was down", e)
	}
}
