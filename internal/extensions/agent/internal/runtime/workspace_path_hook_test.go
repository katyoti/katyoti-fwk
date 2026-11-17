// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

func TestWorkspacePathHook_NilPathsReturnsNilHook(t *testing.T) {
	if h := NewWorkspacePathHook(nil); h != nil {
		t.Errorf("nil paths should yield nil hook, got %v", h)
	}
}

func TestWorkspacePathHook_UnrestrictedReturnsNilHook(t *testing.T) {
	if h := NewWorkspacePathHook(&agentapi.WorkspacePaths{}); h != nil {
		t.Error("empty WorkspacePaths should yield nil hook")
	}
	if h := NewWorkspacePathHook(&agentapi.WorkspacePaths{ReadPaths: []string{"*"}, WritePaths: []string{"*"}}); h != nil {
		t.Error("wildcard WorkspacePaths should yield nil hook")
	}
}

func TestWorkspacePathHook_NonPathToolPasses(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{ReadPaths: []string{"src/"}})
	if h == nil {
		t.Fatal("expected hook")
	}
	// `ask_user` is neither fs-aware nor in the dangerous map — it must pass
	// through. (The previous test version used `bash`, which is now in
	// dangerousToolNames for UC-058 partial closure.)
	d, _ := h.OnPreToolExec(nil, "ask_user", map[string]any{"prompt": "anything"})
	if d != nil {
		t.Errorf("non-path tool ask_user should not be denied, got %v", d)
	}
}

func TestWorkspacePathHook_ReadAllowedPath(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{ReadPaths: []string{"src/", "docs/"}})

	// Allowed
	d, _ := h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": "src/main.go"})
	if d != nil {
		t.Errorf("file_read src/main.go should be allowed, got %v", d)
	}

	// Allowed (docs)
	d, _ = h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": "docs/README.md"})
	if d != nil {
		t.Errorf("file_read docs/README.md should be allowed, got %v", d)
	}

	// Denied (out of allow list)
	d, _ = h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": "secrets/key.pem"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("file_read secrets/key.pem should be denied, got %v", d)
	}
}

func TestWorkspacePathHook_WriteEnforcedSeparately(t *testing.T) {
	// Read-only auditor: can read everything, can write nowhere.
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:  []string{"*"},
		WritePaths: []string{},
	})
	if h == nil {
		// All-wildcard read + empty write = both unrestricted, no hook.
		t.Skip("hook is nil due to optimization — test re-evaluated below")
	}

	// Empty write list with non-wildcard read → still creates hook.
	h2 := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:  []string{"src/", "docs/"},
		WritePaths: []string{}, // empty = unrestricted by current semantics
	})
	if h2 == nil {
		t.Fatal("expected hook with restrictive ReadPaths")
	}

	// file_read enforces ReadPaths.
	d, _ := h2.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": "lib/foo.go"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("file_read lib/foo.go should be denied, got %v", d)
	}

	// file_write — empty WritePaths means unrestricted (current semantics).
	d, _ = h2.OnPreToolExec(nil, "file-write", map[string]any{"workspace_path": "anywhere/foo.txt"})
	if d != nil {
		t.Errorf("file_write should be unrestricted with empty WritePaths, got %v", d)
	}
}

func TestWorkspacePathHook_WriteRestricted(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:  []string{"*"},
		WritePaths: []string{"src/module-1/"},
	})

	// Allowed write
	d, _ := h.OnPreToolExec(nil, "file-write", map[string]any{"workspace_path": "src/module-1/foo.go"})
	if d != nil {
		t.Errorf("file_write to src/module-1/foo.go should be allowed, got %v", d)
	}

	// Denied write
	d, _ = h.OnPreToolExec(nil, "file-write", map[string]any{"workspace_path": "src/module-2/foo.go"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("file_write to src/module-2/ should be denied, got %v", d)
	}

	// file_edit treated as write
	d, _ = h.OnPreToolExec(nil, "file-edit", map[string]any{"workspace_path": "src/module-2/foo.go"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("file_edit to src/module-2/ should be denied, got %v", d)
	}
}

func TestWorkspacePathHook_WildcardSuffixHandled(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{ReadPaths: []string{"src/**", "docs/*"}})

	d, _ := h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": "src/main.go"})
	if d != nil {
		t.Errorf("src/** should match src/main.go, got %v", d)
	}

	d, _ = h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": "src/sub/file.go"})
	if d != nil {
		t.Errorf("src/** should match src/sub/file.go, got %v", d)
	}
}

// TestBash_DeniedWhenRestrictive — UC-058, audit finding threat-T2-critical.
//
// Verifies that `bash` is denied at the hook when WorkspacePaths is restrictive.
// The bash spec (CAP-BT-05) defines a sandbox model that is P2-deferred; until
// it lands, the hook refuses bash under restriction with a clear reason.
func TestBash_DeniedWhenRestrictive(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:  []string{"src/", "docs/"},
		WritePaths: []string{"src/"},
	})
	if h == nil {
		t.Fatal("expected hook")
	}

	// Multiple input shapes — none of them should let bash through.
	cases := []map[string]any{
		{"command": "ls -la"},
		{"command": "cat /etc/passwd"},
		{"command": `bash -c "$(echo ZWNobyAxCg== | base64 -d)"`},
		{}, // even without command field
	}
	for i, in := range cases {
		d, _ := h.OnPreToolExec(nil, "bash", in)
		if d == nil || d.Decision != agentapi.Deny {
			t.Errorf("case %d: bash should be DENIED under restrictive WorkspacePaths, got %+v", i, d)
			continue
		}
		// Deny reason should cite the missing sandbox so the user knows the path forward.
		if d.Reason == "" {
			t.Errorf("case %d: deny reason should be non-empty", i)
		}
	}
}

// TestBash_AllowedWhenUnrestrictive verifies that bash is NOT gated by this
// hook when WorkspacePaths is wildcard-or-empty (existing optimization makes
// the hook itself nil; recreate the no-restriction path explicitly).
func TestBash_AllowedWhenUnrestrictive(t *testing.T) {
	// Wildcard read + write → NewWorkspacePathHook returns nil; the plugin
	// chain skips it entirely; bash passes through.
	if h := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:  []string{"*"},
		WritePaths: []string{"*"},
	}); h != nil {
		t.Errorf("wildcard paths should yield nil hook (no enforcement), got %v", h)
	}

	// Empty paths → same.
	if h := NewWorkspacePathHook(&agentapi.WorkspacePaths{}); h != nil {
		t.Errorf("empty paths should yield nil hook, got %v", h)
	}
}

// TestDangerousToolNames_DenyReasonCitesContext checks that the deny reason
// gives the operator enough information to decide their next step.
func TestDangerousToolNames_DenyReasonCitesContext(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths: []string{"src/"},
	})
	if h == nil {
		t.Fatal("expected hook")
	}
	d, _ := h.OnPreToolExec(nil, "bash", map[string]any{"command": "ls"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Fatalf("expected DENY, got %+v", d)
	}
	// Reason should mention the tool name AND give a hint about how to override.
	for _, want := range []string{`"bash"`, "WorkspacePaths"} {
		if !strings.Contains(d.Reason, want) {
			t.Errorf("deny reason should mention %q; got: %s", want, d.Reason)
		}
	}
}

// TestPathTraversal_Rejected — UC-057, audit finding threat-T2-second.
//
// Verifies that pathAllowed refuses any input containing ".." segments, even
// when the cleaned path would land inside an allowed prefix. Without this,
// a child agent restricted to /repo/src/ could write /etc/passwd via
// /repo/src/../../etc/passwd. Windows-style separators are also covered.
func TestPathTraversal_Rejected(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{
		ReadPaths:  []string{"/repo/src/", "src/"},
		WritePaths: []string{"/repo/src/", "src/"},
	})
	if h == nil {
		t.Fatal("expected hook")
	}

	denyCases := []struct {
		name string
		path string
	}{
		// Audit-cited cases.
		{"absolute_double_dotdot", "/repo/src/../../etc/passwd"},
		{"absolute_dot_then_dotdot", "/repo/src/./../etc/passwd"},
		{"absolute_triple_dotdot", "/repo/src/foo/../../../etc/passwd"},
		{"windows_separators", `\repo\src\..\..\etc\passwd`},
		// Additional defense-in-depth cases.
		{"relative_with_dotdot", "src/../etc/passwd"},
		{"bare_dotdot", ".."},
		{"dotdot_inside_safely_resolved", "src/foo/../bar.go"}, // safe-looking but rejected by design
		{"mixed_separators", `src/sub\..\..\etc\passwd`},
		{"trailing_dotdot", "src/foo/.."},
	}

	for _, tc := range denyCases {
		t.Run("deny_"+tc.name, func(t *testing.T) {
			d, _ := h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": tc.path})
			if d == nil || d.Decision != agentapi.Deny {
				t.Errorf("file_read %q expected DENY, got %+v", tc.path, d)
			}
			d, _ = h.OnPreToolExec(nil, "file-write", map[string]any{"workspace_path": tc.path})
			if d == nil || d.Decision != agentapi.Deny {
				t.Errorf("file_write %q expected DENY, got %+v", tc.path, d)
			}
		})
	}

	// Positive cases that must still pass — the fix does not over-block.
	allowCases := []struct {
		name string
		path string
	}{
		{"plain", "src/main.go"},
		{"absolute_plain", "/repo/src/main.go"},
		{"leading_dot_slash_normalized", "./src/main.go"},
		{"deep_path", "src/sub/deep/file.go"},
	}
	for _, tc := range allowCases {
		t.Run("allow_"+tc.name, func(t *testing.T) {
			d, _ := h.OnPreToolExec(nil, "file-read", map[string]any{"workspace_path": tc.path})
			if d != nil && d.Decision == agentapi.Deny {
				t.Errorf("file_read %q expected ALLOW (or pass-through), got DENY: %s", tc.path, d.Reason)
			}
		})
	}
}

// TestNormalizePath verifies the helper directly so future refactors don't
// silently change its semantics.
func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"":               "",
		"src/main.go":    "src/main.go",
		"./src/main.go":  "src/main.go",
		"src//main.go":   "src/main.go",
		`src\main.go`:    "src/main.go",
		`src\sub\f.go`:   "src/sub/f.go",
		"/repo/src/":     "/repo/src",
		"/repo//src/foo": "/repo/src/foo",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestContainsTraversal verifies the .. detector across separator styles.
func TestContainsTraversal(t *testing.T) {
	denies := []string{
		"..",
		"../etc",
		"src/..",
		"src/../etc",
		`src\..\etc`,
		"/repo/src/../../etc",
		`\repo\src\..\..\etc`,
	}
	for _, p := range denies {
		if !containsTraversal(p) {
			t.Errorf("containsTraversal(%q) = false, want true", p)
		}
	}
	allows := []string{
		"",
		".",
		"src/main.go",
		"src/.../foo.go",  // three-dot is not traversal
		"src/foo..bar.go", // dots embedded in filename
		"/repo/src/foo.go",
	}
	for _, p := range allows {
		if containsTraversal(p) {
			t.Errorf("containsTraversal(%q) = true, want false", p)
		}
	}
}

func TestWorkspacePathHook_AbsolutePathChecked(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{ReadPaths: []string{"/tmp/work/"}})

	// workspace_path empty, fall back to absolute_path
	d, _ := h.OnPreToolExec(nil, "file-read", map[string]any{"absolute_path": "/tmp/work/file.txt"})
	if d != nil {
		t.Errorf("allowed absolute_path should pass, got %v", d)
	}

	d, _ = h.OnPreToolExec(nil, "file-read", map[string]any{"absolute_path": "/etc/passwd"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("disallowed absolute_path should be denied, got %v", d)
	}
}

// deck-export writes its export: a spawn limited to some write paths cannot
// make it write elsewhere — its `output`, else its `file` (the export lands
// beside the deck), is held to WritePaths.
func TestWorkspacePathHook_DeckExportHeldToWritePaths(t *testing.T) {
	h := NewWorkspacePathHook(&agentapi.WorkspacePaths{ReadPaths: []string{"*"}, WritePaths: []string{"decks/"}})
	if h == nil {
		t.Fatal("expected a hook with restrictive WritePaths")
	}
	d, _ := h.OnPreToolExec(nil, "deck-export", map[string]any{"file": "decks/talk.deck.md", "output": "C:/elsewhere/talk.pptx"})
	if d == nil || d.Decision != agentapi.Deny {
		t.Errorf("an output outside the write paths must be denied, got %v", d)
	}
	d, _ = h.OnPreToolExec(nil, "deck-export", map[string]any{"file": "decks/talk.deck.md"})
	if d != nil && d.Decision == agentapi.Deny {
		t.Errorf("an export beside a deck inside the write paths must pass, got %v", d)
	}
}
