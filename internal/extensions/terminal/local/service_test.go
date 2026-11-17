// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	"github.com/katyoti/katyoti-fwk/internal/extensions/terminal/api"
)

// Unit tests for handler shape — validation + idempotency.
//
// These tests do NOT exercise ServeTask (it has heavy Application
// dependencies). End-to-end behavior is verified by the smoke test that
// spawns a real ktw-agents binary; see TestSmoke_TerminalSlot in this
// directory's smoke_test.go file.

func newTestService(t *testing.T) *service {
	t.Helper()
	return &service{
		runs: map[string]*run{},
	}
}

// StartTask without an Application bound (test bypassed the factory)
// must fail cleanly rather than panic. The factory normally enforces a
// non-nil Application; tests that build a `service` directly should not
// reach the bus-resolution path.
func TestStartTask_RequiresAppBound(t *testing.T) {
	s := newTestService(t)
	_, err := s.StartTask(context.Background(), api.StartTaskInput{TaskName: "noop"})
	if err == nil {
		t.Fatalf("expected error when no Application bound to service")
	}
	if !strings.Contains(err.Error(), "Application") {
		t.Errorf("err message should mention Application, got: %v", err)
	}
}

// StartTask without a taskName must fail validation.
func TestStartTask_RequiresTaskName(t *testing.T) {
	s := newTestService(t)
	_, err := s.StartTask(context.Background(), api.StartTaskInput{})
	if err == nil {
		t.Fatalf("expected error for missing taskName")
	}
	if !strings.Contains(err.Error(), "taskName") {
		t.Errorf("err message should mention taskName, got: %v", err)
	}
}

// Cmd against an unknown runID must error (not silently no-op) so the
// client sees that its command wasn't routed.
func TestCmd_UnknownRunID(t *testing.T) {
	s := newTestService(t)
	_, err := s.Cmd(context.Background(), api.CmdInput{
		RunID:   "ghost",
		Command: json.RawMessage(`{"type":"cancel_child"}`),
	})
	if err == nil {
		t.Fatalf("expected error for unknown runID")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err should mention the runID, got: %v", err)
	}
}

// Cmd with empty command must fail validation rather than reach the
// transport with garbage.
func TestCmd_RequiresCommand(t *testing.T) {
	s := newTestService(t)
	_, err := s.Cmd(context.Background(), api.CmdInput{
		RunID:   "anything",
		Command: nil,
	})
	if err == nil {
		t.Fatalf("expected error for empty command")
	}
}

// Cancel against an unknown runID must succeed (idempotent) — the user
// might cancel after the run already completed; the server-side Done
// notification race is normal.
func TestCancel_IdempotentOnUnknownRun(t *testing.T) {
	s := newTestService(t)
	_, err := s.Cancel(context.Background(), api.CancelInput{RunID: "ghost", Reason: "test"})
	if err != nil {
		t.Errorf("Cancel on unknown runID should be idempotent (no error), got: %v", err)
	}
}

// runID generator returns 16 hex chars and stays unique across many calls.
func TestNewRunID_Format(t *testing.T) {
	id, err := newRunID()
	if err != nil {
		t.Fatalf("newRunID: %v", err)
	}
	if len(id) != 16 {
		t.Errorf("runID len = %d, want 16", len(id))
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char %q in runID", string(c))
			break
		}
	}
}

func TestNewRunID_Unique(t *testing.T) {
	const n = 200
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id, err := newRunID()
		if err != nil {
			t.Fatalf("[%d] newRunID: %v", i, err)
		}
		if seen[id] {
			t.Fatalf("[%d] duplicate runID: %s", i, id)
		}
		seen[id] = true
	}
}

// service.lookup / service.remove behave correctly under add/remove cycle.
func TestService_LookupRemove(t *testing.T) {
	s := newTestService(t)
	if got := s.lookup("nope"); got != nil {
		t.Errorf("lookup on empty map should return nil")
	}
	r := &run{id: "abc"}
	s.runs["abc"] = r
	if got := s.lookup("abc"); got != r {
		t.Errorf("lookup returned %v, want %v", got, r)
	}
	s.remove("abc")
	if got := s.lookup("abc"); got != nil {
		t.Errorf("lookup after remove should return nil")
	}
}

// A run that could not start (its model's key not set) says in its Done the
// error's message key, so the GUI says it in the person's language.
func TestDoneNotificationCarriesTheErrorsMessageKey(t *testing.T) {
	err := fmt.Errorf("terminal: run task: %w", usermsg.Wrap(errors.New("key not set"), "agent.modelKeyMissing", map[string]any{"model": "foundry-sonnet"}))
	done := doneNotification("r1", err)
	if done.RunID != "r1" || done.Error != err.Error() || done.Msg == nil || done.Msg.Key != "agent.modelKeyMissing" || done.Msg.Params["model"] != "foundry-sonnet" {
		t.Fatalf("a failed run's Done is %+v", done)
	}
	if done := doneNotification("r1", nil); done.Error != "" || done.Msg != nil {
		t.Fatalf("a run that ended well's Done is %+v", done)
	}
}
