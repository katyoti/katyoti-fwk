// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A run that ended is still listed after a restart — its status and why — and
// only the last ones are kept.
func TestAnEndedRunIsStillListedAfterARestart(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	first := durableService(t, dir)
	useMachine(first, failer{})
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "failer"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "failed", func() bool { return runInfo(t, first, r.RunID).Status == taskapi.RunStatusFailed })
	// Its status is set before its end is written: the new process reads the disk.
	eventually(t, "its end on disk", func() bool { return exists(filepath.Join(dir, "ended", r.RunID+".json")) })

	second := durableService(t, dir) // a new process
	useMachine(second, failer{})
	got := runInfo(t, second, r.RunID)
	if got.Status != taskapi.RunStatusFailed || !strings.Contains(got.Error, "the disk is full") || got.EndedAt == nil {
		t.Fatalf("after a restart Runs says %+v; want the run, failed, why and when", got)
	}

	second.runs.keepEnded = 2 // set before its runs start
	for i := 0; i < 3; i++ {
		f, err := second.Run(ctx, taskapi.TaskRunRequest{TaskName: "failer"})
		if err != nil {
			t.Fatal(err)
		}
		eventually(t, "failed", func() bool { return runInfo(t, second, f.RunID).Status == taskapi.RunStatusFailed })
	}
	eventually(t, "the oldest let go", func() bool {
		es, _ := os.ReadDir(filepath.Join(dir, "ended"))
		return len(es) == 2
	})
	third := durableService(t, dir)
	if n := len(third.runs.endedRuns()); n != 2 {
		t.Errorf("%d ended runs kept; want the last 2", n)
	}
}
