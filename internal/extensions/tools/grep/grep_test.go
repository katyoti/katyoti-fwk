// SPDX-License-Identifier: Apache-2.0

package grep

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

func setupFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		os.WriteFile(full, []byte(content), 0o644)
	}
}

// ── files_with_matches mode (default) ───────────────────────────────────

func TestGrep_FilesWithMatches_Default(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go":  "package main\nfunc hello() {}\n",
		"b.go":  "package lib\nfunc world() {}\n",
		"c.txt": "no go here\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{Pattern: "func"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 2 {
		t.Errorf("FileCount = %d, want 2", out.FileCount)
	}
	if !strings.Contains(out.Content, "a.go") || !strings.Contains(out.Content, "b.go") {
		t.Errorf("Content = %q, want both a.go and b.go", out.Content)
	}
	if strings.Contains(out.Content, "c.txt") {
		t.Errorf("Content should not include c.txt")
	}
}

// ── content mode ───────────────────────────────────────────────────────

func TestGrep_ContentMode(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go": "line1\nfunc hello() {}\nline3\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{
		Pattern:    "func",
		OutputMode: "content",
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "2:func hello()") {
		t.Errorf("Content = %q, want line number + match", out.Content)
	}
}

func TestGrep_ContentMode_WithContext(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go": "line1\nline2\nfunc target()\nline4\nline5\n",
	})
	r := &fakeResolver{root: root}
	ctx := 1
	out, err := executeGrep(context.Background(), GrepInput{
		Pattern:    "target",
		OutputMode: "content",
		Context:    &ctx,
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	// Should include line2 (before) and line4 (after) as context.
	if !strings.Contains(out.Content, "2:line2") {
		t.Errorf("Content missing before-context: %q", out.Content)
	}
	if !strings.Contains(out.Content, "4:line4") {
		t.Errorf("Content missing after-context: %q", out.Content)
	}
}

// ── count mode ─────────────────────────────────────────────────────────

func TestGrep_CountMode(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go": "foo bar foo baz foo\nfoo\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{
		Pattern:    "foo",
		OutputMode: "count",
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Content, "a.go:4") {
		t.Errorf("Content = %q, want a.go:4 (4 matches)", out.Content)
	}
}

// ── case insensitive ───────────────────────────────────────────────────

func TestGrep_CaseInsensitive(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go": "Hello World\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{
		Pattern:    "hello",
		CaseInsens: true,
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1 (case-insensitive)", out.FileCount)
	}
}

// ── type filter ─────────────────────────────────────────────────────────

func TestGrep_TypeFilter(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go":  "func hello()\n",
		"b.py":  "def hello():\n",
		"c.txt": "hello\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{Pattern: "hello", Type: "go"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 1 {
		t.Errorf("FileCount = %d, want 1 (only .go files)", out.FileCount)
	}
	if !strings.Contains(out.Content, "a.go") {
		t.Errorf("Content = %q, want a.go", out.Content)
	}
}

// ── glob filter ─────────────────────────────────────────────────────────

func TestGrep_GlobFilter(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"src/a.ts":  "const x = 1;\n",
		"src/b.tsx": "const y = 2;\n",
		"lib/c.ts":  "const z = 3;\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{Pattern: "const", Glob: "src/**"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 2 {
		t.Errorf("FileCount = %d, want 2 (only src/)", out.FileCount)
	}
}

// ── no matches ─────────────────────────────────────────────────────────

func TestGrep_NoMatches(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{"a.go": "x\n"})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{Pattern: "zzz_nonexistent"}, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.FileCount != 0 {
		t.Errorf("FileCount = %d, want 0", out.FileCount)
	}
}

// ── empty pattern → error ─────────────────────────────────────────────

func TestGrep_EmptyPattern_Error(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	_, err := executeGrep(context.Background(), GrepInput{Pattern: ""}, r)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── pagination ─────────────────────────────────────────────────────────

func TestGrep_Pagination(t *testing.T) {
	root := tempRoot(t)
	files := map[string]string{}
	for i := 0; i < 10; i++ {
		files[filepath.Join("src", strings.Repeat("a", i+1)+".go")] = "func x()\n"
	}
	setupFiles(t, root, files)
	r := &fakeResolver{root: root}
	limit := 3
	out, err := executeGrep(context.Background(), GrepInput{
		Pattern:   "func",
		HeadLimit: &limit,
		Offset:    2,
	}, r)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.Content), "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3 (offset=2, limit=3 from 10)", len(lines))
	}
	if !out.Truncated {
		t.Error("Truncated = false, want true")
	}
}

// ── both paths set → error ─────────────────────────────────────────────

func TestGrep_BothPathsSet_Error(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	_, err := executeGrep(context.Background(), GrepInput{
		Pattern: "x", WorkspacePath: "src", AbsolutePath: "/tmp",
	}, r)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Errorf("err = %v", err)
	}
}

// ── permission policy ──────────────────────────────────────────────────

func TestPermission_DefaultRoot_Allow(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(GrepInput{Pattern: "x"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow", res.Behavior)
	}
}

func TestPermission_AbsoluteOutside_Ask(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t)
	in, _ := json.Marshal(GrepInput{Pattern: "x", AbsolutePath: outside})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAsk {
		t.Errorf("Behavior = %v, want Ask", res.Behavior)
	}
	if len(res.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d", len(res.Suggestions))
	}
	if !strings.HasPrefix(res.Suggestions[0].Rule, "grep.absolute_path:") {
		t.Errorf("wrong prefix: %q", res.Suggestions[0].Rule)
	}
}

// ── VCS exclusion ──────────────────────────────────────────────────────

func TestGrep_ExcludesGitDir(t *testing.T) {
	root := tempRoot(t)
	setupFiles(t, root, map[string]string{
		"a.go":        "func main()\n",
		".git/HEAD":   "ref: refs/heads/main\n",
		".git/config": "[core]\n",
	})
	r := &fakeResolver{root: root}
	out, err := executeGrep(context.Background(), GrepInput{Pattern: "ref"}, r)
	if err != nil {
		t.Fatal(err)
	}
	// .git/ files should be excluded from results.
	if strings.Contains(out.Content, ".git") {
		t.Errorf("Content includes .git files: %q", out.Content)
	}
}
