// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// TestCache_GC_RemovesTmpDirs guards the .tmp sweep — Cache.Save writes
// to a sibling <sidecar>.tmp/ folder and renames into place; a kill
// between mkdir and rename leaves the .tmp behind. GC removes them
// unconditionally regardless of the keep allowlist.
func TestCache_GC_RemovesTmpDirs(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	// Simulate two abandoned .tmp dirs from prior crashed Save calls.
	for _, name := range []string{"ktw-example.tmp", "ktw-docs.tmp"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
		// Drop a placeholder file so RemoveAll has something to do.
		if err := os.WriteFile(filepath.Join(path, "metadata.yaml"), []byte("partial"), 0o644); err != nil {
			t.Fatalf("seed file in %s: %v", name, err)
		}
	}

	result, err := cache.GC(nil) // empty keep → tmp sweep only
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(result.TmpDirsRemoved) != 2 {
		t.Errorf("TmpDirsRemoved = %v, want 2 entries", result.TmpDirsRemoved)
	}
	if len(result.OrphansRemoved) != 0 {
		t.Errorf("OrphansRemoved = %v, want 0 (no allowlist provided)", result.OrphansRemoved)
	}

	// Verify the dirs really are gone.
	for _, name := range []string{"ktw-example.tmp", "ktw-docs.tmp"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should be removed, got err=%v", name, err)
		}
	}
}

// TestCache_GC_RemovesOrphans guards the orphan sweep — sidecar folders
// not in the `keep` allowlist get removed. Used by the hub's
// `catalog cache gc` subcommand to clean entries for sidecars no longer
// in Univers.yaml.
func TestCache_GC_RemovesOrphans(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	// Seed three valid cache entries.
	for _, name := range []string{"ktw-example", "ktw-docs", "ktw-orphan"} {
		seedCache(t, cache, makeManifest(name, "1.0", "sha256:"+name))
	}

	// Allowlist: ktw-example + ktw-docs. ktw-orphan should be removed.
	result, err := cache.GC([]string{"ktw-example", "ktw-docs"})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if want := filepath.Base(cache.RecordDir("ktw-orphan", testExePath(cache, "ktw-orphan"))); len(result.OrphansRemoved) != 1 || result.OrphansRemoved[0] != want {
		t.Errorf("OrphansRemoved = %v, want [%s] (the record folder, name@key)", result.OrphansRemoved, want)
	}
	if len(result.TmpDirsRemoved) != 0 {
		t.Errorf("TmpDirsRemoved = %v, want 0 (no .tmp seeded)", result.TmpDirsRemoved)
	}

	// Surviving entries should be addressable via ListNames.
	names, err := cache.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	sort.Strings(names)
	wantNames := []string{"ktw-docs", "ktw-example"}
	if len(names) != len(wantNames) {
		t.Errorf("ListNames = %v, want %v", names, wantNames)
	} else {
		for i := range names {
			if names[i] != wantNames[i] {
				t.Errorf("ListNames[%d] = %q, want %q", i, names[i], wantNames[i])
				break
			}
		}
	}
}

// TestCache_GC_EmptyKeep_DoesNotTouchValidEntries guards against the
// foot-gun where passing an empty `keep` (e.g. accidentally passing
// `nil` when the universe is mid-edit) would wipe every cached
// sidecar. The implementation explicitly checks `len(keepSet) > 0`
// before considering entries orphan-candidates.
func TestCache_GC_EmptyKeep_DoesNotTouchValidEntries(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	seedCache(t, cache, makeManifest("ktw-example", "1.0", "sha256:a"))
	seedCache(t, cache, makeManifest("ktw-docs", "1.0", "sha256:b"))

	result, err := cache.GC(nil)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(result.OrphansRemoved) != 0 {
		t.Errorf("OrphansRemoved = %v, want 0 (empty keep should not touch valid entries)",
			result.OrphansRemoved)
	}
	names, _ := cache.ListNames()
	if len(names) != 2 {
		t.Errorf("expected 2 entries to survive, got %d: %v", len(names), names)
	}
}

// TestCache_GC_NonExistentRoot returns empty without erroring — the
// hub's `cache gc` command runs even when the cache has never been
// populated (no folder created yet by Save).
func TestCache_GC_NonExistentRoot(t *testing.T) {
	cache := NewCache(filepath.Join(t.TempDir(), "does-not-exist"))
	result, err := cache.GC([]string{"ktw-example"})
	if err != nil {
		t.Fatalf("GC on missing root should not error: %v", err)
	}
	if result == nil || len(result.TmpDirsRemoved) != 0 || len(result.OrphansRemoved) != 0 {
		t.Errorf("expected empty result, got %+v", result)
	}
}

// TestCache_GC_BothClassesRemoved exercises the mixed case: both
// .tmp leftovers and orphans get removed in a single pass.
func TestCache_GC_BothClassesRemoved(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)

	// Two valid entries; we'll allowlist only one.
	seedCache(t, cache, makeManifest("ktw-example", "1.0", "sha256:a"))
	seedCache(t, cache, makeManifest("ktw-removed", "1.0", "sha256:b"))
	// One abandoned .tmp.
	tmpDir := filepath.Join(dir, "ktw-stale.tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		t.Fatalf("seed .tmp: %v", err)
	}

	result, err := cache.GC([]string{"ktw-example"})
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if len(result.TmpDirsRemoved) != 1 || result.TmpDirsRemoved[0] != "ktw-stale.tmp" {
		t.Errorf("TmpDirsRemoved = %v, want [ktw-stale.tmp]", result.TmpDirsRemoved)
	}
	if want := filepath.Base(cache.RecordDir("ktw-removed", testExePath(cache, "ktw-removed"))); len(result.OrphansRemoved) != 1 || result.OrphansRemoved[0] != want {
		t.Errorf("OrphansRemoved = %v, want [%s] (the record folder, name@key)", result.OrphansRemoved, want)
	}
}

// TestCache_ListNames_FiltersTmp confirms the side door: ListNames
// hides .tmp dirs so the hub's `cache list` command doesn't
// accidentally surface them as if they were real sidecar entries
// (only `cache gc` knows about them).
func TestCache_ListNames_FiltersTmp(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	seedCache(t, cache, makeManifest("ktw-example", "1.0", "sha256:a"))
	// Mix in a .tmp leftover.
	if err := os.MkdirAll(filepath.Join(dir, "ktw-stale.tmp"), 0o755); err != nil {
		t.Fatalf("seed .tmp: %v", err)
	}

	names, err := cache.ListNames()
	if err != nil {
		t.Fatalf("ListNames: %v", err)
	}
	if len(names) != 1 || names[0] != "ktw-example" {
		t.Errorf("ListNames = %v, want [ktw-example] (.tmp must be filtered)", names)
	}
}

// A folder member's record (taken from a folder, Save sees a directory) is
// swept once its folder is gone; while the folder stays it is kept, and a
// record taken from a binary is never swept for its binary's absence.
func TestCache_GC_SweepsAFolderRecordWhoseFolderIsGone(t *testing.T) {
	cache := NewCache(t.TempDir())
	folder := t.TempDir()
	if err := cache.Save("pack", &mbapi.Manifest{Provider: "pack"}, folder, SourceDigests{}); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "ktw-a.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cache.Save("ktw-a", &mbapi.Manifest{Provider: "ktw-a"}, exe, SourceDigests{}); err != nil {
		t.Fatal(err)
	}
	keep := []string{"pack", "ktw-a"}
	if r, err := cache.GC(keep); err != nil || len(r.OrphansRemoved) != 0 {
		t.Fatalf("GC with both sources present: %v, %v; want nothing swept", r, err)
	}
	if err := os.RemoveAll(folder); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	r, err := cache.GC(keep)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.OrphansRemoved) != 1 || r.OrphansRemoved[0] != filepath.Base(cache.RecordDir("pack", folder)) {
		t.Fatalf("swept %v, want the folder member's record alone", r.OrphansRemoved)
	}
}
