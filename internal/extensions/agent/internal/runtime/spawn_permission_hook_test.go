// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

func TestSpawnPermissionHook_NilProducesFailClosedHook(t *testing.T) {
	// R4 fail-closed: nil perms is promoted to a zero-value SpawnPermissions{}
	// so the dangerous-tool gate fires. The hook is always constructed; nil is
	// not silently bypassed.
	hook := NewSpawnPermissionHook(nil)
	if hook == nil {
		t.Fatal("NewSpawnPermissionHook(nil) should produce a fail-closed hook, not nil")
	}

	// A dangerous tool is denied.
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("nil perms should fail-closed-deny bash, got %v", d)
	}

	// A safe tool is allowed (profile gate decides further).
	if d, _ := hook.OnPreToolExec(nil, "file-read", nil); d != nil {
		t.Errorf("nil perms should still allow file_read, got %v", d)
	}
}

func TestSpawnPermissionHook_WildcardAllowsAll(t *testing.T) {
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowedTools: []string{"*"},
	})
	decision, err := hook.OnPreToolExec(nil, "any_tool", nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if decision != nil {
		t.Errorf("wildcard should allow (nil decision), got %v", decision)
	}
}

func TestSpawnPermissionHook_AllowList(t *testing.T) {
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowedTools: []string{"file-read", "glob"},
	})

	// Allowed tool → nil decision (allow).
	if d, _ := hook.OnPreToolExec(nil, "file-read", nil); d != nil {
		t.Errorf("file_read should be allowed, got %v", d)
	}

	// Not in allow list → Deny.
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bash should be denied, got %v", d)
	}
}

func TestSpawnPermissionHook_DenyList(t *testing.T) {
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		DisallowedTools: []string{"bash", "web-fetch"},
	})

	// In deny list → Deny.
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bash should be denied, got %v", d)
	}

	// Not in deny list → Allow (nil decision).
	if d, _ := hook.OnPreToolExec(nil, "file-read", nil); d != nil {
		t.Errorf("file_read should be allowed, got %v", d)
	}
}

func TestSpawnPermissionHook_DenyListPrecedesAllowList(t *testing.T) {
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowedTools:    []string{"*"},
		DisallowedTools: []string{"bash"},
	})
	// bash is allowed by wildcard but denied by list → deny wins.
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("deny list should take precedence, got %v", d)
	}
}

func TestSpawnPermissionHook_RestrictedMode(t *testing.T) {
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		PermissionMode: "restricted",
	})

	// Safe tool → allow.
	if d, _ := hook.OnPreToolExec(nil, "file-read", nil); d != nil {
		t.Errorf("file_read should be allowed in restricted mode, got %v", d)
	}
	if d, _ := hook.OnPreToolExec(nil, "complete", nil); d != nil {
		t.Errorf("complete should be allowed in restricted mode, got %v", d)
	}

	// Unsafe tool → deny.
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bash should be denied in restricted mode, got %v", d)
	}
	d, _ = hook.OnPreToolExec(nil, "web-fetch", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("web_fetch should be denied in restricted mode, got %v", d)
	}
}

func TestSpawnPermissionHook_DefaultModeFailClosedForDangerousTools(t *testing.T) {
	// R4 fail-closed: default mode with empty AllowedTools used to allow every
	// tool. Now dangerous tools are denied unless AllowAll is set. Safe tools
	// (file_read, glob, complete, etc.) remain allowed.
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		PermissionMode: "default",
	})
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bash should be fail-closed-denied in default mode without AllowAll, got %v", d)
	}
	if d, _ := hook.OnPreToolExec(nil, "file-read", nil); d != nil {
		t.Errorf("file_read should still be allowed under fail-closed default, got %v", d)
	}
}

func TestSpawnPermissionHook_AllowAllBypassesFailClosed(t *testing.T) {
	// R4 explicit opt-out: AllowAll bypasses the dangerous-tool gate. Used by
	// top-level interactive user runs where the profile gate is the next layer.
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowAll: true,
	})
	for _, tool := range []string{"bash", "rest-api-fetch", "web-fetch", "file-write", "file-edit", "delegate", "spawn-teammate"} {
		if d, _ := hook.OnPreToolExec(nil, tool, nil); d != nil {
			t.Errorf("AllowAll should bypass fail-closed for %q, got %v", tool, d)
		}
	}
}

func TestSpawnPermissionHook_AllowAllStillHonorsDisallowedTools(t *testing.T) {
	// AllowAll does NOT override explicit DisallowedTools — caller intent wins.
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowAll:        true,
		DisallowedTools: []string{"bash"},
	})
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bash should be denied by DisallowedTools even under AllowAll, got %v", d)
	}
	// Other dangerous tools pass under AllowAll.
	if d, _ := hook.OnPreToolExec(nil, "file-write", nil); d != nil {
		t.Errorf("file_write should pass under AllowAll, got %v", d)
	}
}

func TestSpawnPermissionHook_AllowedToolsBypassesFailClosed(t *testing.T) {
	// Explicit AllowedTools = caller declared intent. file_write is dangerous
	// but the caller listed it → respect their intent (step 2 won't trigger
	// step 4).
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		AllowedTools: []string{"file-read", "file-write"},
	})
	if d, _ := hook.OnPreToolExec(nil, "file-write", nil); d != nil {
		t.Errorf("file_write listed in AllowedTools should pass, got %v", d)
	}
	// A dangerous tool NOT in the list is denied by step 2 (allow-list mismatch),
	// not by step 4 — but the user-visible outcome is the same.
	d, _ := hook.OnPreToolExec(nil, "bash", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bash not in AllowedTools should be denied, got %v", d)
	}
}

// TestSpawnPermissionHook_BubbleModeDeniesAtHook — defense in depth.
// The factory rejects bubble at attach time (R9 path B); if a caller ever
// constructs the hook directly with PermissionMode=bubble and bypasses the
// factory, the hook now denies at the OnPreToolExec call site rather than
// silently allowing (the historic no-op behavior).
func TestSpawnPermissionHook_BubbleModeDeniesAtHook(t *testing.T) {
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{
		PermissionMode: "bubble",
		// Set AllowedTools wildcard so the deny isn't coming from step 2,
		// and set a non-dangerous tool so step 4 doesn't fire — we want to
		// verify step 5 catches bubble independently of the other gates.
		AllowedTools: []string{"*"},
	})
	d, _ := hook.OnPreToolExec(nil, "file-read", nil)
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("bubble mode should be denied (not implemented yet); got %v", d)
	}
}

func TestSpawnPermissionHook_FailClosedAllowsReadOnlyAndDeniesWrite(t *testing.T) {
	// Zero-value perms with the fail-closed gate active. The dangerous-tool
	// list covers tools that can write outside the workspace or execute code;
	// genuinely read-only tools pass.
	//
	// Note: SafeToolSubset (for PermissionMode=restricted) includes file_write
	// + file_edit because coordinator-spawned workers need them. Under
	// fail-closed default (no PermissionMode declared), those same write tools
	// are denied — the caller must declare AllowedTools or set AllowAll to
	// signal intent.
	hook := NewSpawnPermissionHook(&agentapi.SpawnPermissions{})

	allowed := []string{"file-read", "glob", "grep", "complete", "task_status", "check-messages", "send_message"}
	for _, tool := range allowed {
		if d, _ := hook.OnPreToolExec(nil, tool, nil); d != nil {
			t.Errorf("non-dangerous tool %q should pass under fail-closed default, got %v", tool, d)
		}
	}

	denied := []string{"file-write", "file-edit", "bash", "rest-api-fetch", "web-fetch", "delegate", "spawn-teammate"}
	for _, tool := range denied {
		d, _ := hook.OnPreToolExec(nil, tool, nil)
		if d == nil || d.Decision != agentapi.Deny {
			t.Errorf("dangerous tool %q should be denied under fail-closed default, got %v", tool, d)
		}
	}
}

func TestSafeToolSubset_Complete(t *testing.T) {
	// Ensure safe subset matches the documented list.
	expected := map[string]bool{"file-read": true, "file-write": true, "file-edit": true, "glob": true, "complete": true}
	if len(SafeToolSubset) != len(expected) {
		t.Errorf("SafeToolSubset size = %d, want %d", len(SafeToolSubset), len(expected))
	}
	for _, tool := range SafeToolSubset {
		if !expected[tool] {
			t.Errorf("unexpected tool in safe subset: %q", tool)
		}
	}
}
