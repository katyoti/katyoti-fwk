// SPDX-License-Identifier: Apache-2.0

package task

import (
	"fmt"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/worktree"
)

// scheduleWorktreeCleanup removes the worktree when the background task
// finishes. Polls TaskStatus every second until a terminal state is reached.
//
// Polling here is ACCEPTED (not an oversight): the goroutine's lifetime is
// bounded on two independent axes — background spawns require MaxDurationS,
// so the watched task always terminates; and bg-entry TTL eviction makes
// TaskStatus return an error after the terminal record is pruned, which
// exits the loop even if the terminal tick was missed. Worst case lifetime:
// task duration + retention TTL. Replacing the poll with a child-lifecycle
// subscription is part of the taskplugin decomposition work.
//
// If the child still has uncommitted changes, RemoveWorktree (without force)
// preserves the worktree so the user can inspect/merge. A manual cleanup or
// the periodic CleanupStaleWorktrees scanner will eventually remove it.
//
// A worktree kept (its changes uncommitted, or it could not be removed) is a
// warning on the parent's run, with why. Noop if either the manager or the
// bgMgr is nil.
func scheduleWorktreeCleanup(bgMgr taskapi.BackgroundTaskManager, wtMgr *worktree.Manager, taskID, slug string, warn func(string)) {
	if bgMgr == nil || wtMgr == nil || taskID == "" || slug == "" {
		return
	}
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			rec, err := bgMgr.TaskStatus(taskID)
			if err != nil || rec == nil {
				return
			}
			if isTerminal(rec.Status) {
				// Don't force — if the child left uncommitted changes, keep the worktree.
				if err := wtMgr.RemoveWorktree(slug, false); err != nil && warn != nil {
					warn(fmt.Sprintf("the worktree %s of child %s was kept: %v", slug, taskID, err))
				}
				return
			}
		}
	}()
}

func isTerminal(s taskapi.TaskLifecycle) bool {
	return s == taskapi.TaskCompleted || s == taskapi.TaskFailed || s == taskapi.TaskKilled
}
