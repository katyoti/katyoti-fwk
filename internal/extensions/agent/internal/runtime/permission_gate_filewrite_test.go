// SPDX-License-Identifier: Apache-2.0

package runtime

// End-to-end integration tests for the permission gate using the
// real file-write tool. Mirrors permission_gate_filread_test.go but
// for the write path. Together they prove that the rules system
// works end-to-end for both file tools that ship in ms04-p1.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/file_write"
)

// fakeFileWriteResolver implements file_write.PathResolver against
// a caller-provided workspace root. Mirrors the file-read fixture.
type fakeFileWriteResolver struct {
	root string
}

func (r *fakeFileWriteResolver) Resolve(relPath string) (string, error) {
	return filepath.Join(r.root, relPath), nil
}

func (r *fakeFileWriteResolver) WorkspaceRoot() string { return r.root }

// realFileWriteDispatcher builds a Dispatcher with a real
// file-write tool wired against a temp workspace, and returns the
// workspace root for the test to set up fixtures and inspect the
// post-state.
func realFileWriteDispatcher(t *testing.T) (*Dispatcher, string) {
	t.Helper()
	root := t.TempDir()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("abs tempdir: %v", err)
	}

	resolver := &fakeFileWriteResolver{root: abs}
	tool := file_write.NewFileWriteTool(resolver)

	d := NewDispatcher()
	d.RegisterPlugin(toolsapi.NewToolPlugin(tool))
	return d, abs
}

func writeFileFW(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMarshalWrite(t *testing.T, in file_write.WriteInput) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(data)
}

// ── Workspace_path create-new: silent allow ────────────────────────────

func TestFileWriteIntegration_WorkspacePath_CreateNew_SilentAllow(t *testing.T) {
	d, root := realFileWriteDispatcher(t)

	// file-write needs a filestate Store registered for existing-file
	// writes. For new files (no prior version) the store can be nil.
	// Real production runs always have a store; the integration test
	// mirrors that.
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce} // would block if called
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshalWrite(t, file_write.WriteInput{
		WorkspacePath: "src/main.go",
		Content:       "package main\n",
	})
	out := applyPermissionGate(ctx, d, "file-write", in)

	if !out.allow {
		t.Fatalf("allow = false, want true. errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate called %d times, want 0 (workspace_path should not prompt)", len(gate.permissionCalls))
	}
	// file-write needs filestate registered to actually call the
	// tool body — but we're testing the GATE here, not the handler
	// dispatch. Confirm gate said allow and stop. The gate→handler
	// integration is exercised separately by the unit tests.
	_ = root
}

// ── Absolute_path inside workspace: silent allow ──────────────────────

func TestFileWriteIntegration_AbsolutePath_InsideWorkspace_SilentAllow(t *testing.T) {
	d, root := realFileWriteDispatcher(t)

	target := filepath.Join(root, "internal", "thing.txt")
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce}
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshalWrite(t, file_write.WriteInput{
		AbsolutePath: target,
		Content:      "x",
	})
	out := applyPermissionGate(ctx, d, "file-write", in)

	if !out.allow {
		t.Fatalf("allow = false, want true. errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate called %d times, want 0", len(gate.permissionCalls))
	}
}

// ── Absolute_path outside workspace: prompts with file-write suggestions ─

func TestFileWriteIntegration_AbsolutePath_Outside_Prompts(t *testing.T) {
	d, _ := realFileWriteDispatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "external.txt")

	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionAllowOnce}
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshalWrite(t, file_write.WriteInput{
		AbsolutePath: target,
		Content:      "external content",
	})
	out := applyPermissionGate(ctx, d, "file-write", in)

	if !out.allow {
		t.Fatalf("allow = false, want true after AllowOnce. errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Fatalf("HumanGate called %d times, want 1", len(gate.permissionCalls))
	}

	req := gate.permissionCalls[0]
	if req.ToolName != "file-write" {
		t.Errorf("ToolName = %q, want file-write", req.ToolName)
	}
	if !strings.Contains(req.Question, "outside the workspace") {
		t.Errorf("Question = %q, want substring 'outside the workspace'", req.Question)
	}
	if len(req.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d, want 3", len(req.Suggestions))
	}
	// Critical: rules use file-write prefix (not file-read).
	if !strings.HasPrefix(req.Suggestions[0].Rule, "file-write.absolute_path:") {
		t.Errorf("first suggestion rule = %q, want file-write.absolute_path: prefix", req.Suggestions[0].Rule)
	}
	if req.Suggestions[2].Rule != "file-write.absolute_path:**" {
		t.Errorf("broad suggestion = %q, want file-write.absolute_path:**", req.Suggestions[2].Rule)
	}
}

// ── Profile pre-approval ──────────────────────────────────────────────

func TestFileWriteIntegration_AbsolutePath_Outside_ProfileAllows(t *testing.T) {
	d, _ := realFileWriteDispatcher(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "spec.md")

	outsideSlash := filepath.ToSlash(outside)
	profile := &permrules.Profile{
		Name: "test",
		AllowRules: []permrules.Rule{
			permrules.MustParseRule("file-write.absolute_path:" + outsideSlash + "/**"),
		},
	}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce} // would block if called
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in := mustMarshalWrite(t, file_write.WriteInput{
		AbsolutePath: target,
		Content:      "x",
	})
	out := applyPermissionGate(ctx, d, "file-write", in)

	if !out.allow {
		t.Fatalf("allow = false, want true (profile should silence). errMsg=%q", out.errMsg)
	}
	if len(gate.permissionCalls) != 0 {
		t.Errorf("HumanGate called %d times, want 0 (profile pre-approval)", len(gate.permissionCalls))
	}
}

// ── Plan-mode-style profile blocks all writes ─────────────────────────

func TestFileWriteIntegration_PlanMode_BlocksWorkspaceWrites(t *testing.T) {
	// The shipped plan-mode profile denies file-write:**. Even
	// workspace_path writes should be blocked because profile deny
	// fires BEFORE the tool's CheckPermissions.
	d, _ := realFileWriteDispatcher(t)

	profile := &permrules.Profile{
		Name:      "plan-mode",
		DenyRules: []permrules.Rule{permrules.MustParseRule("file-write:**")},
	}
	profiles := &fakeProfiles{profile: profile}
	ctx := newAppCtxWithGateAndProfiles(t, nil, profiles)

	in := mustMarshalWrite(t, file_write.WriteInput{
		WorkspacePath: "src/main.go",
		Content:       "package main\n",
	})
	out := applyPermissionGate(ctx, d, "file-write", in)

	if out.allow {
		t.Fatal("allow = true, want false (plan-mode should block file-write)")
	}
	if !strings.Contains(out.errMsg, "blocked by permission profile") {
		t.Errorf("errMsg = %q, want 'blocked by permission profile'", out.errMsg)
	}
	if !strings.Contains(out.errMsg, "plan-mode") {
		t.Errorf("errMsg should name the profile, got %q", out.errMsg)
	}
}

// ── AllowAlways: second write to same dir is silent ───────────────────

func TestFileWriteIntegration_AllowAlways_SecondCallSilent(t *testing.T) {
	d, _ := realFileWriteDispatcher(t)
	outside := t.TempDir()
	target1 := filepath.Join(outside, "config1.yaml")
	target2 := filepath.Join(outside, "config2.yaml")

	profile := &permrules.Profile{Name: "session"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionAllowAlways,
		selectedRule: "file-write.absolute_path:**",
	}
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in1 := mustMarshalWrite(t, file_write.WriteInput{AbsolutePath: target1, Content: "k: v\n"})
	out1 := applyPermissionGate(ctx, d, "file-write", in1)
	if !out1.allow {
		t.Fatalf("first call: allow = false, want true. errMsg=%q", out1.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Fatalf("first call: HumanGate called %d times, want 1", len(gate.permissionCalls))
	}
	if len(profiles.addedAllow) != 1 {
		t.Fatalf("first call: addedAllow len = %d, want 1", len(profiles.addedAllow))
	}

	// Second call: different file, same broad scope. Should be silent.
	in2 := mustMarshalWrite(t, file_write.WriteInput{AbsolutePath: target2, Content: "k: v\n"})
	out2 := applyPermissionGate(ctx, d, "file-write", in2)
	if !out2.allow {
		t.Fatalf("second call: allow = false, want true (session rule should silence). errMsg=%q", out2.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Errorf("second call: HumanGate called %d times, want 1 (still just the first call)", len(gate.permissionCalls))
	}
}

// ── DenyAlways: second write silently blocked ─────────────────────────

func TestFileWriteIntegration_DenyAlways_SecondCallSilentlyBlocked(t *testing.T) {
	d, _ := realFileWriteDispatcher(t)
	outside := t.TempDir()
	target1 := filepath.Join(outside, "secret1.txt")
	target2 := filepath.Join(outside, "secret2.txt")

	profile := &permrules.Profile{Name: "session"}
	profiles := &fakeProfiles{profile: profile}
	gate := &fakeHumanGate{
		decision:     hgapi.PermissionDecisionDenyAlways,
		selectedRule: "file-write:**",
	}
	ctx := newAppCtxWithGateAndProfiles(t, gate, profiles)

	in1 := mustMarshalWrite(t, file_write.WriteInput{AbsolutePath: target1, Content: "x"})
	out1 := applyPermissionGate(ctx, d, "file-write", in1)
	if out1.allow {
		t.Fatal("first call: allow = true, want false (DenyAlways)")
	}
	if len(profiles.addedDeny) != 1 {
		t.Fatalf("first call: addedDeny len = %d, want 1", len(profiles.addedDeny))
	}

	// Second call: any file-write should now be silently blocked.
	in2 := mustMarshalWrite(t, file_write.WriteInput{AbsolutePath: target2, Content: "y"})
	out2 := applyPermissionGate(ctx, d, "file-write", in2)
	if out2.allow {
		t.Fatal("second call: allow = true, want false (session deny should block)")
	}
	if !strings.Contains(out2.errMsg, "blocked by permission profile") {
		t.Errorf("second call errMsg = %q", out2.errMsg)
	}
	if len(gate.permissionCalls) != 1 {
		t.Errorf("HumanGate called %d times, want 1", len(gate.permissionCalls))
	}
}

// ── Tool-level read-before-write enforcement (sanity check) ──────────

// This test verifies that file-write's own CheckPermissions does
// NOT enforce read-before-write — that's the handler's job, not
// the gate's. The gate path returns Allow for a workspace_path
// even when the file already exists; the actual rejection happens
// when the dispatcher invokes the handler.
//
// We can't easily test the dispatcher invocation here without a
// real filestate service registered in the application context;
// the unit tests in file-write_test.go cover that path directly.
// This test just confirms the gate is permissive and the rejection
// must come from elsewhere.
func TestFileWriteIntegration_GateAllowsExistingFile_HandlerWillReject(t *testing.T) {
	d, root := realFileWriteDispatcher(t)
	// Pre-create a file so it "exists" for the would-be handler.
	writeFileFW(t, filepath.Join(root, "existing.txt"), "old content")

	gate := &fakeHumanGate{decision: hgapi.PermissionDecisionDenyOnce}
	ctx := newAppCtxWithGate(t, gate)

	in := mustMarshalWrite(t, file_write.WriteInput{
		WorkspacePath: "existing.txt",
		Content:       "new content",
	})
	out := applyPermissionGate(ctx, d, "file-write", in)

	// The GATE allows it (workspace_path) — the handler is what
	// will reject it for missing read-before-write. Read-before-
	// write is a runtime invariant, not a permission decision.
	if !out.allow {
		t.Errorf("allow = false, want true (the gate is permissive; the handler enforces read-before-write at runtime)")
	}

	// Use _ = filestate to keep the import honest if the test
	// shape changes.
	_ = filestate.NewMemoryStore
}
