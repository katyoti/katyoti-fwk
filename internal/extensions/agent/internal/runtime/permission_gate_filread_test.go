// SPDX-License-Identifier: Apache-2.0

package runtime

// End-to-end integration tests for the permission gate using the real
// file-read tool (not the synthetic permissionTestTool from
// loop_dispatcher_test.go). These tests cover the full path:
//
//   Dispatcher.LookupTool → tool.CheckPermissions → profile.DenyMatches
//   → tool.CheckPermissions returning Ask → profile.AllowMatches
//   → PromptPermission via fakeHumanGate → AllowAlways/DenyAlways
//   → session profile mutation
//
// They are the canonical proof that the rules system works end-to-end
// for the FIRST production tool that uses it. Any future file tool
// (file-write, file-edit, glob, grep) that adopts the same pattern
// gets covered by analogous tests in this style.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_read"
)

// fakeFileReadResolver implements file_read.PathResolver against a
// caller-provided workspace root. Used by every integration test
// below.
type fakeFileReadResolver struct {
	root string
}

func (r *fakeFileReadResolver) Resolve(relPath string) (string, error) {
	// Simple workspace-relative resolution. The integration tests
	// don't exercise the resolver's `..` and absolute-rejection
	// paths — those are covered by file-read's own validate_test.go.
	return filepath.Join(r.root, relPath), nil
}

func (r *fakeFileReadResolver) WorkspaceRoot() string { return r.root }

// realFileReadDispatcher builds a Dispatcher with a real file-read
// tool wired against a temp workspace, and returns the workspace root
// so the test can write files into it before running the gate.
func realFileReadDispatcher(t *testing.T) (*Dispatcher, string) {
	t.Helper()
	root := t.TempDir()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs tempdir: %v", err)
	}

	resolver := &fakeFileReadResolver{root: abs}
	tool := file_read.NewFileReadTool(resolver)

	d := NewDispatcher()
	d.RegisterPlugin(toolsapi.NewToolPlugin(tool))
	return d, abs
}

// writeFile is a tiny helper for setting up workspace fixtures.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// ── Workspace_path branch: silent allow, no prompt ──────────────────────

func TestFileReadIntegration_WorkspacePath_SilentAllow(t *testing.T) {
	d, root := realFileReadDispatcher(t)
	writeFile(t, filepath.Join(root, "src/main.go"), "package main\n")

	// Gate should NOT call PromptPermission for a workspace_path read.
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce} // would block if called
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshal(t, file_read.ReadInput{WorkspacePath: "src/main.go"})
	out := applyPermissionGate(ctx, d, "file-read", in)

	if !out.allow {
		t.Fatalf("allow = false, want true (workspace_path should be silent allow). errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate called %d times, want 0 (workspace_path should not prompt)", len(gate.permissionCalls))
	}
}

// ── Absolute_path inside workspace: silent allow ────────────────────────

func TestFileReadIntegration_AbsolutePath_InsideWorkspace_SilentAllow(t *testing.T) {
	d, root := realFileReadDispatcher(t)
	target := filepath.Join(root, "internal", "thing.txt")
	writeFile(t, target, "hello\n")

	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce}
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshal(t, file_read.ReadInput{AbsolutePath: target})
	out := applyPermissionGate(ctx, d, "file-read", in)

	if !out.allow {
		t.Fatalf("allow = false, want true (absolute inside workspace). errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate called %d times, want 0 (inside-workspace absolute is allowed silently)", len(gate.permissionCalls))
	}
}

// ── Absolute_path outside workspace: prompts via PromptPermission ──────

func TestFileReadIntegration_AbsolutePath_Outside_Prompts(t *testing.T) {
	d, _ := realFileReadDispatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	writeFile(t, target, "secret\n")

	// User picks Allow Once.
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshal(t, file_read.ReadInput{AbsolutePath: target})
	out := applyPermissionGate(ctx, d, "file-read", in)

	if !out.allow {
		t.Fatalf("allow = false, want true after AllowOnce. errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Fatalf("HumanGate called %d times, want 1", len(gate.permissionCalls))
	}

	// Verify the prompt carried the file-read suggestions through
	// (the rules system end-to-end check).
	req := gate.permissionCalls[0]
	if req.ToolName != "file-read" {
		t.Errorf("ToolName = %q, want file-read", req.ToolName)
	}
	if !strings.Contains(req.Question, "outside the workspace") {
		t.Errorf("Question = %q, want substring 'outside the workspace'", req.Question)
	}
	if len(req.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d, want 3 (exact / parent / any)", len(req.Suggestions))
	}
	// Sanity: the broadest suggestion should be the file-read.absolute_path:** wildcard.
	if req.Suggestions[2].Rule != "file-read.absolute_path:**" {
		t.Errorf("broad suggestion rule = %q, want file-read.absolute_path:**", req.Suggestions[2].Rule)
	}
}

// ── Profile pre-approval: outside-workspace read silenced by profile ───

func TestFileReadIntegration_AbsolutePath_Outside_ProfileAllows(t *testing.T) {
	d, _ := realFileReadDispatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "spec.md")
	writeFile(t, target, "# spec\n")

	// Profile pre-approves any file-read.absolute_path under
	// the outside dir. The prompt should NOT fire.
	//
	// Rules use forward slashes (gitignore convention); file-read
	// normalizes its dimension value to forward slashes too.
	outsideSlash := filepath.ToSlash(outside)
	profile := &permrules.Profile{
		Name: "test",
		AllowRules: []permrules.Rule{
			permrules.MustParseRule("file-read.absolute_path:" + outsideSlash + "/**"),
		},
	}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce} // would block if called
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in := mustMarshal(t, file_read.ReadInput{AbsolutePath: target})
	out := applyPermissionGate(ctx, d, "file-read", in)

	if !out.allow {
		t.Fatalf("allow = false, want true (profile should silence the ask). errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate called %d times, want 0 (profile pre-approval should skip prompt)", len(gate.permissionCalls))
	}
}

// ── Profile hard-deny: workspace_path blocked by profile (plan-mode) ───

func TestFileReadIntegration_PlanModeBlocksAllReads(t *testing.T) {
	// Simulate a plan-mode-style profile with file-read denied
	// outright. Even workspace_path reads should be blocked because
	// the profile deny rule fires BEFORE tool.CheckPermissions runs.
	//
	// Note: ms04-p1's plan-mode actually allows reads (it blocks
	// writes/edit/bash). This test uses a hypothetical "no-reads"
	// profile to verify the deny path works for file-read
	// regardless of which branch (workspace vs absolute) the LLM
	// uses. This proves profile deny is the floor.
	d, root := realFileReadDispatcher(t)
	writeFile(t, filepath.Join(root, "src/main.go"), "package main\n")

	profile := &permrules.Profile{
		Name:      "no-reads",
		DenyRules: []permrules.Rule{permrules.MustParseRule("file-read:**")},
	}
	profiles := &fakeProfiles{profile: profile}
	ctx := newAppCtxWithGateAndProfiles(t, nil, profiles)

	in := mustMarshal(t, file_read.ReadInput{WorkspacePath: "src/main.go"})
	out := applyPermissionGate(ctx, d, "file-read", in)

	if out.allow {
		t.Fatal("allow = true, want false (profile deny should block workspace_path)")
	}
	if !strings.Contains(out.errMsg, "blocked by permission profile") {
		t.Errorf("errMsg = %q, want 'blocked by permission profile'", out.errMsg)
	}
	if !strings.Contains(out.errMsg, "no-reads") {
		t.Errorf("errMsg = %q, should name the profile", out.errMsg)
	}
}

// ── AllowAlways: second call to the same path is silent ───────────────

func TestFileReadIntegration_AllowAlways_SecondCallSilent(t *testing.T) {
	d, _ := realFileReadDispatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "config.yaml")
	writeFile(t, target, "k: v\n")

	// First call: user picks Allow Always with the broadest scope
	// (the third suggestion → file-read.absolute_path:**).
	profile := &permrules.Profile{Name: "session"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionAllowAlways,
		selectedRule: "file-read.absolute_path:**",
	}
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in := mustMarshal(t, file_read.ReadInput{AbsolutePath: target})
	out1 := applyPermissionGate(ctx, d, "file-read", in)
	if !out1.allow {
		t.Fatalf("first call allow = false, want true. errMsg=%q", out1.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Fatalf("first call: HumanGate called %d times, want 1", len(gate.permissionCalls))
	}
	if len(profiles.addedAllow) != 1 {
		t.Fatalf("first call: addedAllow len = %d, want 1 (rule should be added)", len(profiles.addedAllow))
	}

	// Second call: same path. The session profile now has the
	// always-allow rule, so the gate should silence the prompt.
	target2 := filepath.Join(outside, "other.yaml")
	writeFile(t, target2, "k: v\n")
	in2 := mustMarshal(t, file_read.ReadInput{AbsolutePath: target2})
	out2 := applyPermissionGate(ctx, d, "file-read", in2)

	if !out2.allow {
		t.Fatalf("second call allow = false, want true (session rule should silence). errMsg=%q", out2.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Errorf("second call: HumanGate called %d times, want 1 (still just the first call's prompt)", len(gate.permissionCalls))
	}
}

// ── DenyAlways: second call to any matching path is silently blocked ──

func TestFileReadIntegration_DenyAlways_SecondCallSilentlyBlocked(t *testing.T) {
	d, _ := realFileReadDispatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "secret1.txt")
	writeFile(t, target, "x\n")

	// First call: user picks Deny Always with the synthesized
	// `file-read:**` scope.
	profile := &permrules.Profile{Name: "session"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionDenyAlways,
		selectedRule: "file-read:**",
	}
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in := mustMarshal(t, file_read.ReadInput{AbsolutePath: target})
	out1 := applyPermissionGate(ctx, d, "file-read", in)
	if out1.allow {
		t.Fatal("first call allow = true, want false (DenyAlways)")
	}
	if len(profiles.addedDeny) != 1 {
		t.Fatalf("first call: addedDeny len = %d, want 1", len(profiles.addedDeny))
	}

	// Second call: any file-read should now be silently blocked
	// at the profile-deny stage, before the tool is even consulted.
	target2 := filepath.Join(outside, "secret2.txt")
	writeFile(t, target2, "y\n")
	in2 := mustMarshal(t, file_read.ReadInput{AbsolutePath: target2})
	out2 := applyPermissionGate(ctx, d, "file-read", in2)

	if out2.allow {
		t.Fatal("second call allow = true, want false (session deny should block)")
	}
	if !strings.Contains(out2.errMsg, "blocked by permission profile") {
		t.Errorf("second call errMsg = %q, want 'blocked by permission profile'", out2.errMsg)
	}
	// HumanGate should NOT have been called a second time — the
	// session deny rule short-circuits before any prompt.
	if len(gate.permissionCalls) != 1 {
		t.Errorf("second call: HumanGate called %d times, want 1 (still just the first call)", len(gate.permissionCalls))
	}
}

// ── AllowAlways with narrow scope: only matching paths are silenced ───

func TestFileReadIntegration_AllowAlways_NarrowScope_BroaderPathStillPrompts(t *testing.T) {
	d, _ := realFileReadDispatcher(t)
	outside := t.TempDir()
	subA := filepath.Join(outside, "a")
	subB := filepath.Join(outside, "b")
	targetA := filepath.Join(subA, "file.txt")
	targetB := filepath.Join(subB, "file.txt")
	writeFile(t, targetA, "a\n")
	writeFile(t, targetB, "b\n")

	// First call: user picks Allow Always with the parent-dir
	// scope for subA only (not the broader outside dir). Uses
	// forward-slash form to match file-read's dimension format.
	profile := &permrules.Profile{Name: "session"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionAllowAlways,
		selectedRule: "file-read.absolute_path:" + filepath.ToSlash(subA) + "/**",
	}
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in1 := mustMarshal(t, file_read.ReadInput{AbsolutePath: targetA})
	out1 := applyPermissionGate(ctx, d, "file-read", in1)
	if !out1.allow {
		t.Fatalf("first call allow = false, want true. errMsg=%q", out1.errMsg)
	}

	// Second call: a path NOT under subA. The narrow rule should
	// not cover it, so the prompt should fire again.
	in2 := mustMarshal(t, file_read.ReadInput{AbsolutePath: targetB})
	gate.decision = hgapi.PermissionDecisionDenyOnce // user denies the second prompt
	out2 := applyPermissionGate(ctx, d, "file-read", in2)

	if out2.allow {
		t.Fatalf("second call allow = true, want false (different scope should re-prompt and user denied). errMsg=%q", out2.errMsg)
	}
	if len(gate.permissionCalls) != 2 {
		t.Errorf("HumanGate called %d times, want 2 (narrow scope should not cover the second call)", len(gate.permissionCalls))
	}
}

// ── helpers ─────────────────────────────────────────────────────────────

func mustMarshal(t *testing.T, in file_read.ReadInput) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(data)
}

// _ keeps the imports honest if any of the above gets temporarily
// disabled while iterating.
var _ = context.Background
