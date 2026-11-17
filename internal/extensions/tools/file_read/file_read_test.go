// SPDX-License-Identifier: Apache-2.0

package file_read

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
)

// ── helpers ─────────────────────────────────────────────────────────────

// newRunner creates a temp workspace, writes the given files, and
// returns the resolver, the workspace root, and a function to call
// runRead directly (bypassing the typed-tool wrapper). The runner
// uses a fresh in-memory filestate store attached to context, so
// dedup tests get an isolated cache per call.
func newRunner(t *testing.T, files map[string]string) (resolver *fakeResolver, root string, run func(ReadInput) (ReadOutput, error)) {
	t.Helper()
	root = tempRoot(t)
	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	resolver = &fakeResolver{root: root}
	store := filestate.NewMemoryStore(0)
	ctx := withTestStore(context.Background(), store)
	run = func(in ReadInput) (ReadOutput, error) {
		return runRead(ctx, in, resolver)
	}
	return resolver, root, run
}

// runnerWithStore is like newRunner but lets the caller share the
// store across multiple invocations (used for dedup tests).
func runnerWithStore(t *testing.T, files map[string]string, store filestate.Store) (*fakeResolver, string, func(ReadInput) (ReadOutput, error)) {
	t.Helper()
	root := tempRoot(t)
	for name, content := range files {
		full := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	resolver := &fakeResolver{root: root}
	ctx := withTestStore(context.Background(), store)
	run := func(in ReadInput) (ReadOutput, error) {
		return runRead(ctx, in, resolver)
	}
	return resolver, root, run
}

// testStoreKey is a context key used by tests to inject a filestate
// Store without going through application.UseService. The handler's
// filestate.From(ctx) lookup falls back to this key when no
// Application is registered. For tests we override the lookup by
// passing a synthesized context.
type testStoreKey struct{}

func withTestStore(ctx context.Context, store filestate.Store) context.Context {
	return context.WithValue(ctx, testStoreKey{}, store)
}

func withTestConfig(ctx context.Context, cfg *FileReadConfig) context.Context {
	return context.WithValue(ctx, testConfigKey{}, cfg)
}

// init: replace the handler's filestate lookup with a test-aware
// shim. This is the cleanest way to inject a store without spinning
// up the full Application stack.
//
// We do this by overriding the package-level filestateFrom variable
// in the test build only — see the corresponding declaration below.
func init() {
	filestateFrom = func(ctx context.Context) filestate.Store {
		if s, ok := ctx.Value(testStoreKey{}).(filestate.Store); ok {
			return s
		}
		return nil
	}
}

func intP(n int) *int { return &n }

func mustText(t *testing.T, out ReadOutput) TextOutputFile {
	t.Helper()
	if out.Type != OutputTypeText {
		t.Fatalf("expected text output, got type=%q", out.Type)
	}
	tf, ok := out.File.(TextOutputFile)
	if !ok {
		t.Fatalf("expected TextOutputFile, got %T", out.File)
	}
	return tf
}

// ── happy path: text reading ────────────────────────────────────────────

func TestRunRead_HappyPath_FullFile(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{
		"a.txt": "line one\nline two\nline three\n",
	})
	out, err := run(ReadInput{WorkspacePath: "a.txt"})
	if err != nil {
		t.Fatalf("runRead: %v", err)
	}
	tf := mustText(t, out)
	if tf.NumLines != 3 || tf.TotalLines != 3 || tf.StartLine != 1 {
		t.Errorf("counts wrong: NumLines=%d TotalLines=%d StartLine=%d", tf.NumLines, tf.TotalLines, tf.StartLine)
	}
	wantLines := []string{"     1\tline one", "     2\tline two", "     3\tline three"}
	for _, w := range wantLines {
		if !strings.Contains(tf.Content, w) {
			t.Errorf("Content missing %q\nfull:\n%s", w, tf.Content)
		}
	}
	if out.Warning != "" {
		t.Errorf("unexpected warning: %q", out.Warning)
	}
}

func TestRunRead_OffsetAndLimit(t *testing.T) {
	body := ""
	for i := 1; i <= 20; i++ {
		body += fmt.Sprintf("line%d\n", i)
	}
	_, _, run := newRunner(t, map[string]string{"big.txt": body})

	out, err := run(ReadInput{
		WorkspacePath: "big.txt",
		Offset:        intP(5),
		Limit:         intP(3),
	})
	if err != nil {
		t.Fatalf("runRead: %v", err)
	}
	tf := mustText(t, out)
	if tf.NumLines != 3 {
		t.Errorf("NumLines = %d, want 3", tf.NumLines)
	}
	if tf.StartLine != 6 {
		t.Errorf("StartLine = %d, want 6 (offset 5 → line 6 1-based)", tf.StartLine)
	}
	if tf.TotalLines != 20 {
		t.Errorf("TotalLines = %d, want 20", tf.TotalLines)
	}
	// First line in output should be line6 with display number 6.
	if !strings.Contains(tf.Content, "     6\tline6") {
		t.Errorf("Content missing first windowed line: %s", tf.Content)
	}
	if !strings.Contains(tf.Content, "     8\tline8") {
		t.Errorf("Content missing last windowed line: %s", tf.Content)
	}
	if strings.Contains(tf.Content, "line9") {
		t.Errorf("Content should not contain line9: %s", tf.Content)
	}
}

// ── empty file and offset past EOF (warnings, not errors) ───────────────

func TestRunRead_EmptyFile_Warning(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"empty.txt": ""})
	out, err := run(ReadInput{WorkspacePath: "empty.txt"})
	if err != nil {
		t.Fatalf("runRead: %v (expected warning, not error)", err)
	}
	tf := mustText(t, out)
	if tf.NumLines != 0 || tf.TotalLines != 0 {
		t.Errorf("counts: NumLines=%d TotalLines=%d, want 0/0", tf.NumLines, tf.TotalLines)
	}
	if out.Warning == "" || !strings.Contains(out.Warning, "empty") {
		t.Errorf("expected empty-file warning, got %q", out.Warning)
	}
}

func TestRunRead_OffsetPastEOF_Warning(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"x.txt": "a\nb\n"})
	out, err := run(ReadInput{WorkspacePath: "x.txt", Offset: intP(100)})
	if err != nil {
		t.Fatalf("runRead: %v", err)
	}
	tf := mustText(t, out)
	if tf.NumLines != 0 {
		t.Errorf("NumLines = %d, want 0", tf.NumLines)
	}
	if tf.TotalLines != 2 {
		t.Errorf("TotalLines = %d, want 2", tf.TotalLines)
	}
	if !strings.Contains(out.Warning, "past end of file") {
		t.Errorf("expected past-EOF warning, got %q", out.Warning)
	}
}

// ── caps ────────────────────────────────────────────────────────────────

func TestRunRead_SizeCap_RejectsLargeFile(t *testing.T) {
	// Generate a file just over the cap. We override the cap via
	// a tiny config so the test stays fast.
	root := tempRoot(t)
	big := strings.Repeat("a", 1024) // 1 KB
	if err := os.WriteFile(filepath.Join(root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}

	resolver := &fakeResolver{root: root}
	ctx := withTestConfig(withTestStore(context.Background(), filestate.NewMemoryStore(0)), &FileReadConfig{
		MaxBytes:      512, // smaller than the file
		MaxTokens:     25000,
		DefaultLines:  2000,
		TokensPerChar: 4,
	})
	_, err := runRead(ctx, ReadInput{WorkspacePath: "big.txt"}, resolver)
	if err == nil {
		t.Fatal("expected size cap rejection")
	}
	if !strings.Contains(err.Error(), "exceeds the 512-byte cap") {
		t.Errorf("error = %q, want size-cap message", err.Error())
	}
}

func TestRunRead_TokenCap_RejectsLargeOutput(t *testing.T) {
	// 2000 chars formatted with line numbers easily exceeds a
	// 100-token cap.
	body := strings.Repeat("xxxxxxxxxx\n", 200) // 200 lines, ~10 chars each
	root := tempRoot(t)
	if err := os.WriteFile(filepath.Join(root, "x.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver := &fakeResolver{root: root}
	ctx := withTestConfig(withTestStore(context.Background(), filestate.NewMemoryStore(0)), &FileReadConfig{
		MaxBytes:      1 << 20,
		MaxTokens:     50, // very low
		DefaultLines:  2000,
		TokensPerChar: 4,
	})
	_, err := runRead(ctx, ReadInput{WorkspacePath: "x.txt"}, resolver)
	if err == nil || !strings.Contains(err.Error(), "exceeds the 50-token cap") {
		t.Errorf("expected token-cap error, got %v", err)
	}
}

// ── absolute path branch and workspace boundary ─────────────────────────

func TestRunRead_AbsolutePath_InsideWorkspace_GetsHint(t *testing.T) {
	_, root, run := newRunner(t, map[string]string{"hello.txt": "hi\n"})
	abs := filepath.Join(root, "hello.txt")

	out, err := run(ReadInput{AbsolutePath: abs})
	if err != nil {
		t.Fatalf("runRead: %v", err)
	}
	tf := mustText(t, out)
	if tf.NumLines != 1 {
		t.Errorf("NumLines = %d, want 1", tf.NumLines)
	}
	if !strings.Contains(out.Warning, "prefer 'workspace_path'") {
		t.Errorf("expected workspace-hint warning, got %q", out.Warning)
	}
}

func TestRunRead_AbsolutePath_OutsideWorkspace_NoHint(t *testing.T) {
	// Create a file outside the workspace by making a separate temp dir.
	outside := t.TempDir()
	outFile := filepath.Join(outside, "outer.txt")
	if err := os.WriteFile(outFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, run := newRunner(t, nil)
	out, err := run(ReadInput{AbsolutePath: outFile})
	if err != nil {
		t.Fatalf("runRead: %v", err)
	}
	if strings.Contains(out.Warning, "prefer 'workspace_path'") {
		t.Errorf("outside-workspace read should NOT get the prefer-workspace_path hint, got %q", out.Warning)
	}
}

// ── validation errors propagate from the handler ────────────────────────

func TestRunRead_BothPathsSet(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"a.txt": "x"})
	_, err := run(ReadInput{WorkspacePath: "a.txt", AbsolutePath: "/etc/hosts"})
	if !errors.Is(err, ErrPathFieldsBoth) {
		t.Errorf("err = %v, want ErrPathFieldsBoth", err)
	}
}

func TestRunRead_NeitherPathSet(t *testing.T) {
	_, _, run := newRunner(t, nil)
	_, err := run(ReadInput{})
	if !errors.Is(err, ErrPathFieldsNeither) {
		t.Errorf("err = %v, want ErrPathFieldsNeither", err)
	}
}

func TestRunRead_BinaryRejected(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"bin.exe": "MZ\x90"})
	_, err := run(ReadInput{WorkspacePath: "bin.exe"})
	if !errors.Is(err, ErrBinaryRejected) {
		t.Errorf("err = %v, want ErrBinaryRejected", err)
	}
}

func TestRunRead_DirectoryRejected(t *testing.T) {
	root := tempRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &fakeResolver{root: root}
	ctx := withTestStore(context.Background(), filestate.NewMemoryStore(0))
	_, err := runRead(ctx, ReadInput{WorkspacePath: "subdir"}, resolver)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("err = %v, want directory error", err)
	}
}

func TestRunRead_FileNotFound(t *testing.T) {
	_, _, run := newRunner(t, nil)
	_, err := run(ReadInput{WorkspacePath: "missing.txt"})
	if err == nil {
		t.Fatal("expected file-not-found error")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("err = %v, want stat error", err)
	}
}

// ── deduplication ───────────────────────────────────────────────────────

func TestRunRead_Dedup_HitOnSameWindow(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, _, run := runnerWithStore(t, map[string]string{"a.txt": "one\ntwo\n"}, store)

	first, err := run(ReadInput{WorkspacePath: "a.txt"})
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if first.Type != OutputTypeText {
		t.Fatalf("first read type = %q, want text", first.Type)
	}

	second, err := run(ReadInput{WorkspacePath: "a.txt"})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if second.Type != OutputTypeFileUnchanged {
		t.Errorf("second read type = %q, want file_unchanged (dedup hit)", second.Type)
	}
}

func TestRunRead_Dedup_MissOnMtimeChange(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.txt": "one\n"}, store)

	if _, err := run(ReadInput{WorkspacePath: "a.txt"}); err != nil {
		t.Fatalf("first read: %v", err)
	}

	// Advance mtime by re-writing the file with a newer time.
	target := filepath.Join(root, "a.txt")
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(target, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	out, err := run(ReadInput{WorkspacePath: "a.txt"})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if out.Type != OutputTypeText {
		t.Errorf("second read type = %q, want text (mtime changed → no dedup)", out.Type)
	}
}

func TestRunRead_Dedup_MissOnDifferentOffset(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, _, run := runnerWithStore(t, map[string]string{"a.txt": "one\ntwo\nthree\n"}, store)

	if _, err := run(ReadInput{WorkspacePath: "a.txt", Offset: intP(0), Limit: intP(2)}); err != nil {
		t.Fatalf("first read: %v", err)
	}
	out, err := run(ReadInput{WorkspacePath: "a.txt", Offset: intP(1), Limit: intP(2)})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if out.Type != OutputTypeText {
		t.Errorf("second read type = %q, want text (different offset → no dedup)", out.Type)
	}
}

func TestRunRead_Dedup_MissOnWriteOriginEntry(t *testing.T) {
	// Pre-populate the store with an entry that has nil Offset/Limit
	// (simulating a file_write recording). file_read MUST NOT dedup
	// against it (FR-FR-10).
	root := tempRoot(t)
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(target)

	store := filestate.NewMemoryStore(0)
	store.Record(target, filestate.Entry{
		Content:   "post-write content",
		Timestamp: info.ModTime(),
		// Offset and Limit deliberately nil → write-origin entry
	})

	resolver := &fakeResolver{root: root}
	ctx := withTestStore(context.Background(), store)
	out, err := runRead(ctx, ReadInput{WorkspacePath: "a.txt"}, resolver)
	if err != nil {
		t.Fatalf("runRead: %v", err)
	}
	if out.Type != OutputTypeText {
		t.Errorf("type = %q, want text (write-origin entry must not be dedup target)", out.Type)
	}
}

// ── permission policy ───────────────────────────────────────────────────

func TestPermission_WorkspacePath_Allow(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(ReadInput{WorkspacePath: "a.txt"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow", res.Behavior)
	}
}

func TestPermission_AbsolutePath_InsideWorkspace_Allow(t *testing.T) {
	root := tempRoot(t)
	r := &fakeResolver{root: root}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(ReadInput{AbsolutePath: filepath.Join(root, "a.txt")})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow (inside workspace)", res.Behavior)
	}
}

func TestPermission_AbsolutePath_Outside_Ask(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t) // separate dir, definitely not under r.root
	target := filepath.Join(outside, "secret.txt")
	in, _ := json.Marshal(ReadInput{AbsolutePath: target})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAsk {
		t.Errorf("Behavior = %v, want Ask (outside workspace)", res.Behavior)
	}
	if !strings.Contains(res.Message, "outside the workspace") {
		t.Errorf("message = %q, want 'outside the workspace'", res.Message)
	}
	// Rules system fields must be populated. The Dimensions value is
	// normalized to forward slashes (gitignore convention) so the
	// doublestar matcher behaves consistently across Windows and POSIX.
	wantSlash := filepath.ToSlash(target)
	if res.Dimensions == nil || res.Dimensions["absolute_path"] != wantSlash {
		t.Errorf("Dimensions[absolute_path] = %q, want %q", res.Dimensions["absolute_path"], wantSlash)
	}
}

func TestPermission_AbsolutePath_Outside_Suggestions(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t)
	target := filepath.Join(outside, "secret.txt")
	in, _ := json.Marshal(ReadInput{AbsolutePath: target})
	res := policy(context.Background(), in)

	if len(res.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d, want 3 (exact / parent / any)", len(res.Suggestions))
	}

	// All suggestion rules use forward-slash form so they're
	// portable across Windows and POSIX.
	slashTarget := filepath.ToSlash(target)

	// 1. Exact-file suggestion
	wantExact := "file-read.absolute_path:" + slashTarget
	if res.Suggestions[0].Rule != wantExact {
		t.Errorf("first suggestion rule = %q, want %q", res.Suggestions[0].Rule, wantExact)
	}

	// 2. Parent-directory suggestion. Built by stripping the last
	// "/" segment from the slash form (matches the helper).
	parent := slashTarget[:strings.LastIndex(slashTarget, "/")]
	wantParent := "file-read.absolute_path:" + parent + "/**"
	if res.Suggestions[1].Rule != wantParent {
		t.Errorf("parent suggestion rule = %q, want %q", res.Suggestions[1].Rule, wantParent)
	}

	// 3. Broadest suggestion (any out-of-workspace read)
	if res.Suggestions[2].Rule != "file-read.absolute_path:**" {
		t.Errorf("broad suggestion rule = %q, want file-read.absolute_path:**", res.Suggestions[2].Rule)
	}
}

func TestPermission_BothPathsSet_Deny(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(ReadInput{WorkspacePath: "a", AbsolutePath: "/b"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionDeny {
		t.Errorf("Behavior = %v, want Deny", res.Behavior)
	}
}

func TestPermission_MalformedJSON_Deny(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	res := policy(context.Background(), json.RawMessage(`{not json`))
	if res.Behavior != toolsapi.PermissionDeny {
		t.Errorf("Behavior = %v, want Deny", res.Behavior)
	}
	if !strings.Contains(res.Message, "malformed") {
		t.Errorf("message = %q, want 'malformed'", res.Message)
	}
}

// ── describer ──────────────────────────────────────────────────────────

func TestDescribeRead(t *testing.T) {
	cases := []struct {
		name string
		in   ReadInput
		want string
	}{
		{
			"workspace plain",
			ReadInput{WorkspacePath: "src/foo.go"},
			"file-read workspace_path=src/foo.go",
		},
		{
			"absolute plain",
			ReadInput{AbsolutePath: "/etc/hosts"},
			"file-read absolute_path=/etc/hosts",
		},
		{
			"workspace with offset+limit",
			ReadInput{WorkspacePath: "big.log", Offset: intP(1000), Limit: intP(200)},
			"file-read workspace_path=big.log offset=1000 limit=200",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, _ := json.Marshal(c.in)
			got := describeRead(raw)
			if got != c.want {
				t.Errorf("describeRead = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDescribeRead_InvalidJSON(t *testing.T) {
	got := describeRead(json.RawMessage(`{not json`))
	if !strings.Contains(got, "invalid input") {
		t.Errorf("got = %q, want substring 'invalid input'", got)
	}
}
