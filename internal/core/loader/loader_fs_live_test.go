// SPDX-License-Identifier: Apache-2.0

package loader

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// loader_fs_live_test.go — a directory is a mutable source.
//
// The case that failed was an EDIT made from outside the process. Adding a file
// always worked, because a new name misses the cache; editing one did not,
// because the name was already there. Silent and partial is the worst shape of
// failure, so it gets its own test.

// writeFile writes content and forces a distinct mtime, because a test can
// easily rewrite a file within one filesystem timestamp tick — which is the one
// case the stat gate cannot see, and not the case under test.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
}

// TestFSLoader_SeesAnExternalEdit is the acceptance test for live sources: a
// file edited by someone else, after this loader has already read it.
func TestFSLoader_SeesAnExternalEdit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "skills/doc-create/SKILL.md", "version one")
	l := NewFSLoader(dir)

	got, err := l.ReadFile("skills/doc-create/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "version one" {
		t.Fatalf("first read = %q", got)
	}

	// Someone edits the file — not through this loader.
	writeFile(t, dir, "skills/doc-create/SKILL.md", "version two, edited by an author")

	got, err = l.ReadFile("skills/doc-create/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "version two, edited by an author" {
		t.Fatalf("after an external edit, read = %q — the cache served content that is no longer on disk", got)
	}
}

// TestFSLoader_SeesAFileAppearAndDisappear pins the two cases that DID work, so
// fixing the edit case cannot break them.
func TestFSLoader_SeesAFileAppearAndDisappear(t *testing.T) {
	dir := t.TempDir()
	l := NewFSLoader(dir)

	if got, _ := l.ReadFile("tasks/later.md"); got != nil {
		t.Fatalf("read before creation = %q, want nil", got)
	}
	writeFile(t, dir, "tasks/later.md", "now it exists")
	got, err := l.ReadFile("tasks/later.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "now it exists" {
		t.Fatalf("after creation, read = %q — a cached miss outlived the file's absence", got)
	}

	if err := os.Remove(filepath.Join(dir, "tasks/later.md")); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.ReadFile("tasks/later.md"); got != nil {
		t.Fatalf("after removal, read = %q, want nil", got)
	}
}

// TestFSLoader_UnchangedFileStaysCached asserts the gate did not cost the cache
// its purpose: a file nobody touched is served without re-reading.
func TestFSLoader_UnchangedFileStaysCached(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ref.md", "stable content")
	l := NewFSLoader(dir)

	if _, err := l.ReadFile("ref.md"); err != nil {
		t.Fatal(err)
	}
	// Replace the file's CONTENT on disk without changing its stamp, by
	// restoring the recorded size and mtime. A real edit cannot do this; it is
	// how the test observes whether the second read hit the cache.
	p := filepath.Join(dir, "ref.md")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("stable conten!"), 0o644); err != nil { // same length
		t.Fatal(err)
	}
	if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}

	got, err := l.ReadFile("ref.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "stable content" {
		t.Errorf("read = %q — an unchanged stamp should have been served from the cache", got)
	}
}

// TestFSLoader_InvalidateDropsEverything asserts the wholesale drop, for a
// caller told a source changed rather than one checking path by path.
func TestFSLoader_InvalidateDropsEverything(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "A1")
	l := NewFSLoader(dir)
	if _, err := l.ReadFile("a.md"); err != nil {
		t.Fatal(err)
	}

	// Same trick: change content while preserving the stamp, so only an
	// explicit Invalidate can make the next read see it.
	p := filepath.Join(dir, "a.md")
	fi, _ := os.Stat(p)
	if err := os.WriteFile(p, []byte("A2"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}

	l.Invalidate()
	got, err := l.ReadFile("a.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "A2" {
		t.Errorf("after Invalidate, read = %q, want A2", got)
	}
}

// TestFSLoader_SealedSourceCachesOutright asserts a declared-immutable
// directory is cached without re-checking — and, more importantly, that a LIVE
// one is not, since marking a live folder immutable would serve stale content
// for exactly the source designed to be updated.
func TestFSLoader_SealedSourceCachesOutright(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pack.md", "sealed at build time")

	sealed := NewFSLoader(dir).MarkImmutable()
	if !IsImmutable(sealed) {
		t.Fatal("MarkImmutable did not take")
	}
	if _, err := sealed.ReadFile("pack.md"); err != nil {
		t.Fatal(err)
	}
	// Edit it anyway. A sealed source is entitled to ignore this: the content
	// cannot change without rebuilding the artifact that carries it.
	writeFile(t, dir, "pack.md", "edited on disk, which cannot happen for a real sealed pack")
	got, err := sealed.ReadFile("pack.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "sealed at build time" {
		t.Errorf("a sealed source re-read: %q", got)
	}

	// The same folder, NOT declared sealed, sees the edit.
	live := NewFSLoader(dir)
	if IsImmutable(live) {
		t.Fatal("a plain FSLoader claimed to be immutable — mutable must be the default")
	}
	if _, err := live.ReadFile("pack.md"); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "pack.md", "a third version")
	got, err = live.ReadFile("pack.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "a third version" {
		t.Errorf("a live source did not see an edit: %q", got)
	}
}
