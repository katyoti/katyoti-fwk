// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// R2 — symlink-aware path resolution in pathAllowed. Closes the second
// half of audit threat-T2 / high-finding-1: a symlink inside the workspace
// pointing OUTSIDE the allowed prefix used to silently allow access. The
// new EvalSymlinks fail-closed step rejects it.
//
// Skipped on Windows: symlinks there need admin or developer mode + the
// resolution semantics differ enough that a separate test is warranted
// when the team has a Windows CI runner. POSIX-style symlinks cover the
// audit's attack scenario.

func TestPathAllowed_SymlinkPointsOutsideWorkspace_Rejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need admin/developer mode on Windows")
	}

	workspace := t.TempDir()
	outside := t.TempDir() // separate dir, NOT inside workspace

	// Plant the escape: workspace/escape → outside.
	escape := filepath.Join(workspace, "escape")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	// Create a real file at the symlink target so EvalSymlinks succeeds.
	hosts := filepath.Join(outside, "hosts")
	if err := os.WriteFile(hosts, []byte("attacker-content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	allowed := []string{workspace}

	// Direct path inside the workspace prefix BUT the symlink lands outside.
	// Textual match succeeds (escape starts with workspace) — fail-closed
	// must catch the symlink escape.
	input := filepath.Join(workspace, "escape", "hosts")
	if pathAllowed(input, allowed) {
		t.Errorf("pathAllowed should deny symlink escape: %s → %s", input, hosts)
	}
}

func TestPathAllowed_SymlinkWithinWorkspace_Allowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need admin/developer mode on Windows")
	}

	workspace := t.TempDir()
	// Create a real subdirectory inside workspace and a symlink to it.
	realDir := filepath.Join(workspace, "real")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	hosts := filepath.Join(realDir, "file.txt")
	if err := os.WriteFile(hosts, []byte("ok"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Internal symlink: workspace/alias → workspace/real.
	alias := filepath.Join(workspace, "alias")
	if err := os.Symlink(realDir, alias); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	allowed := []string{workspace}
	input := filepath.Join(workspace, "alias", "file.txt")
	if !pathAllowed(input, allowed) {
		t.Errorf("pathAllowed should allow symlink within workspace: %s → %s", input, hosts)
	}
}

func TestPathAllowed_NonExistentPath_FallsBackToParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests need admin/developer mode on Windows")
	}

	workspace := t.TempDir()
	allowed := []string{workspace}

	// Path doesn't exist yet (file_write target). Parent dir exists (it's
	// the workspace tmpdir) and is the same as the allowed prefix → allow.
	input := filepath.Join(workspace, "new-file.txt")
	if !pathAllowed(input, allowed) {
		t.Errorf("pathAllowed should allow new file inside workspace via parent resolution: %s", input)
	}

	// Also: a path whose PARENT is a symlink-out should be rejected.
	outside := t.TempDir()
	escape := filepath.Join(workspace, "escape-dir")
	if err := os.Symlink(outside, escape); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	escaped := filepath.Join(escape, "new-file.txt")
	if pathAllowed(escaped, allowed) {
		t.Errorf("pathAllowed should deny new file whose parent symlinks out: %s", escaped)
	}
}
