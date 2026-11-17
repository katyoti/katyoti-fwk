// SPDX-License-Identifier: Apache-2.0

package glob

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

type fakeResolver struct{ root string }

func (f *fakeResolver) Resolve(rel string) (string, error) { return filepath.Join(f.root, rel), nil }
func (f *fakeResolver) WorkspaceRoot() string              { return f.root }

func tempRoot(t *testing.T) string {
	t.Helper()
	r, _ := filepath.Abs(t.TempDir())
	return r
}

func setupFiles(t *testing.T, root string, files []string) {
	t.Helper()
	for _, f := range files {
		full := filepath.Join(root, f)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte("x"), 0o644)
	}
}

// ── Happy path ──────────────────────────────────────────────────────────

func TestGlob_WorkspaceRoot_Default(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, []string{"a.go", "b.go", "sub/c.go"})
	resolver := &fakeResolver{root: root}

	out, err := executeGlob(context.Background(), GlobInput{Pattern: "**/*.go"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if out.NumFiles != 3 {
		t.Errorf("NumFiles = %d, want 3 (got: %v)", out.NumFiles, out.Filenames)
	}
}

func TestGlob_WorkspacePath_Subdir(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, []string{"src/a.go", "src/b.go", "other/c.go"})
	resolver := &fakeResolver{root: root}

	out, err := executeGlob(context.Background(), GlobInput{Pattern: "*.go", WorkspacePath: "src"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if out.NumFiles != 2 {
		t.Errorf("NumFiles = %d, want 2", out.NumFiles)
	}
}

func TestGlob_AbsolutePath(t *testing.T) {
	outside := tempRoot(t)
	setupFiles(t, outside, []string{"x.txt", "y.txt"})
	resolver := &fakeResolver{root: tempRoot(t)} // different root

	out, err := executeGlob(context.Background(), GlobInput{Pattern: "*.txt", AbsolutePath: outside}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if out.NumFiles != 2 {
		t.Errorf("NumFiles = %d, want 2", out.NumFiles)
	}
}

func TestGlob_NoMatches(t *testing.T) {
	root := tempRoot(t)
	resolver := &fakeResolver{root: root}
	out, err := executeGlob(context.Background(), GlobInput{Pattern: "**/*.xyz"}, resolver)
	if err != nil {
		t.Fatal(err)
	}
	if out.NumFiles != 0 {
		t.Errorf("NumFiles = %d, want 0", out.NumFiles)
	}
	if out.Message != "No files found" {
		t.Errorf("Message = %q", out.Message)
	}
}

func TestGlob_EmptyPattern_Error(t *testing.T) {
	root := tempRoot(t)
	resolver := &fakeResolver{root: root}
	_, err := executeGlob(context.Background(), GlobInput{Pattern: ""}, resolver)
	if err == nil {
		t.Fatal("expected error for empty pattern")
	}
}

func TestGlob_BothPathsSet_Error(t *testing.T) {
	root := tempRoot(t)
	resolver := &fakeResolver{root: root}
	_, err := executeGlob(context.Background(), GlobInput{
		Pattern: "*.go", WorkspacePath: "src", AbsolutePath: "/tmp",
	}, resolver)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("err = %v, want 'not both'", err)
	}
}

func TestGlob_PathNotExist_Error(t *testing.T) {
	root := tempRoot(t)
	resolver := &fakeResolver{root: root}
	_, err := executeGlob(context.Background(), GlobInput{
		Pattern: "*.go", WorkspacePath: "nonexistent",
	}, resolver)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("err = %v, want 'does not exist'", err)
	}
}

// ── Permission policy ──────────────────────────────────────────────────

func TestPermission_DefaultRoot_Allow(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(GlobInput{Pattern: "**/*.go"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("default root: Behavior = %v, want Allow", res.Behavior)
	}
}

func TestPermission_WorkspacePath_Allow(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(GlobInput{Pattern: "*.go", WorkspacePath: "src"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow", res.Behavior)
	}
}

func TestPermission_AbsoluteOutside_Ask(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t)
	in, _ := json.Marshal(GlobInput{Pattern: "*.go", AbsolutePath: outside})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAsk {
		t.Errorf("Behavior = %v, want Ask", res.Behavior)
	}
	if len(res.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d, want 3", len(res.Suggestions))
	}
	if !strings.HasPrefix(res.Suggestions[0].Rule, "glob.absolute_path:") {
		t.Errorf("wrong prefix: %q", res.Suggestions[0].Rule)
	}
}

func TestPermission_BothSet_Deny(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(GlobInput{Pattern: "*.go", WorkspacePath: "src", AbsolutePath: "/tmp"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionDeny {
		t.Errorf("Behavior = %v, want Deny", res.Behavior)
	}
}

// ── Describer ──────────────────────────────────────────────────────────

func TestDescribeGlob(t *testing.T) {
	raw, _ := json.Marshal(GlobInput{Pattern: "**/*.go", WorkspacePath: "src"})
	got := describeGlob(raw)
	if got != "glob pattern=**/*.go workspace_path=src" {
		t.Errorf("got %q", got)
	}
}
