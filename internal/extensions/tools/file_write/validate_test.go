// SPDX-License-Identifier: Apache-2.0

package file_write

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeResolver is a stub PathResolver for testing the workspace
// branch without spinning up a real workspace.
type fakeResolver struct {
	root      string
	resolvErr error
}

func (f *fakeResolver) Resolve(relPath string) (string, error) {
	if f.resolvErr != nil {
		return "", f.resolvErr
	}
	return filepath.Join(f.root, relPath), nil
}

func (f *fakeResolver) WorkspaceRoot() string { return f.root }

func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("abs tempdir: %v", err)
	}
	return root
}

// ── Validate ────────────────────────────────────────────────────────────

func TestValidate_PathFieldExclusivity(t *testing.T) {
	cases := []struct {
		name string
		in   WriteInput
		want error
	}{
		{"both empty", WriteInput{Content: "x"}, ErrPathFieldsNeither},
		{"both set", WriteInput{WorkspacePath: "a", AbsolutePath: "/b", Content: "x"}, ErrPathFieldsBoth},
		{"workspace only", WriteInput{WorkspacePath: "a", Content: "x"}, nil},
		{"absolute only", WriteInput{AbsolutePath: "/a", Content: "x"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Validate(c.in, 1<<20)
			if !errors.Is(got, c.want) {
				t.Errorf("Validate = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidate_EmptyContent_Allowed(t *testing.T) {
	// Empty content is a legitimate use case (touch a `.gitkeep`).
	in := WriteInput{WorkspacePath: "a", Content: ""}
	if err := Validate(in, 1<<20); err != nil {
		t.Errorf("Validate(empty content) = %v, want nil", err)
	}
}

func TestValidate_ContentTooLarge(t *testing.T) {
	in := WriteInput{WorkspacePath: "a", Content: "abcdef"}
	err := Validate(in, 5)
	if !errors.Is(err, ErrContentTooLarge) {
		t.Errorf("Validate = %v, want ErrContentTooLarge", err)
	}
}

func TestValidate_ZeroCap_Disabled(t *testing.T) {
	// maxBytes=0 disables the cap (used by the permission gate).
	huge := make([]byte, 1<<22) // 4 MiB
	in := WriteInput{WorkspacePath: "a", Content: string(huge)}
	if err := Validate(in, 0); err != nil {
		t.Errorf("Validate with cap=0 = %v, want nil (cap disabled)", err)
	}
}

// ── Resolve ─────────────────────────────────────────────────────────────

func TestResolve_WorkspaceBranch(t *testing.T) {
	root := tempRoot(t)
	r := &fakeResolver{root: root}

	got, err := Resolve(WriteInput{WorkspacePath: "sub/file.txt", Content: "x"}, r)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(root, "sub/file.txt")
	if got.AbsPath != want {
		t.Errorf("AbsPath = %q, want %q", got.AbsPath, want)
	}
	if got.FromAbsoluteField {
		t.Error("FromAbsoluteField = true, want false (workspace branch)")
	}
}

func TestResolve_AbsoluteBranch(t *testing.T) {
	root := tempRoot(t)
	target := filepath.Join(root, "abs.txt")

	got, err := Resolve(WriteInput{AbsolutePath: target, Content: "x"}, &fakeResolver{root: root})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.AbsPath != filepath.Clean(target) {
		t.Errorf("AbsPath = %q, want %q", got.AbsPath, target)
	}
	if !got.FromAbsoluteField {
		t.Error("FromAbsoluteField = false, want true")
	}
}

// ── ClassifyAgainstWorkspace ────────────────────────────────────────────

func TestClassifyAgainstWorkspace_Inside(t *testing.T) {
	root, _ := filepath.Abs("/ws")
	cases := []string{
		root,
		filepath.Join(root, "a.txt"),
		filepath.Join(root, "sub", "b.txt"),
	}
	for _, p := range cases {
		if !ClassifyAgainstWorkspace(p, root) {
			t.Errorf("Classify(%q, %q) = false, want true", p, root)
		}
	}
}

func TestClassifyAgainstWorkspace_Outside_WsfooTrap(t *testing.T) {
	// /wsfoo must NOT match /ws — string-prefix is wrong, must be
	// separator-aware.
	root, _ := filepath.Abs("/ws")
	wsfoo, _ := filepath.Abs("/wsfoo/bar")
	if ClassifyAgainstWorkspace(wsfoo, root) {
		t.Errorf("Classify(%q, %q) = true (the /wsfoo trap), want false", wsfoo, root)
	}
}

func TestClassifyAgainstWorkspace_EmptyRoot_FailsClosed(t *testing.T) {
	abs, _ := filepath.Abs("/anything")
	if ClassifyAgainstWorkspace(abs, "") {
		t.Error("empty root should classify everything as outside (fail-closed)")
	}
}

// ── Binary blocklist ────────────────────────────────────────────────────

func TestIsBinaryExtension(t *testing.T) {
	rejected := []string{
		"/x/foo.exe", "/x/foo.dll", "/x/foo.png", "/x/foo.JPG",
		"/x/foo.pdf", "/x/foo.ipynb", "/x/foo.zip", "/x/foo.mp4",
	}
	for _, p := range rejected {
		if !IsBinaryExtension(p) {
			t.Errorf("IsBinaryExtension(%q) = false, want true", p)
		}
	}
	accepted := []string{
		"/x/foo.go", "/x/foo.py", "/x/foo.txt", "/x/foo.md",
		"/x/foo", "/x/foo.yaml", "/x/foo.json",
	}
	for _, p := range accepted {
		if IsBinaryExtension(p) {
			t.Errorf("IsBinaryExtension(%q) = true, want false", p)
		}
	}
}

// ── Device blocklist ────────────────────────────────────────────────────

func TestIsBlockedDevicePath_POSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX device paths are blocked but filepath.Clean differs on Windows")
	}
	rejected := []string{
		"/dev/zero", "/dev/random", "/dev/urandom",
		"/dev/null", "/dev/stdin", "/dev/stdout", "/dev/stderr", "/dev/tty",
	}
	for _, p := range rejected {
		if !IsBlockedDevicePath(p) {
			t.Errorf("IsBlockedDevicePath(%q) = false, want true", p)
		}
	}
}

func TestIsBlockedDevicePath_Windows(t *testing.T) {
	rejected := []string{
		"NUL", "nul", "CON", "con",
		"PRN", "AUX", "COM1", "LPT1",
		"nul.txt", // base name is reserved → reject
	}
	for _, p := range rejected {
		if !IsBlockedDevicePath(p) {
			t.Errorf("IsBlockedDevicePath(%q) = false, want true", p)
		}
	}
}

func TestIsBlockedDevicePath_Allows(t *testing.T) {
	allowed := []string{
		"/etc/hosts", "/home/user/file.txt", "/tmp/something",
		"normal-file.go", "deep/path/file.json",
	}
	for _, p := range allowed {
		if IsBlockedDevicePath(p) {
			t.Errorf("IsBlockedDevicePath(%q) = true, want false", p)
		}
	}
}
