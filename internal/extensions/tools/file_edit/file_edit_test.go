// SPDX-License-Identifier: Apache-2.0

package file_edit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/pathtool"
)

type fakeResolver struct{ root string }

func (f *fakeResolver) Resolve(rel string) (string, error) { return filepath.Join(f.root, rel), nil }
func (f *fakeResolver) WorkspaceRoot() string              { return f.root }

type testStoreKey struct{}

func withTestStore(ctx context.Context, store filestate.Store) context.Context {
	return context.WithValue(ctx, testStoreKey{}, store)
}

func init() {
	filestateFrom = func(ctx context.Context) filestate.Store {
		if s, ok := ctx.Value(testStoreKey{}).(filestate.Store); ok {
			return s
		}
		return nil
	}
}

func tempRoot(t *testing.T) string {
	t.Helper()
	r, _ := filepath.Abs(t.TempDir())
	return r
}

func newRunner(t *testing.T, files map[string]string) (*fakeResolver, string, func(EditInput) (EditOutput, error)) {
	t.Helper()
	root := tempRoot(t)
	for name, content := range files {
		full := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}
	resolver := &fakeResolver{root: root}
	store := filestate.NewMemoryStore(0)
	ctx := withTestStore(context.Background(), store)
	run := func(in EditInput) (EditOutput, error) {
		return runEdit(ctx, in, resolver)
	}
	return resolver, root, run
}

func runnerWithStore(t *testing.T, files map[string]string, store filestate.Store) (*fakeResolver, string, func(EditInput) (EditOutput, error)) {
	t.Helper()
	root := tempRoot(t)
	for name, content := range files {
		full := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}
	resolver := &fakeResolver{root: root}
	ctx := withTestStore(context.Background(), store)
	run := func(in EditInput) (EditOutput, error) {
		return runEdit(ctx, in, resolver)
	}
	return resolver, root, run
}

func prepareReadEntry(t *testing.T, store filestate.Store, path string) {
	t.Helper()
	info, _ := os.Stat(path)
	data, _ := os.ReadFile(path)
	zero := 0
	lim := 2000
	store.Record(path, filestate.Entry{
		Content: string(data), Timestamp: info.ModTime(), Offset: &zero, Limit: &lim,
	})
}

// ── Single replacement ──────────────────────────────────────────────────

func TestRunEdit_SingleReplacement(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.go": "hello world\n"}, store)
	prepareReadEntry(t, store, filepath.Join(root, "a.go"))

	out, err := run(EditInput{WorkspacePath: "a.go", OldString: "hello", NewString: "goodbye"})
	if err != nil {
		t.Fatalf("runEdit: %v", err)
	}
	if out.Replacements != 1 {
		t.Errorf("Replacements = %d, want 1", out.Replacements)
	}
	got, _ := os.ReadFile(filepath.Join(root, "a.go"))
	if string(got) != "goodbye world\n" {
		t.Errorf("file content = %q", string(got))
	}
}

// ── Replace all ─────────────────────────────────────────────────────────

func TestRunEdit_ReplaceAll(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.go": "foo bar foo baz foo\n"}, store)
	prepareReadEntry(t, store, filepath.Join(root, "a.go"))

	out, err := run(EditInput{WorkspacePath: "a.go", OldString: "foo", NewString: "qux", ReplaceAll: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Replacements != 3 {
		t.Errorf("Replacements = %d, want 3", out.Replacements)
	}
	got, _ := os.ReadFile(filepath.Join(root, "a.go"))
	if string(got) != "qux bar qux baz qux\n" {
		t.Errorf("content = %q", string(got))
	}
}

// ── Multiple matches without replace_all → error ────────────────────────

func TestRunEdit_MultipleMatches_RejectsWithoutReplaceAll(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.go": "foo foo foo\n"}, store)
	prepareReadEntry(t, store, filepath.Join(root, "a.go"))

	_, err := run(EditInput{WorkspacePath: "a.go", OldString: "foo", NewString: "bar"})
	if err == nil {
		t.Fatal("expected error for multiple matches")
	}
	if !strings.Contains(err.Error(), "3 times") {
		t.Errorf("err = %v, want mention of count", err)
	}
}

// ── old_string not found ───────────────────────────────────────────────

func TestRunEdit_OldStringNotFound(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.go": "hello\n"}, store)
	prepareReadEntry(t, store, filepath.Join(root, "a.go"))

	_, err := run(EditInput{WorkspacePath: "a.go", OldString: "goodbye", NewString: "x"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

// ── old_string == new_string → error ───────────────────────────────────

func TestRunEdit_NoChange(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"a.go": "x\n"})
	_, err := run(EditInput{WorkspacePath: "a.go", OldString: "x", NewString: "x"})
	if err == nil || !strings.Contains(err.Error(), "identical") {
		t.Errorf("err = %v, want 'identical'", err)
	}
}

// ── Read-before-write enforcement ──────────────────────────────────────

func TestRunEdit_RejectWithoutPriorRead(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"a.go": "x\n"})
	_, err := run(EditInput{WorkspacePath: "a.go", OldString: "x", NewString: "y"})
	if err == nil || !strings.Contains(err.Error(), "must be read first") {
		t.Errorf("err = %v, want 'must be read first'", err)
	}
}

// ── New file via empty old_string ──────────────────────────────────────

func TestRunEdit_CreateNewFile_EmptyOldString(t *testing.T) {
	_, root, run := newRunner(t, nil)
	out, err := run(EditInput{WorkspacePath: "new/file.txt", OldString: "", NewString: "created content"})
	if err != nil {
		t.Fatalf("runEdit: %v", err)
	}
	if !out.Created {
		t.Error("Created = false, want true")
	}
	got, _ := os.ReadFile(filepath.Join(root, "new/file.txt"))
	if string(got) != "created content" {
		t.Errorf("content = %q", string(got))
	}
}

func TestRunEdit_EmptyOldString_ExistingNonEmpty_Rejects(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.go": "content\n"}, store)
	prepareReadEntry(t, store, filepath.Join(root, "a.go"))

	_, err := run(EditInput{WorkspacePath: "a.go", OldString: "", NewString: "new"})
	if err == nil || !strings.Contains(err.Error(), "empty old_string") {
		t.Errorf("err = %v, want rejection for empty old_string on non-empty file", err)
	}
}

// ── .ipynb rejection ───────────────────────────────────────────────────

func TestRunEdit_IpynbRejected(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"nb.ipynb": "{}"})
	_, err := run(EditInput{WorkspacePath: "nb.ipynb", OldString: "x", NewString: "y"})
	if err == nil || !strings.Contains(err.Error(), "notebook edit") {
		t.Errorf("err = %v, want ipynb rejection", err)
	}
}

// ── Permission policy ────────────────────────────────────────────────

func TestPermission_WorkspacePath_Allow(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(EditInput{WorkspacePath: "a.go", OldString: "x", NewString: "y"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow", res.Behavior)
	}
}

func TestPermission_AbsolutePath_Outside_Ask(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t)
	target := filepath.Join(outside, "x.go")
	in, _ := json.Marshal(EditInput{AbsolutePath: target, OldString: "x", NewString: "y"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAsk {
		t.Errorf("Behavior = %v, want Ask", res.Behavior)
	}
	if len(res.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d, want 3", len(res.Suggestions))
	}
	if !strings.HasPrefix(res.Suggestions[0].Rule, "file-edit.absolute_path:") {
		t.Errorf("suggestion uses wrong tool prefix: %q", res.Suggestions[0].Rule)
	}
}

// ── pathtool.BuildOutsideWorkspaceSuggestions uses tool name param ─────

func TestSuggestions_UseFileEditPrefix(t *testing.T) {
	sug := pathtool.BuildOutsideWorkspaceSuggestions("file-edit", "/home/user/secret.txt")
	if !strings.HasPrefix(sug[0].Rule, "file-edit.") {
		t.Errorf("rule prefix wrong: %q", sug[0].Rule)
	}
}

// ── Describer ───────────────────────────────────────────────────────────

func TestDescribeEdit(t *testing.T) {
	raw, _ := json.Marshal(EditInput{WorkspacePath: "src/main.go", OldString: "x", NewString: "y"})
	got := describeEdit(raw)
	if got != "file-edit workspace_path=src/main.go" {
		t.Errorf("got %q", got)
	}
}

func TestDescribeEdit_ReplaceAll(t *testing.T) {
	raw, _ := json.Marshal(EditInput{WorkspacePath: "x.go", OldString: "a", NewString: "b", ReplaceAll: true})
	got := describeEdit(raw)
	if !strings.Contains(got, "replace_all=true") {
		t.Errorf("got %q, want replace_all", got)
	}
}
