// SPDX-License-Identifier: Apache-2.0

package file_read

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

// fakeResolver is a stub PathResolver for testing the workspace branch
// without spinning up a real workspace.
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

// ── Validate ────────────────────────────────────────────────────────────

func TestValidate_PathFieldExclusivity(t *testing.T) {
	cases := []struct {
		name string
		in   ReadInput
		want error
	}{
		{"both empty", ReadInput{}, ErrPathFieldsNeither},
		{"both set", ReadInput{WorkspacePath: "a", AbsolutePath: "/b"}, ErrPathFieldsBoth},
		{"workspace only", ReadInput{WorkspacePath: "a"}, nil},
		{"absolute only", ReadInput{AbsolutePath: "/a"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Validate(c.in)
			if !errors.Is(got, c.want) {
				t.Errorf("Validate = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidate_OffsetNegative(t *testing.T) {
	bad := -1
	in := ReadInput{WorkspacePath: "a", Offset: &bad}
	if got := Validate(in); !errors.Is(got, ErrOffsetNegative) {
		t.Errorf("Validate = %v, want ErrOffsetNegative", got)
	}
}

func TestValidate_OffsetZero_OK(t *testing.T) {
	zero := 0
	in := ReadInput{WorkspacePath: "a", Offset: &zero}
	if err := Validate(in); err != nil {
		t.Errorf("Validate(offset=0) = %v, want nil", err)
	}
}

func TestValidate_LimitNonPositive(t *testing.T) {
	cases := []int{0, -1, -100}
	for _, n := range cases {
		n := n
		in := ReadInput{WorkspacePath: "a", Limit: &n}
		if got := Validate(in); !errors.Is(got, ErrLimitNonPositive) {
			t.Errorf("Validate(limit=%d) = %v, want ErrLimitNonPositive", n, got)
		}
	}
}

func TestValidate_LimitPositive_OK(t *testing.T) {
	one := 1
	in := ReadInput{WorkspacePath: "a", Limit: &one}
	if err := Validate(in); err != nil {
		t.Errorf("Validate(limit=1) = %v, want nil", err)
	}
}

// ── Resolve ─────────────────────────────────────────────────────────────

func TestResolve_WorkspaceBranch(t *testing.T) {
	root := tempRoot(t)
	r := &fakeResolver{root: root}

	got, err := Resolve(ReadInput{WorkspacePath: "sub/file.txt"}, r)
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

func TestResolve_WorkspaceBranch_PropagatesResolverError(t *testing.T) {
	r := &fakeResolver{resolvErr: errors.New("boom")}
	_, err := Resolve(ReadInput{WorkspacePath: "x"}, r)
	if err == nil {
		t.Fatal("Resolve = nil error, want resolver error wrapped")
	}
	if !contains(err.Error(), "resolve workspace_path") {
		t.Errorf("error = %q, want substring 'resolve workspace_path'", err.Error())
	}
}

func TestResolve_AbsoluteBranch(t *testing.T) {
	root := tempRoot(t)
	target := filepath.Join(root, "abs.txt")

	got, err := Resolve(ReadInput{AbsolutePath: target}, &fakeResolver{root: root})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.AbsPath != filepath.Clean(target) {
		t.Errorf("AbsPath = %q, want %q", got.AbsPath, target)
	}
	if !got.FromAbsoluteField {
		t.Error("FromAbsoluteField = false, want true (absolute branch)")
	}
}

func TestResolve_AbsoluteBranch_TildeExpansion(t *testing.T) {
	got, err := Resolve(ReadInput{AbsolutePath: "~"}, &fakeResolver{root: "/tmp"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.AbsPath == "" || got.AbsPath == "~" {
		t.Errorf("tilde not expanded: AbsPath = %q", got.AbsPath)
	}
}

// ── ClassifyAgainstWorkspace ────────────────────────────────────────────

func TestClassifyAgainstWorkspace_Inside(t *testing.T) {
	root := absForOS("/ws")
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

func TestClassifyAgainstWorkspace_Outside(t *testing.T) {
	root := absForOS("/ws")
	cases := []string{
		absForOS("/etc/passwd"),
		absForOS("/wsfoo/bar"), // sneaky: same prefix string but different dir
		absForOS("/ws-other/x"),
		absForOS("/"),
	}
	for _, p := range cases {
		if ClassifyAgainstWorkspace(p, root) {
			t.Errorf("Classify(%q, %q) = true, want false", p, root)
		}
	}
}

func TestClassifyAgainstWorkspace_EmptyRoot_FailsClosed(t *testing.T) {
	if ClassifyAgainstWorkspace(absForOS("/anything"), "") {
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
		t.Skip("POSIX device paths are blocked but test fixture cleaned by filepath.Clean differently on Windows")
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
	// These reserved names are rejected on every OS — the file_read
	// tool refuses them defensively even on POSIX so behavior is
	// uniform across the binaries.
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

// ── helpers ─────────────────────────────────────────────────────────────

func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(t.TempDir())
	if err != nil {
		t.Fatalf("abs tempdir: %v", err)
	}
	return root
}

// absForOS converts a POSIX-style test path to an absolute path that
// matches the running OS's conventions. On Windows, "/ws" becomes
// "C:\ws" (or similar) so that filepath.Clean treats it as absolute.
func absForOS(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	abs, _ := filepath.Abs(p)
	return abs
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
