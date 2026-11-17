// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Goroutine/lock hygiene regression tests:
// kill-vs-start race, terminal-transition idempotency, and
// panic containment in the lifecycle goroutine.

func newBareBgManager() *bgManager {
	return NewBackgroundTaskManager(nil).(*bgManager)
}

func putPendingEntry(m *bgManager, taskID, parent string) *bgEntry {
	entry := &bgEntry{
		record: taskapi.TaskRecord{
			ID:          taskID,
			TaskName:    "hygiene-task",
			ParentRunID: parent,
			Status:      taskapi.TaskPending,
			StartedAt:   time.Now(),
		},
		done: make(chan struct{}),
	}
	m.mu.Lock()
	m.entries[taskID] = entry
	m.mu.Unlock()
	return entry
}

// TestBgManager_KillBeforeStart_RunNeverStarts — a Kill that lands before
// runLifecycle publishes Running must win: the lifecycle goroutine sees the
// terminal status and aborts WITHOUT starting the run. Pre-fix, the status
// flipped Killed→Running and the task executed with nobody holding its
// cancel func.
func TestBgManager_KillBeforeStart_RunNeverStarts(t *testing.T) {
	m := newBareBgManager()
	putPendingEntry(m, "t-kill-race", "parent-kr")

	if err := m.Kill("t-kill-race"); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	// taskSvc is nil — if runLifecycle got past the terminal guard it would
	// fail with "TaskService not available" (status Failed) instead of
	// preserving Killed.
	m.runLifecycle("t-kill-race", taskapi.TaskRunRequest{TaskName: "hygiene-task"})

	rec, err := m.TaskStatus("t-kill-race")
	if err != nil {
		t.Fatalf("TaskStatus: %v", err)
	}
	if rec.Status != taskapi.TaskKilled {
		t.Errorf("status = %q, want %q (Kill must win over a late start)", rec.Status, taskapi.TaskKilled)
	}
	if notifs := m.Notifications("parent-kr"); len(notifs) != 1 {
		t.Errorf("got %d notifications, want exactly 1 (the kill)", len(notifs))
	}
}

// TestBgManager_KillTwice_SingleNotification — terminal transitions are
// idempotent: a second Kill (or the lifecycle's ctx-cancelled path calling
// killTask again) must not enqueue a duplicate notification.
func TestBgManager_KillTwice_SingleNotification(t *testing.T) {
	m := newBareBgManager()
	putPendingEntry(m, "t-double-kill", "parent-dk")

	if err := m.Kill("t-double-kill"); err != nil {
		t.Fatalf("first Kill: %v", err)
	}
	if err := m.Kill("t-double-kill"); err != nil {
		t.Fatalf("second Kill: %v", err)
	}

	notifs := m.Notifications("parent-dk")
	if len(notifs) != 1 {
		t.Fatalf("got %d notifications, want exactly 1", len(notifs))
	}
	if notifs[0].Status != taskapi.TaskKilled {
		t.Errorf("notification status = %q, want %q", notifs[0].Status, taskapi.TaskKilled)
	}
}

// panickingTaskService panics inside Run — stands in for a machine factory
// or strategy blowing up after the lifecycle goroutine started.
type panickingTaskService struct{ taskapi.TaskService }

func (panickingTaskService) Run(_ context.Context, _ taskapi.TaskRunRequest) (taskapi.TaskRunResponse, error) {
	panic("boom-lifecycle-hygiene")
}

// TestBgManager_PanickedLifecycle_MarksFailed — a panic in the lifecycle
// goroutine must be recovered and converted into a Failed record + parent
// notification. Pre-fix the record was stranded in Running forever (and an
// unrecovered goroutine panic kills the process).
func TestBgManager_PanickedLifecycle_MarksFailed(t *testing.T) {
	m := newBareBgManager()
	m.taskSvc = panickingTaskService{}
	putPendingEntry(m, "t-panic", "parent-pn")

	m.runLifecycle("t-panic", taskapi.TaskRunRequest{TaskName: "hygiene-task"})

	rec, err := m.TaskStatus("t-panic")
	if err != nil {
		t.Fatalf("TaskStatus: %v", err)
	}
	if rec.Status != taskapi.TaskFailed {
		t.Fatalf("status = %q, want %q", rec.Status, taskapi.TaskFailed)
	}
	if !strings.Contains(rec.Error, "panicked") {
		t.Errorf("record error = %q, want it to mention the panic", rec.Error)
	}
	notifs := m.Notifications("parent-pn")
	if len(notifs) != 1 || notifs[0].Status != taskapi.TaskFailed {
		t.Errorf("notifications = %#v, want exactly one Failed", notifs)
	}
}

// TestBgManager_ConcurrentKillStatusProgress — hammer Kill / TaskStatus /
// Progress concurrently against a running lifecycle. Asserts nothing beyond
// "no deadlock, terminal status sticks" — its real job is giving the -race
// CI lane something to bite on for the cancel-write / record-read fences.
func TestBgManager_ConcurrentKillStatusProgress(t *testing.T) {
	m := newBareBgManager()
	putPendingEntry(m, "t-conc", "parent-cc")

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 50; i++ {
			_, _ = m.TaskStatus("t-conc")
			_, _ = m.Progress("t-conc")
		}
	}()
	go m.runLifecycle("t-conc", taskapi.TaskRunRequest{TaskName: "hygiene-task"}) // taskSvc nil → fails fast
	_ = m.Kill("t-conc")
	<-done

	rec, err := m.TaskStatus("t-conc")
	if err != nil {
		t.Fatalf("TaskStatus: %v", err)
	}
	if !isTerminalLifecycle(rec.Status) {
		t.Errorf("status = %q, want a terminal state", rec.Status)
	}
}
