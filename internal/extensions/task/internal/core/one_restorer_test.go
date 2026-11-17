// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// ONE RESTORER — a run left by a process that is gone is restored by exactly
// one process; a process that is alive is never taken for gone.

// leftRun makes a waiting run whose process is gone, in dir; it returns the
// run's manifest as a process would have read it.
func leftRun(t *testing.T, dir string) (runManifest, *Service) {
	t.Helper()
	first := durableService(t, dir)
	r, err := first.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	orphan(t, first.runs, r.RunID)
	ms, err := first.runs.manifests()
	if err != nil || len(ms) != 1 {
		t.Fatalf("manifests = %v, %v", ms, err)
	}
	return ms[0], first
}

// Several serving processes sweeping at the same moment restore a gone
// process's run once: one resumes it, the others leave it alone — never two
// resumes (its agent and tools would run twice), never a resume and an
// interrupt (the files just written would be removed).
func TestARunLeftByAGoneProcessIsRestoredOnce(t *testing.T) {
	for round := 0; round < 10; round++ {
		dir := scratchDir(t)
		m, first := leftRun(t, dir)
		var svcs []*Service
		for i := 0; i < 4; i++ {
			svcs = append(svcs, durableService(t, dir))
		}
		start := make(chan struct{})
		reps := make([]RestoreReport, len(svcs))
		var wg sync.WaitGroup
		for i, s := range svcs {
			wg.Add(1)
			go func(i int, s *Service) {
				defer wg.Done()
				<-start
				reps[i] = s.RestoreRuns(context.Background())
			}(i, s)
		}
		close(start)
		wg.Wait()
		resumed, interrupted := 0, 0
		for _, r := range reps {
			resumed += len(r.Resumed)
			interrupted += len(r.Interrupted)
		}
		if resumed != 1 || interrupted != 0 {
			t.Fatalf("round %d: resumed %d times, interrupted %d times; want resumed once", round, resumed, interrupted)
		}
		if !exists(first.runs.manifestPath(m.RunID)) {
			t.Fatalf("round %d: the restored run's manifest is gone", round)
		}
		for _, s := range append(svcs, first) {
			wake(t, s, m.RunID)
		}
	}
}

// A process that read the run's manifest before another restored it does not
// restore it again: its claim on the gone owner finds the run owned anew.
func TestAStaleReadOfARestoredRunDoesNotRestoreItAgain(t *testing.T) {
	dir := scratchDir(t)
	m, first := leftRun(t, dir) // m: the manifest as read before any restore
	a := durableService(t, dir)
	if rep := a.RestoreRuns(context.Background()); len(rep.Resumed) != 1 {
		t.Fatalf("restore = %+v", rep)
	}
	b := durableService(t, dir)
	if release, ok := b.runs.claim(m); ok {
		release()
		t.Error("a stale read of a restored run won its claim: it would be restored twice")
	}
	wake(t, a, m.RunID)
	wake(t, first, m.RunID)
}

// A process that died while restoring a run (its claim left behind) does not
// block the run: the next process takes the claim over and restores it.
func TestAClaimLeftByAGoneRestorerIsTakenOver(t *testing.T) {
	dir := scratchDir(t)
	m, first := leftRun(t, dir)
	if err := os.WriteFile(first.runs.claimPath(m.RunID, m.Owner), []byte("crashed-restorer"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := durableService(t, dir)
	if rep := second.RestoreRuns(context.Background()); len(rep.Resumed) != 1 {
		t.Fatalf("restore = %+v; want the run restored over the gone restorer's claim", rep)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, m.RunID+".claim-*")); len(left) != 0 {
		t.Errorf("claims left behind: %v", left)
	}
	wake(t, second, m.RunID)
	wake(t, first, m.RunID)
}

// An owner is alive while its file is locked — however old the file, however
// long it slept: its runs are its own. Its lock released (it died), they are
// restored.
func TestAnOwnerLivesWhileItsLockIsHeld(t *testing.T) {
	dir := scratchDir(t)
	first := durableService(t, dir)
	r, err := first.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	file := first.runs.ownerPath(first.runs.owner)
	old := time.Now().Add(-time.Hour) // asleep for an hour: nothing written meanwhile
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}

	second := durableService(t, dir)
	if rep := second.RestoreRuns(context.Background()); len(rep.Resumed)+len(rep.Interrupted)+len(rep.Restarted) != 0 {
		t.Fatalf("restore = %+v; a live owner's runs were taken because its file is old", rep)
	}
	first.runs.close() // it dies
	if rep := second.RestoreRuns(context.Background()); len(rep.Resumed) != 1 {
		t.Errorf("restore = %+v; a gone owner's waiting run is resumed", rep)
	}
	wake(t, second, r.RunID)
	wake(t, first, r.RunID)
}
