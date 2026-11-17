// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

func runInfo(t *testing.T, svc *Service, id string) taskapi.RunInfo {
	t.Helper()
	out, err := svc.Runs(context.Background(), taskapi.ListRunsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return runsByID(out.Runs)[id]
}

// Runs says why a run failed, was killed or was interrupted — it said "failed"
// and nothing more.
func TestRunsSaysWhyARunEnded(t *testing.T) {
	old := KillAfter
	KillAfter = 30 * time.Millisecond
	t.Cleanup(func() { KillAfter = old })
	ctx := context.Background()
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	useMachine(svc, failer{})
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	useStubborn(svc, release)

	f, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "failer"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed", func() bool { return runInfo(t, svc, f.RunID).Status == taskapi.RunStatusFailed })
	if e := runInfo(t, svc, f.RunID).Error; !strings.Contains(e, "the disk is full") {
		t.Errorf("failed run's error = %q; want why", e)
	}

	before := stubbornActions.Load()
	k, err := svc.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "its action running", func() bool { return stubbornActions.Load() > before })
	if _, err := svc.Stop(ctx, taskapi.StopRequest{RunID: k.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "killed", func() bool { return runInfo(t, svc, k.RunID).Status == taskapi.RunStatusKilled })
	if e := runInfo(t, svc, k.RunID).Error; !strings.Contains(e, "did not stop") {
		t.Errorf("killed run's error = %q; want why", e)
	}

	dir := scratchDir(t)
	first := durableService(t, dir)
	useStubborn(first, release)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "running", func() bool { return groupLoad(t, first, "g").Running == 1 })
	began(t, first.runs, r.RunID)
	orphan(t, first.runs, r.RunID)
	second := durableService(t, dir)
	useStubborn(second, release)
	second.RestoreRuns(ctx)
	if e := runInfo(t, second, r.RunID).Error; !strings.Contains(e, "restart") || !strings.Contains(e, "not restarted") {
		t.Errorf("interrupted run's error = %q; want interrupted by a restart, and why", e)
	}
}
