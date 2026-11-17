// SPDX-License-Identifier: Apache-2.0

package pathtool

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── Resolve ─────────────────────────────────────────────────────────────

type fakeResolver struct{ root string }

func (f *fakeResolver) Resolve(rel string) (string, error) {
	return filepath.Join(f.root, rel), nil
}
func (f *fakeResolver) WorkspaceRoot() string { return f.root }

func TestResolve_WorkspaceBranch(t *testing.T) {
	root, _ := filepath.Abs(t.TempDir())
	r := &fakeResolver{root: root}
	got, err := Resolve("sub/file.txt", "", r, "file-read")
	if err != nil {
		t.Fatal(err)
	}
	if got.AbsPath != filepath.Join(root, "sub/file.txt") {
		t.Errorf("AbsPath = %q", got.AbsPath)
	}
	if got.FromAbsoluteField {
		t.Error("FromAbsoluteField = true, want false")
	}
}

func TestResolve_AbsoluteBranch_TildeExpansion(t *testing.T) {
	got, err := Resolve("", "~", &fakeResolver{root: "/tmp"}, "file-read")
	if err != nil {
		t.Fatal(err)
	}
	if got.AbsPath == "" || got.AbsPath == "~" {
		t.Errorf("tilde not expanded: %q", got.AbsPath)
	}
	if !got.FromAbsoluteField {
		t.Error("FromAbsoluteField = false")
	}
}

// ── ClassifyAgainstWorkspace ────────────────────────────────────────────

func TestClassify_Inside(t *testing.T) {
	root, _ := filepath.Abs("/ws")
	if !ClassifyAgainstWorkspace(filepath.Join(root, "a.txt"), root) {
		t.Error("should be inside")
	}
}

func TestClassify_WsfooTrap(t *testing.T) {
	root, _ := filepath.Abs("/ws")
	wsfoo, _ := filepath.Abs("/wsfoo/bar")
	if ClassifyAgainstWorkspace(wsfoo, root) {
		t.Error("the /wsfoo trap: should NOT be inside /ws")
	}
}

func TestClassify_EmptyRoot_FailsClosed(t *testing.T) {
	abs, _ := filepath.Abs("/anything")
	if ClassifyAgainstWorkspace(abs, "") {
		t.Error("empty root should fail closed")
	}
}

// ── Binary blocklist ────────────────────────────────────────────────────

func TestIsBinaryExtension(t *testing.T) {
	for _, p := range []string{"/x/f.exe", "/x/f.dll", "/x/f.png", "/x/f.JPG", "/x/f.pdf"} {
		if !IsBinaryExtension(p) {
			t.Errorf("IsBinaryExtension(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/x/f.go", "/x/f.py", "/x/f.txt", "/x/f"} {
		if IsBinaryExtension(p) {
			t.Errorf("IsBinaryExtension(%q) = true, want false", p)
		}
	}
}

// ── Device blocklist ────────────────────────────────────────────────────

func TestIsBlockedDevicePath_POSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX device paths differ on Windows due to filepath.Clean")
	}
	for _, p := range []string{"/dev/zero", "/dev/null", "/dev/stdin"} {
		if !IsBlockedDevicePath(p) {
			t.Errorf("IsBlockedDevicePath(%q) = false", p)
		}
	}
}

func TestIsBlockedDevicePath_Windows(t *testing.T) {
	for _, p := range []string{"NUL", "nul", "CON", "nul.txt"} {
		if !IsBlockedDevicePath(p) {
			t.Errorf("IsBlockedDevicePath(%q) = false", p)
		}
	}
}

func TestIsBlockedDevicePath_Allows(t *testing.T) {
	for _, p := range []string{"/etc/hosts", "normal.go"} {
		if IsBlockedDevicePath(p) {
			t.Errorf("IsBlockedDevicePath(%q) = true", p)
		}
	}
}

// ── Suggestions ─────────────────────────────────────────────────────────

func TestBuildOutsideWorkspaceSuggestions(t *testing.T) {
	got := BuildOutsideWorkspaceSuggestions("file-read", "/home/user/secret.txt")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Rule != "file-read.absolute_path:/home/user/secret.txt" {
		t.Errorf("[0].Rule = %q", got[0].Rule)
	}
	if got[1].Rule != "file-read.absolute_path:/home/user/**" {
		t.Errorf("[1].Rule = %q", got[1].Rule)
	}
	if got[2].Rule != "file-read.absolute_path:**" {
		t.Errorf("[2].Rule = %q", got[2].Rule)
	}
}

func TestBuildOutsideWorkspaceSuggestions_ToolNamePrefix(t *testing.T) {
	// Verify the tool name parameter drives the rule prefix.
	got := BuildOutsideWorkspaceSuggestions("file-edit", "/etc/hosts")
	if !strings.HasPrefix(got[0].Rule, "file-edit.") {
		t.Errorf("wrong prefix: %q", got[0].Rule)
	}
}

// ── AttachPathHintIfNeeded ──────────────────────────────────────────────

func TestAttachPathHint_InsideWorkspace_AddsHint(t *testing.T) {
	root, _ := filepath.Abs(t.TempDir())
	r := &fakeResolver{root: root}
	res := ResolvedPath{AbsPath: filepath.Join(root, "a.txt"), FromAbsoluteField: true}
	got := AttachPathHintIfNeeded("", res, r)
	if !strings.Contains(got, "prefer 'workspace_path'") {
		t.Errorf("got %q, want hint", got)
	}
}

func TestAttachPathHint_OutsideWorkspace_NoHint(t *testing.T) {
	root, _ := filepath.Abs(t.TempDir())
	r := &fakeResolver{root: root}
	outside, _ := filepath.Abs(t.TempDir())
	res := ResolvedPath{AbsPath: filepath.Join(outside, "a.txt"), FromAbsoluteField: true}
	got := AttachPathHintIfNeeded("", res, r)
	if got != "" {
		t.Errorf("got %q, want empty (outside workspace)", got)
	}
}

func TestAttachPathHint_WorkspacePath_NoHint(t *testing.T) {
	root, _ := filepath.Abs(t.TempDir())
	r := &fakeResolver{root: root}
	res := ResolvedPath{AbsPath: filepath.Join(root, "a.txt"), FromAbsoluteField: false}
	got := AttachPathHintIfNeeded("", res, r)
	if got != "" {
		t.Errorf("got %q, want empty (workspace branch)", got)
	}
}
