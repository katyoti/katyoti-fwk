// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// newTestSuspensionManager builds a bare suspensionManager — suspend/resume
// tests exercise only the wakeup-channel bookkeeping, no scheduler.
func newTestSuspensionManager(t *testing.T) taskapi.SuspensionManager {
	t.Helper()
	return newSuspensionManager()
}

// TestSuspend_ReturnsChannel_ResumeDelivers pins the basic
// round-trip — Suspend gives us a channel, Resume delivers the
// payload to the receiver.
func TestSuspend_ReturnsChannel_ResumeDelivers(t *testing.T) {
	sm := newTestSuspensionManager(t)

	ch, err := sm.Suspend("task-A", "cwm:!workspace/object/abc/resolved")
	if err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	if ch == nil {
		t.Fatal("Suspend returned nil channel")
	}

	want := json.RawMessage(`{"answer":"yes"}`)
	if err := sm.Resume("task-A", want); err != nil {
		t.Fatalf("Resume: %v", err)
	}

	select {
	case got := <-ch:
		if string(got) != string(want) {
			t.Errorf("got %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resume payload")
	}
}

// TestSuspend_EmptyWaitURI_Errors guards against accidental empty
// waitURI calls that would create an undebuggable suspension.
func TestSuspend_EmptyWaitURI_Errors(t *testing.T) {
	sm := newTestSuspensionManager(t)
	if _, err := sm.Suspend("task-A", ""); err == nil {
		t.Error("expected error for empty waitURI")
	}
}

// A wait with no task id is refused: its answer could never reach it (it
// waited for ever under a made-up id, a log line saying so).
func TestSuspend_EmptyTaskID_IsRefused(t *testing.T) {
	sm := newTestSuspensionManager(t)
	ch, err := sm.Suspend("", "cwm:!workspace/object/x/resolved")
	if err == nil || ch != nil || !strings.Contains(err.Error(), "no task id") {
		t.Fatalf("Suspend with no task id = %v, %v; want it refused, saying why", ch, err)
	}
	if snaps := sm.Suspensions(); len(snaps) != 0 {
		t.Errorf("got %d suspensions, want none", len(snaps))
	}
}

// TestResume_UnknownTask_Errors pins the safety guard — Resume for
// a non-suspended task surfaces a clear error.
func TestResume_UnknownTask_Errors(t *testing.T) {
	sm := newTestSuspensionManager(t)
	err := sm.Resume("never-suspended", json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error resuming unknown task")
	}
}

// TestResume_Idempotent confirms double-Resume is safe — second
// call returns nil rather than panicking on a buffered-channel
// overflow. Matters because Phase E will have EventService
// handlers that might fire twice on subject overlap.
func TestResume_Idempotent(t *testing.T) {
	sm := newTestSuspensionManager(t)
	ch, _ := sm.Suspend("task-A", "cwm:!workspace/object/x/resolved")
	if err := sm.Resume("task-A", json.RawMessage(`"first"`)); err != nil {
		t.Fatalf("first Resume: %v", err)
	}
	// Drain so the second resume can't accidentally succeed via the
	// buffer slot.
	<-ch
	if err := sm.Resume("task-A", json.RawMessage(`"second"`)); err != nil {
		t.Errorf("second Resume should be a no-op (idempotent); got %v", err)
	}
}

// TestSuspend_RepeatedReturnsSameChannel pins the shared-wakeup
// contract — calling Suspend twice with the same taskID returns
// the same channel, NOT a new one (the ask-user in-flight guard
// already prevents concurrent calls in the chat path, but the
// contract matters for programmatic callers).
func TestSuspend_RepeatedReturnsSameChannel(t *testing.T) {
	sm := newTestSuspensionManager(t)
	ch1, _ := sm.Suspend("task-A", "cwm:!workspace/object/x/resolved")
	ch2, _ := sm.Suspend("task-A", "cwm:!workspace/object/y/resolved")
	if ch1 != ch2 {
		t.Error("repeated Suspend with same taskID should return same channel")
	}
}

// TestSuspensions_SnapshotShape confirms the snapshot exposes
// {taskID, waitURI, startedAt} for each in-flight suspension and
// excludes delivered ones.
func TestSuspensions_SnapshotShape(t *testing.T) {
	sm := newTestSuspensionManager(t)
	sm.Suspend("task-A", "cwm:!workspace/object/a/resolved")
	sm.Suspend("task-B", "cwm:!workspace/object/b/resolved")

	snaps := sm.Suspensions()
	if len(snaps) != 2 {
		t.Fatalf("got %d snapshots, want 2", len(snaps))
	}
	ids := map[string]bool{}
	for _, s := range snaps {
		ids[s.TaskID] = true
		if s.WaitURI == "" {
			t.Errorf("snapshot for %q has empty WaitURI", s.TaskID)
		}
		if s.StartedAt.IsZero() {
			t.Errorf("snapshot for %q has zero StartedAt", s.TaskID)
		}
	}
	if !ids["task-A"] || !ids["task-B"] {
		t.Errorf("missing expected task IDs; got %v", ids)
	}

	// Resume task-A → snapshot drops it.
	if err := sm.Resume("task-A", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	after := sm.Suspensions()
	if len(after) != 1 || after[0].TaskID != "task-B" {
		t.Errorf("after Resume snapshot should hold only task-B; got %+v", after)
	}
}

// TestSuspend_Concurrent_NoRace exercises the mutex under N
// goroutines suspending + resuming. Race-free correctness here
// matters because every chat ask-user invocation goes through
// this path eventually.
func TestSuspend_Concurrent_NoRace(t *testing.T) {
	sm := newTestSuspensionManager(t)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			tid := jsonString("task-", i)
			ch, err := sm.Suspend(tid, "cwm:!workspace/object/x/resolved")
			if err != nil {
				t.Errorf("Suspend %d: %v", i, err)
				return
			}
			// Resume from another goroutine so the receiver actually
			// blocks-then-wakes (not just buffer-fill).
			go func() {
				_ = sm.Resume(tid, json.RawMessage(`"ok"`))
			}()
			select {
			case <-ch:
				// good
			case <-time.After(2 * time.Second):
				t.Errorf("task %d: timed out waiting for resume", i)
			}
		}()
	}
	wg.Wait()
}

// jsonString is a tiny helper that avoids fmt for a trivial join —
// pulls in less dependency surface in the test.
func jsonString(prefix string, i int) string {
	out := prefix
	if i < 10 {
		out += string(rune('0' + i))
	} else {
		out += string(rune('a' + (i - 10)))
	}
	return out
}
