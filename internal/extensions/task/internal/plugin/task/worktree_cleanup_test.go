// SPDX-License-Identifier: Apache-2.0

package task

import (
	"strings"
	"sync"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/worktree"
)

type endedChild struct{ taskapi.BackgroundTaskManager }

func (endedChild) TaskStatus(id string) (*taskapi.TaskRecord, error) {
	return &taskapi.TaskRecord{ID: id, Status: taskapi.TaskCompleted}, nil
}

// A child's worktree that could not be removed when it ended is a warning on
// the parent's run, with why — it was kept silently.
func TestAKeptWorktreeIsAWarning(t *testing.T) {
	dir := t.TempDir()
	wt := worktree.NewManager(dir, worktree.Config{BaseDir: dir})
	var mu sync.Mutex
	var got []string
	scheduleWorktreeCleanup(endedChild{}, wt, "child-1", "child-1", func(r string) {
		mu.Lock()
		got = append(got, r)
		mu.Unlock()
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || !strings.Contains(got[0], "child-1") || !strings.Contains(got[0], "kept") {
		t.Errorf("warnings = %q; want the kept worktree, with why", got)
	}
}
