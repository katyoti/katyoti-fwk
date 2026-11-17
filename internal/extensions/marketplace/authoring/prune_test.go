// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"os"
	"path/filepath"
	"testing"
)

// Under the version-first layout `<version>/<arch>/<file>`, a version folder is
// SHARED by every item. These tests pin the invariant that pruning one item's
// version removes only that item's files — the bug a RemoveAll on the version
// folder would reintroduce.

func mkfile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPruneEmptyDirs_StopsAtOccupiedDir: a directory another item still occupies
// must survive.
func TestPruneEmptyDirs_StopsAtOccupiedDir(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "1.0.0", "windows-amd64", "ktw-hub.exe")
	bystander := filepath.Join(root, "1.0.0", "windows-amd64", "ktw-docs.exe")
	mkfile(t, victim)
	mkfile(t, bystander)

	if err := os.Remove(victim); err != nil {
		t.Fatal(err)
	}
	pruneEmptyDirs(root, filepath.Dir(victim))

	if _, err := os.Stat(bystander); err != nil {
		t.Errorf("a co-located item's asset was destroyed: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(bystander)); err != nil {
		t.Errorf("shared arch dir removed while still occupied: %v", err)
	}
}

// TestPruneEmptyDirs_CollapsesWhenEmpty: once the last item leaves, the arch AND
// version folders are GC'd — but never the marketplace root itself.
func TestPruneEmptyDirs_CollapsesWhenEmpty(t *testing.T) {
	root := t.TempDir()
	only := filepath.Join(root, "1.0.0", "windows-amd64", "ktw-hub.exe")
	mkfile(t, only)

	if err := os.Remove(only); err != nil {
		t.Fatal(err)
	}
	pruneEmptyDirs(root, filepath.Dir(only))

	if _, err := os.Stat(filepath.Join(root, "1.0.0", "windows-amd64")); !os.IsNotExist(err) {
		t.Errorf("empty arch dir should be GC'd, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "1.0.0")); !os.IsNotExist(err) {
		t.Errorf("empty version dir should be GC'd, got err=%v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Errorf("the marketplace root must never be removed: %v", err)
	}
}

// TestPruneEmptyDirs_KeepsOtherVersions: pruning one version leaves the others.
func TestPruneEmptyDirs_KeepsOtherVersions(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "1.0.0", "linux-amd64", "ktw-hub")
	cur := filepath.Join(root, "2.0.0", "linux-amd64", "ktw-hub")
	mkfile(t, old)
	mkfile(t, cur)

	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	pruneEmptyDirs(root, filepath.Dir(old))

	if _, err := os.Stat(filepath.Join(root, "1.0.0")); !os.IsNotExist(err) {
		t.Errorf("pruned version dir should be gone, got err=%v", err)
	}
	if _, err := os.Stat(cur); err != nil {
		t.Errorf("a retained version was destroyed: %v", err)
	}
}

func TestIsAbsoluteURL(t *testing.T) {
	for _, u := range []string{"https://cdn.example/x/ktw-hub", "file://D:/m/ktw-hub"} {
		if !isAbsoluteURL(u) {
			t.Errorf("isAbsoluteURL(%q) = false, want true", u)
		}
	}
	for _, u := range []string{"1.0.0/windows-amd64/ktw-hub.exe", "ktw-hub"} {
		if isAbsoluteURL(u) {
			t.Errorf("isAbsoluteURL(%q) = true, want false", u)
		}
	}
}
