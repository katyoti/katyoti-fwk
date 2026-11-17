// SPDX-License-Identifier: Apache-2.0

package file_write

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/filestate"
)

// ── helpers ─────────────────────────────────────────────────────────────

// newRunner creates a temp workspace, optionally pre-writes files
// into it, and returns the resolver, root, and a function to call
// runWrite directly with a fresh in-memory filestate store wired
// via the test seam.
func newRunner(t *testing.T, files map[string]string) (resolver *fakeResolver, root string, run func(WriteInput) (WriteOutput, error)) {
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
	run = func(in WriteInput) (WriteOutput, error) {
		return runWrite(ctx, in, resolver)
	}
	return resolver, root, run
}

// runnerWithStore is like newRunner but lets the caller share the
// store across multiple invocations (used for read-before-write
// tests where we pre-populate a read entry).
func runnerWithStore(t *testing.T, files map[string]string, store filestate.Store) (*fakeResolver, string, func(WriteInput) (WriteOutput, error)) {
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
	run := func(in WriteInput) (WriteOutput, error) {
		return runWrite(ctx, in, resolver)
	}
	return resolver, root, run
}

// testStoreKey is a context key used by tests to inject a filestate
// Store. The handler's `filestateFrom` indirection is overridden in
// init() to look at this key first.
type testStoreKey struct{}

func withTestStore(ctx context.Context, store filestate.Store) context.Context {
	return context.WithValue(ctx, testStoreKey{}, store)
}

func withTestConfig(ctx context.Context, cfg *FileWriteConfig) context.Context {
	return context.WithValue(ctx, testConfigKey{}, cfg)
}

// init replaces the handler's filestate lookup with a test-aware
// shim. This is the cleanest way to inject a store without spinning
// up the full Application stack.
//
// **Why duplicated** from file_read: file_write has its own
// `filestateFrom` package var (see file-write.go). The two will be
// extracted to a shared helper after the third file-tool consumer
// (file_edit) lands.
func init() {
	filestateFrom = func(ctx context.Context) filestate.Store {
		if s, ok := ctx.Value(testStoreKey{}).(filestate.Store); ok {
			return s
		}
		return nil
	}
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func mustCreate(t *testing.T, out WriteOutput) CreateOutputFile {
	t.Helper()
	if out.Type != OutputTypeCreate {
		t.Fatalf("expected create output, got type=%q", out.Type)
	}
	cf, ok := out.File.(CreateOutputFile)
	if !ok {
		t.Fatalf("expected CreateOutputFile, got %T", out.File)
	}
	return cf
}

func mustUpdate(t *testing.T, out WriteOutput) UpdateOutputFile {
	t.Helper()
	if out.Type != OutputTypeUpdate {
		t.Fatalf("expected update output, got type=%q", out.Type)
	}
	uf, ok := out.File.(UpdateOutputFile)
	if !ok {
		t.Fatalf("expected UpdateOutputFile, got %T", out.File)
	}
	return uf
}

// ── Create new file ────────────────────────────────────────────────────

func TestRunWrite_CreateNewFile_NoPriorRead(t *testing.T) {
	_, root, run := newRunner(t, nil)

	out, err := run(WriteInput{
		WorkspacePath: "newfile.txt",
		Content:       "hello world\n",
	})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	cf := mustCreate(t, out)
	if cf.BytesWritten != 12 {
		t.Errorf("BytesWritten = %d, want 12", cf.BytesWritten)
	}

	got := mustReadFile(t, filepath.Join(root, "newfile.txt"))
	if got != "hello world\n" {
		t.Errorf("on-disk content = %q, want %q", got, "hello world\n")
	}
}

func TestRunWrite_CreateInNestedDir_AutoCreatesParents(t *testing.T) {
	_, root, run := newRunner(t, nil)

	out, err := run(WriteInput{
		WorkspacePath: "deep/nested/dir/file.txt",
		Content:       "x",
	})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	if out.Type != OutputTypeCreate {
		t.Errorf("type = %q, want create", out.Type)
	}

	got := mustReadFile(t, filepath.Join(root, "deep/nested/dir/file.txt"))
	if got != "x" {
		t.Errorf("content = %q, want %q", got, "x")
	}
}

func TestRunWrite_EmptyContent_AccepptedForCreate(t *testing.T) {
	_, root, run := newRunner(t, nil)

	out, err := run(WriteInput{
		WorkspacePath: "placeholder.txt",
		Content:       "",
	})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	cf := mustCreate(t, out)
	if cf.BytesWritten != 0 {
		t.Errorf("BytesWritten = %d, want 0", cf.BytesWritten)
	}
	// File must actually exist on disk.
	if _, err := os.Stat(filepath.Join(root, "placeholder.txt")); err != nil {
		t.Errorf("placeholder file not created: %v", err)
	}
}

// ── Overwrite existing file (with prior read) ──────────────────────────

func TestRunWrite_OverwriteWithPriorRead_Succeeds(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.txt": "old\n"}, store)

	// Pre-populate the filestate as if file_read had read this file.
	target := filepath.Join(root, "a.txt")
	info, _ := os.Stat(target)
	zero := 0
	bigLimit := 2000
	store.Record(target, filestate.Entry{
		Content:   "old\n",
		Timestamp: info.ModTime(),
		Offset:    &zero,
		Limit:     &bigLimit,
	})

	out, err := run(WriteInput{WorkspacePath: "a.txt", Content: "new\n"})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	uf := mustUpdate(t, out)
	if uf.BytesWritten != 4 {
		t.Errorf("BytesWritten = %d, want 4", uf.BytesWritten)
	}

	got := mustReadFile(t, target)
	if got != "new\n" {
		t.Errorf("on-disk content = %q, want %q", got, "new\n")
	}
}

// ── Reject overwrite without prior read ───────────────────────────────

func TestRunWrite_RejectOverwriteWithoutPriorRead(t *testing.T) {
	_, _, run := newRunner(t, map[string]string{"a.txt": "old\n"})
	// No filestate entry → write must be rejected.
	_, err := run(WriteInput{WorkspacePath: "a.txt", Content: "new\n"})
	if err == nil {
		t.Fatal("expected error (no prior read)")
	}
	if !strings.Contains(err.Error(), "must be read first") {
		t.Errorf("err = %v, want substring 'must be read first'", err)
	}
}

func TestRunWrite_RejectOverwriteWithWriteOriginEntry(t *testing.T) {
	// Pre-populate the cache with a write-origin entry (Offset/Limit nil).
	// file_write must NOT treat that as a valid prior-read marker.
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.txt": "old\n"}, store)
	target := filepath.Join(root, "a.txt")
	info, _ := os.Stat(target)
	store.Record(target, filestate.Entry{
		Content:   "old\n",
		Timestamp: info.ModTime(),
		// Offset and Limit deliberately nil → write-origin entry
	})

	_, err := run(WriteInput{WorkspacePath: "a.txt", Content: "new\n"})
	if err == nil {
		t.Fatal("expected error (write-origin entry should not satisfy read-before-write)")
	}
	if !strings.Contains(err.Error(), "must be read first") {
		t.Errorf("err = %v, want substring 'must be read first'", err)
	}
}

// ── Reject overwrite with stale mtime ─────────────────────────────────

func TestRunWrite_RejectOverwriteWithStaleMtime(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, map[string]string{"a.txt": "old\n"}, store)
	target := filepath.Join(root, "a.txt")

	// Record a read entry with the file's CURRENT mtime, then
	// advance the mtime on disk to simulate an external change.
	info, _ := os.Stat(target)
	zero := 0
	bigLimit := 2000
	store.Record(target, filestate.Entry{
		Content:   "old\n",
		Timestamp: info.ModTime(),
		Offset:    &zero,
		Limit:     &bigLimit,
	})

	future := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(target, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	_, err := run(WriteInput{WorkspacePath: "a.txt", Content: "new\n"})
	if err == nil {
		t.Fatal("expected stale-mtime error")
	}
	if !strings.Contains(err.Error(), "changed on disk") {
		t.Errorf("err = %v, want substring 'changed on disk'", err)
	}
}

// ── LF normalization ──────────────────────────────────────────────────

func TestRunWrite_NormalizesCRLFToLF(t *testing.T) {
	_, root, run := newRunner(t, nil)
	out, err := run(WriteInput{
		WorkspacePath: "win.txt",
		Content:       "line1\r\nline2\r\nline3\r\n",
	})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	if out.Type != OutputTypeCreate {
		t.Fatalf("type = %q, want create", out.Type)
	}

	got := mustReadFile(t, filepath.Join(root, "win.txt"))
	if strings.Contains(got, "\r\n") {
		t.Errorf("on-disk content still contains CRLF: %q", got)
	}
	if got != "line1\nline2\nline3\n" {
		t.Errorf("got %q, want LF-normalized form", got)
	}
}

// ── Defensive checks ──────────────────────────────────────────────────

func TestRunWrite_BinaryExtensionRejected(t *testing.T) {
	_, _, run := newRunner(t, nil)
	_, err := run(WriteInput{WorkspacePath: "binary.exe", Content: "MZ"})
	if !errors.Is(err, ErrBinaryRejected) {
		t.Errorf("err = %v, want ErrBinaryRejected", err)
	}
}

func TestRunWrite_DirectoryRejected(t *testing.T) {
	root := tempRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolver := &fakeResolver{root: root}
	store := filestate.NewMemoryStore(0)
	target := filepath.Join(root, "subdir")
	info, _ := os.Stat(target)
	zero := 0
	bigLimit := 2000
	store.Record(target, filestate.Entry{
		Content: "", Timestamp: info.ModTime(), Offset: &zero, Limit: &bigLimit,
	})
	ctx := withTestStore(context.Background(), store)
	_, err := runWrite(ctx, WriteInput{WorkspacePath: "subdir", Content: "x"}, resolver)
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Errorf("err = %v, want directory error", err)
	}
}

func TestRunWrite_BothPathsSet(t *testing.T) {
	_, _, run := newRunner(t, nil)
	_, err := run(WriteInput{WorkspacePath: "a", AbsolutePath: "/b", Content: "x"})
	if !errors.Is(err, ErrPathFieldsBoth) {
		t.Errorf("err = %v, want ErrPathFieldsBoth", err)
	}
}

func TestRunWrite_NeitherPathSet(t *testing.T) {
	_, _, run := newRunner(t, nil)
	_, err := run(WriteInput{Content: "x"})
	if !errors.Is(err, ErrPathFieldsNeither) {
		t.Errorf("err = %v, want ErrPathFieldsNeither", err)
	}
}

func TestRunWrite_ContentOverCap(t *testing.T) {
	root := tempRoot(t)
	resolver := &fakeResolver{root: root}
	store := filestate.NewMemoryStore(0)
	ctx := withTestConfig(withTestStore(context.Background(), store), &FileWriteConfig{MaxBytes: 5})
	_, err := runWrite(ctx, WriteInput{WorkspacePath: "x.txt", Content: "abcdef"}, resolver)
	if !errors.Is(err, ErrContentTooLarge) {
		t.Errorf("err = %v, want ErrContentTooLarge", err)
	}
}

// ── filestate not registered for existing file → fail-closed ─────────

func TestRunWrite_NoFilestate_ExistingFile_FailsClosed(t *testing.T) {
	root := tempRoot(t)
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolver := &fakeResolver{root: root}
	// No store in context.
	_, err := runWrite(context.Background(), WriteInput{WorkspacePath: "a.txt", Content: "new\n"}, resolver)
	if err == nil {
		t.Fatal("expected fail-closed error when filestate is missing")
	}
	if !strings.Contains(err.Error(), "filestate cache not registered") {
		t.Errorf("err = %v, want 'filestate cache not registered'", err)
	}
}

func TestRunWrite_NoFilestate_NewFile_StillSucceeds(t *testing.T) {
	root := tempRoot(t)
	resolver := &fakeResolver{root: root}
	// No store in context, but the file doesn't exist → no
	// read-before-write check needed → write should succeed.
	out, err := runWrite(context.Background(), WriteInput{WorkspacePath: "new.txt", Content: "x"}, resolver)
	if err != nil {
		t.Fatalf("expected new-file write to succeed without store: %v", err)
	}
	if out.Type != OutputTypeCreate {
		t.Errorf("type = %q, want create", out.Type)
	}
}

// ── Filestate updated after successful write ─────────────────────────

func TestRunWrite_RecordsWriteEntry_AfterSuccess(t *testing.T) {
	store := filestate.NewMemoryStore(0)
	_, root, run := runnerWithStore(t, nil, store)

	_, err := run(WriteInput{WorkspacePath: "fresh.txt", Content: "hello"})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}

	target := filepath.Join(root, "fresh.txt")
	entry, ok := store.Lookup(target)
	if !ok {
		t.Fatal("filestate entry missing after successful write")
	}
	if entry.IsReadEntry() {
		t.Error("recorded entry should be a write-origin entry (Offset/Limit nil)")
	}
	if entry.Content != "hello" {
		t.Errorf("entry.Content = %q, want %q", entry.Content, "hello")
	}
}

// ── Permission policy ─────────────────────────────────────────────────

func TestPermission_WorkspacePath_Allow(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(WriteInput{WorkspacePath: "a.txt", Content: "x"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow", res.Behavior)
	}
}

func TestPermission_AbsolutePath_InsideWorkspace_Allow(t *testing.T) {
	root := tempRoot(t)
	r := &fakeResolver{root: root}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(WriteInput{AbsolutePath: filepath.Join(root, "a.txt"), Content: "x"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("Behavior = %v, want Allow (inside workspace)", res.Behavior)
	}
}

func TestPermission_AbsolutePath_Outside_Ask(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t)
	target := filepath.Join(outside, "secret.txt")
	in, _ := json.Marshal(WriteInput{AbsolutePath: target, Content: "x"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAsk {
		t.Errorf("Behavior = %v, want Ask", res.Behavior)
	}
	if !strings.Contains(res.Message, "outside the workspace") {
		t.Errorf("message = %q", res.Message)
	}
	// Dimensions normalized to forward slashes.
	wantSlash := filepath.ToSlash(target)
	if res.Dimensions["absolute_path"] != wantSlash {
		t.Errorf("Dimensions[absolute_path] = %q, want %q", res.Dimensions["absolute_path"], wantSlash)
	}
}

func TestPermission_AbsolutePath_Outside_Suggestions(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	outside := tempRoot(t)
	target := filepath.Join(outside, "spec.md")
	in, _ := json.Marshal(WriteInput{AbsolutePath: target, Content: "x"})
	res := policy(context.Background(), in)

	if len(res.Suggestions) != 3 {
		t.Fatalf("Suggestions len = %d, want 3", len(res.Suggestions))
	}

	// All suggestions use the file_write rule prefix and forward
	// slashes.
	slashTarget := filepath.ToSlash(target)
	wantExact := "file-write.absolute_path:" + slashTarget
	if res.Suggestions[0].Rule != wantExact {
		t.Errorf("first suggestion rule = %q, want %q", res.Suggestions[0].Rule, wantExact)
	}
	parent := slashTarget[:strings.LastIndex(slashTarget, "/")]
	wantParent := "file-write.absolute_path:" + parent + "/**"
	if res.Suggestions[1].Rule != wantParent {
		t.Errorf("parent suggestion rule = %q, want %q", res.Suggestions[1].Rule, wantParent)
	}
	if res.Suggestions[2].Rule != "file-write.absolute_path:**" {
		t.Errorf("broad suggestion rule = %q, want file-write.absolute_path:**", res.Suggestions[2].Rule)
	}
}

func TestPermission_BothPathsSet_Deny(t *testing.T) {
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	in, _ := json.Marshal(WriteInput{WorkspacePath: "a", AbsolutePath: "/b", Content: "x"})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionDeny {
		t.Errorf("Behavior = %v, want Deny", res.Behavior)
	}
}

func TestPermission_LargeContent_NotRejectedByGate(t *testing.T) {
	// The gate uses Validate(in, 0) — content size cap is the
	// handler's concern, not the gate's. A 10 MiB content should
	// pass the gate even though it would fail the handler's cap.
	r := &fakeResolver{root: tempRoot(t)}
	policy := makePermissionFunc(r)
	huge := strings.Repeat("a", 10<<20)
	in, _ := json.Marshal(WriteInput{WorkspacePath: "x.txt", Content: huge})
	res := policy(context.Background(), in)
	if res.Behavior != toolsapi.PermissionAllow {
		t.Errorf("gate rejected huge content (Behavior=%v); content size is the handler's concern, not the gate's", res.Behavior)
	}
}

// ── Inside-workspace absolute → hint warning ─────────────────────────

func TestRunWrite_AbsolutePath_InsideWorkspace_GetsHint(t *testing.T) {
	_, root, run := newRunner(t, nil)
	abs := filepath.Join(root, "hello.txt")

	out, err := run(WriteInput{AbsolutePath: abs, Content: "hi"})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	if !strings.Contains(out.Warning, "prefer 'workspace_path'") {
		t.Errorf("expected workspace-hint warning, got %q", out.Warning)
	}
}

func TestRunWrite_AbsolutePath_OutsideWorkspace_NoHint(t *testing.T) {
	outside := t.TempDir()
	outFile := filepath.Join(outside, "outer.txt")

	_, _, run := newRunner(t, nil)
	out, err := run(WriteInput{AbsolutePath: outFile, Content: "outside"})
	if err != nil {
		t.Fatalf("runWrite: %v", err)
	}
	if strings.Contains(out.Warning, "prefer 'workspace_path'") {
		t.Errorf("outside-workspace write should NOT get the hint, got %q", out.Warning)
	}
}

// ── Describer ────────────────────────────────────────────────────────

func TestDescribeWrite(t *testing.T) {
	cases := []struct {
		name string
		in   WriteInput
		want string
	}{
		{
			"workspace plain",
			WriteInput{WorkspacePath: "src/foo.go", Content: "abc"},
			"file-write workspace_path=src/foo.go (3 bytes)",
		},
		{
			"absolute plain",
			WriteInput{AbsolutePath: "/etc/hosts", Content: "abcdef"},
			"file-write absolute_path=/etc/hosts (6 bytes)",
		},
		{
			"empty content",
			WriteInput{WorkspacePath: "x.txt", Content: ""},
			"file-write workspace_path=x.txt (0 bytes)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw, _ := json.Marshal(c.in)
			got := describeWrite(raw)
			if got != c.want {
				t.Errorf("describeWrite = %q, want %q", got, c.want)
			}
		})
	}
}

func TestDescribeWrite_InvalidJSON(t *testing.T) {
	got := describeWrite(json.RawMessage(`{not json`))
	if !strings.Contains(got, "invalid input") {
		t.Errorf("got = %q", got)
	}
}
