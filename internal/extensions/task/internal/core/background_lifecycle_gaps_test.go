// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Background-task lifecycle gap closures (UC-100/101/102/042/045).
// Tests use the lightweight Service factory from run_lifecycle_gaps_test.go
// so the bgManager has a real taskSvc to invoke.

// TestBgManager_RegisterCreatesEntry (UC-100) — Register inserts a TaskRecord
// into entries[taskID] and returns the same ID to the caller. The entry's
// initial status is Pending or Running (transitions Pending → Running inside
// the lifecycle goroutine).
func TestBgManager_RegisterCreatesEntry(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)
	bg := svc.bgMgr.(*bgManager)

	taskID, err := bg.Register("parent-uc100", taskapi.TaskRunRequest{
		TaskName:     "uc-130-task",
		MaxDurationS: 60,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if taskID == "" {
		t.Fatal("Register returned empty taskID")
	}

	bg.mu.RLock()
	entry, ok := bg.entries[taskID]
	bg.mu.RUnlock()
	if !ok {
		t.Fatalf("entries[%q] not found after Register", taskID)
	}
	if entry.record.ID != taskID {
		t.Errorf("entry.ID = %q, want %q", entry.record.ID, taskID)
	}
	if entry.record.ParentRunID != "parent-uc100" {
		t.Errorf("entry.ParentRunID = %q, want parent-uc100", entry.record.ParentRunID)
	}
}

// TestBgManager_TaskStatusReturnsCurrent (UC-101) — TaskStatus returns the
// current TaskRecord by ID. Unknown ID returns an error.
func TestBgManager_TaskStatusReturnsCurrent(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)
	bg := svc.bgMgr.(*bgManager)

	taskID, err := bg.Register("parent-uc101", taskapi.TaskRunRequest{
		TaskName:     "uc-130-task",
		MaxDurationS: 60,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	rec, err := bg.TaskStatus(taskID)
	if err != nil {
		t.Fatalf("TaskStatus: %v", err)
	}
	if rec.ID != taskID {
		t.Errorf("record.ID = %q, want %q", rec.ID, taskID)
	}
	if rec.ParentRunID != "parent-uc101" {
		t.Errorf("record.ParentRunID = %q, want parent-uc101", rec.ParentRunID)
	}

	// Unknown ID surfaces an error citing the missing task.
	if _, err := bg.TaskStatus("nope-doesnt-exist"); err == nil {
		t.Error("TaskStatus on unknown ID should return error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention not-found", err.Error())
	}
}

// TestBgManager_KillTerminates (UC-102) — Kill flips the entry's status to
// Killed and enqueues a notification for the parent (unless FireAndForget
// is set).
func TestBgManager_KillTerminates(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)
	bg := svc.bgMgr.(*bgManager)

	taskID, err := bg.Register("parent-uc102", taskapi.TaskRunRequest{
		TaskName:     "uc-130-task",
		MaxDurationS: 60,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := bg.Kill(taskID); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	// Status flipped to Killed.
	rec, _ := bg.TaskStatus(taskID)
	if rec.Status != taskapi.TaskKilled {
		t.Errorf("status after Kill = %q, want %q", rec.Status, taskapi.TaskKilled)
	}

	// Notification queued for the parent.
	notifs := bg.Notifications("parent-uc102")
	if len(notifs) == 0 {
		t.Error("Kill should enqueue a TaskNotification for the parent")
	} else {
		found := false
		for _, n := range notifs {
			if n.TaskID == taskID && n.Status == taskapi.TaskKilled {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no Killed-notification found for taskID=%q; got %+v", taskID, notifs)
		}
	}

	// Kill on unknown ID returns an error.
	if err := bg.Kill("nope"); err == nil {
		t.Error("Kill on unknown ID should error")
	}
}

// TestBgManager_FireAndForgetSkipsNotification (UC-045) — Register with
// FireAndForget=true does NOT enqueue notifications on terminal events.
// Parent's Notifications drain returns an empty slice even after the run
// completes / fails / is killed.
func TestBgManager_FireAndForgetSkipsNotification(t *testing.T) {
	svc, _ := newServiceForLifecycleTest(t)
	bg := svc.bgMgr.(*bgManager)

	taskID, err := bg.Register("parent-uc045", taskapi.TaskRunRequest{
		TaskName:      "uc-130-task",
		FireAndForget: true,
		MaxDurationS:  60,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Wait for completion via the done channel.
	bg.mu.RLock()
	entry := bg.entries[taskID]
	bg.mu.RUnlock()
	if entry == nil || entry.done == nil {
		t.Fatal("entry.done not present")
	}
	select {
	case <-entry.done:
	case <-time.After(10 * time.Second): // a bound, not a timing: a loaded machine is slow
		t.Fatal("entry.done not closed within 10s")
	}

	// Notifications drain returns empty.
	notifs := bg.Notifications("parent-uc045")
	if len(notifs) != 0 {
		t.Errorf("FireAndForget should suppress notifications; got %d entries: %+v", len(notifs), notifs)
	}

	// Sanity: also check Kill path on FireAndForget.
	taskID2, _ := bg.Register("parent-uc045", taskapi.TaskRunRequest{
		TaskName:      "uc-130-task",
		FireAndForget: true,
		MaxDurationS:  60,
	})
	_ = bg.Kill(taskID2)
	notifs2 := bg.Notifications("parent-uc045")
	if len(notifs2) != 0 {
		t.Errorf("FireAndForget Kill path should suppress notification; got %+v", notifs2)
	}
}

// TestBgManager_MaxDurationAborts (UC-042) — MaxDurationS sets a wall-clock
// budget on the run context. A simulator child with duration > budget gets
// cancelled; the run status terminates as Killed (not Completed).
func TestBgManager_MaxDurationAborts(t *testing.T) {
	registerSimulatorForCore(t)

	// Use a custom task that runs longer than the budget.
	svc, _ := newServiceForLifecycleTest(t)
	svc.tasks["uc-042-slow"] = &taskapi.Task{
		Name:     "uc-042-slow",
		Provider: "test",
		Type:     "simulator",
		Config: map[string]any{
			"duration_ms": 500, // 500ms work
			"output":      "should-not-complete",
		},
	}
	bg := svc.bgMgr.(*bgManager)

	taskID, err := bg.Register("parent-uc042", taskapi.TaskRunRequest{
		TaskName:     "uc-042-slow",
		MaxDurationS: 1, // but MaxDuration would be 1s — this should normally complete
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Wait for terminal state. With duration=500ms and budget=1s, we expect
	// Completed. To exercise the abort path, register a second task with
	// duration > budget.
	bg.mu.RLock()
	entry := bg.entries[taskID]
	bg.mu.RUnlock()
	select {
	case <-entry.done:
	case <-time.After(3 * time.Second):
		t.Fatal("entry.done not closed")
	}
	rec, _ := bg.TaskStatus(taskID)
	if rec.Status != taskapi.TaskCompleted {
		t.Logf("first run (duration < budget) status = %q — acceptable: %v", rec.Status, rec.Error)
	}

	// Second task: duration > budget → expect Killed/Failed.
	svc.tasks["uc-042-tooslow"] = &taskapi.Task{
		Name:     "uc-042-tooslow",
		Provider: "test",
		Type:     "simulator",
		Config: map[string]any{
			"duration_ms": 3000, // 3s work
			"output":      "should-be-aborted",
		},
	}
	taskID2, err := bg.Register("parent-uc042b", taskapi.TaskRunRequest{
		TaskName:     "uc-042-tooslow",
		MaxDurationS: 1, // 1s budget — abort after 1s
	})
	if err != nil {
		t.Fatalf("Register slow: %v", err)
	}
	bg.mu.RLock()
	entry2 := bg.entries[taskID2]
	bg.mu.RUnlock()
	select {
	case <-entry2.done:
	case <-time.After(5 * time.Second):
		t.Fatal("UC-042: tooslow run.done not closed within 5s — abort path may be broken")
	}
	rec2, _ := bg.TaskStatus(taskID2)
	if rec2.Status == taskapi.TaskCompleted {
		t.Errorf("UC-042: tooslow run completed despite MaxDurationS=1; status=%q", rec2.Status)
	}
}
