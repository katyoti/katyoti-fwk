// SPDX-License-Identifier: Apache-2.0

package loopsafety

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// attrs builds a ToolAttrs lookup from two name sets.
func attrs(polling, idle []string) ToolAttrs {
	return attrsB(polling, idle, nil)
}

// attrsB builds a ToolAttrs lookup including the blocking set.
func attrsB(polling, idle, blocking []string) ToolAttrs {
	has := func(set []string, name string) bool {
		for _, n := range set {
			if n == name {
				return true
			}
		}
		return false
	}
	return func(name string) (bool, bool, bool) {
		return has(polling, name), has(idle, name), has(blocking, name)
	}
}

func call(t *testing.T, p *Plugin, tool string, input map[string]any) *api.ToolDecision {
	t.Helper()
	td, err := p.onPreToolExec(nil, tool, input)
	if err != nil {
		t.Fatalf("hook error: %v", err)
	}
	return td
}

// TestLoopSafety_DefaultThreshold pins the default 3-strikes guard for
// arbitrary tools — the third identical call is denied.
func TestLoopSafety_DefaultThreshold(t *testing.T) {
	p := NewPlugin(false, nil)
	input := map[string]any{"x": 1}
	if call(t, p, "file-write", input).Decision == api.Deny {
		t.Fatal("1st call should not trip")
	}
	if call(t, p, "file-write", input).Decision == api.Deny {
		t.Fatal("2nd call should not trip")
	}
	td := call(t, p, "file-write", input)
	if td.Decision != api.Deny {
		t.Fatal("3rd identical call should be denied")
	}
	if !strings.Contains(td.Reason, "file-write") {
		t.Errorf("deny reason should name the tool: %q", td.Reason)
	}
}

// TestLoopSafety_PollingToolThreshold pins the relaxed 10-strikes guard
// for tools that declare ToolDef.Polling.
func TestLoopSafety_PollingToolThreshold(t *testing.T) {
	p := NewPlugin(false, attrs([]string{"task-status"}, nil))
	input := map[string]any{"id": "abc"}
	for i := 1; i < 10; i++ {
		if call(t, p, "task-status", input).Decision == api.Deny {
			t.Fatalf("call %d should not trip (threshold 10)", i)
		}
	}
	if call(t, p, "task-status", input).Decision != api.Deny {
		t.Fatal("10th identical poll should be denied")
	}
}

// TestLoopSafety_BlockingExemption pins the blocking-tool exemption: a tool
// that BLOCKS until a real external event (await-child) is self-rate-limiting
// and must NEVER trip the guard, regardless of mode or call count — unlike a
// non-blocking Polling tool which keeps its threshold.
func TestLoopSafety_BlockingExemption(t *testing.T) {
	// exemptIdle=false (worker mode) — the exemption must hold without keepalive.
	p := NewPlugin(false, attrsB(nil, nil, []string{"await-child"}))
	input := map[string]any{"task_id": "child-1"}
	for i := 0; i < 100; i++ {
		if call(t, p, "await-child", input).Decision == api.Deny {
			t.Fatalf("blocking tool tripped on call %d — must be exempt", i)
		}
	}
}

// TestLoopSafety_IdleKeepAliveExemption pins the chat-loop exemption:
// an Idle-declared tool (ask-user) is the canonical "yield to user"
// signal in keepalive mode. The guard must NEVER fire for it there, no
// matter how many identical calls accumulate.
func TestLoopSafety_IdleKeepAliveExemption(t *testing.T) {
	p := NewPlugin(true, attrs(nil, []string{"ask-user"}))
	emptyQ := map[string]any{"question": ""}
	genericQ := map[string]any{"question": "Anything else?"}

	for i := 0; i < 1000; i++ {
		if call(t, p, "ask-user", emptyQ).Decision == api.Deny {
			t.Fatalf("keepalive idle tool should be exempt; tripped on call %d", i)
		}
	}
	for i := 0; i < 50; i++ {
		if call(t, p, "ask-user", genericQ).Decision == api.Deny {
			t.Fatalf("keepalive idle tool (generic args) should be exempt; tripped on call %d", i)
		}
	}
}

// TestLoopSafety_IdleNonChatStillGuarded confirms one-shot runs (no
// keepalive) keep the safety net on idle tools — three identical asks
// from a non-chat run almost certainly means the loop is stuck.
func TestLoopSafety_IdleNonChatStillGuarded(t *testing.T) {
	p := NewPlugin(false, attrs(nil, []string{"ask-user"}))
	q := map[string]any{"question": "What's the path?"}
	if call(t, p, "ask-user", q).Decision == api.Deny {
		t.Fatal("1st call should not trip")
	}
	if call(t, p, "ask-user", q).Decision == api.Deny {
		t.Fatal("2nd call should not trip")
	}
	if call(t, p, "ask-user", q).Decision != api.Deny {
		t.Fatal("3rd identical non-chat ask should be denied at default threshold")
	}
}

// TestLoopSafety_KeepAliveDoesNotExemptOtherTools confirms the
// exemption is keyed on (Idle attribute AND keepalive), not keepalive
// alone — a chat that goes haywire on file-write is still a runaway.
func TestLoopSafety_KeepAliveDoesNotExemptOtherTools(t *testing.T) {
	p := NewPlugin(true, attrs(nil, []string{"ask-user"}))
	input := map[string]any{"path": "/x"}
	if call(t, p, "file-write", input).Decision == api.Deny {
		t.Fatal("1st call should not trip")
	}
	if call(t, p, "file-write", input).Decision == api.Deny {
		t.Fatal("2nd call should not trip")
	}
	if call(t, p, "file-write", input).Decision != api.Deny {
		t.Fatal("3rd identical file-write under keepalive should still be denied")
	}
}

// TestLoopSafety_ArgumentOrderInsensitive: the canonical-JSON key means
// the model can't evade the guard by reordering fields.
func TestLoopSafety_ArgumentOrderInsensitive(t *testing.T) {
	p := NewPlugin(false, nil)
	call(t, p, "glob", map[string]any{"a": 1, "b": 2})
	call(t, p, "glob", map[string]any{"b": 2, "a": 1})
	if call(t, p, "glob", map[string]any{"a": 1, "b": 2}).Decision != api.Deny {
		t.Fatal("3rd semantically-identical call should be denied regardless of field order")
	}
}

// TestLoopSafety_DifferentArgsDontAccumulate: changing arguments resets
// nothing but counts separately — distinct work is never denied.
func TestLoopSafety_DifferentArgsDontAccumulate(t *testing.T) {
	p := NewPlugin(false, nil)
	for i := 0; i < 20; i++ {
		if call(t, p, "file-read", map[string]any{"path": i}).Decision == api.Deny {
			t.Fatalf("distinct call %d should never be denied", i)
		}
	}
}
