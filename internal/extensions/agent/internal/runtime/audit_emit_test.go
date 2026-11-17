// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"sync"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// I3 — verify the hooks publish via the audit emitter on every Deny.
//
// captureEmitter is a test-only AuditEmitter that records every event
// for assertion. Replaces the package-level emitter for the duration of
// each test via SetAuditEmitter(t.Cleanup-restored).

type captureEmitter struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (c *captureEmitter) EmitDeny(evt AuditEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, evt)
}

func (c *captureEmitter) takeAll() []AuditEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]AuditEvent, len(c.events))
	copy(out, c.events)
	return out
}

func withCapture(t *testing.T) *captureEmitter {
	t.Helper()
	cap := &captureEmitter{}
	prev := auditEmitter.Load()
	SetAuditEmitter(cap)
	t.Cleanup(func() {
		if prev == nil {
			SetAuditEmitter(nil)
		} else {
			SetAuditEmitter(*prev)
		}
	})
	return cap
}

func TestAuditEmit_SpawnPermissionHookDeny(t *testing.T) {
	cap := withCapture(t)

	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		DisallowedTools: []string{"bash"},
	})
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Fatalf("bash should be denied, got %v", d)
	}

	events := cap.takeAll()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Hook != "spawn_permission" {
		t.Errorf("Hook = %q, want spawn_permission", events[0].Hook)
	}
	if events[0].Tool != "bash" {
		t.Errorf("Tool = %q, want bash", events[0].Tool)
	}
}

func TestAuditEmit_FailClosedDenyEmits(t *testing.T) {
	cap := withCapture(t)

	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{}) // zero-value → fail-closed
	d, _ := hook.OnPreToolExec(nil, "file-write", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Fatalf("file_write should be fail-closed-denied, got %v", d)
	}

	events := cap.takeAll()
	if len(events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(events))
	}
	if events[0].Hook != "spawn_permission" {
		t.Errorf("Hook = %q, want spawn_permission", events[0].Hook)
	}
}

func TestAuditEmit_NoEmitOnAllow(t *testing.T) {
	cap := withCapture(t)

	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowAll: true,
	})
	if d, _ := hook.OnPreToolExec(nil, "bash", nil); d != nil {
		t.Errorf("AllowAll should permit, got %v", d)
	}
	if events := cap.takeAll(); len(events) != 0 {
		t.Errorf("Allow path should not emit; got %d events: %+v", len(events), events)
	}
}

func TestAuditEmit_NilEmitterIsNoOp(t *testing.T) {
	// Reset emitter to nil — runtime.emitAuditDeny must remain a no-op.
	prev := auditEmitter.Load()
	SetAuditEmitter(nil)
	t.Cleanup(func() {
		if prev == nil {
			SetAuditEmitter(nil)
		} else {
			SetAuditEmitter(*prev)
		}
	})

	// Should NOT panic.
	emitAuditDeny(AuditEvent{Hook: "test", Tool: "x", Reason: "y"})
}
