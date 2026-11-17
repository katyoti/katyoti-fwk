// SPDX-License-Identifier: Apache-2.0

package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorktreeCleanup_PreservesDirty (UC-093) — when a worktree has
// uncommitted changes, RemoveWorktree(slug, force=false) refuses to delete
// it. This is the cleanup-on-completion contract: if the child left work
// in progress, the worktree must be preserved for user review.
//
// Skips automatically if git isn't on PATH (CI may lack it). Otherwise
// builds a real temp repo, creates a worktree, dirties it, and verifies
// the cleanup path returns the expected error AND the worktree dir still
// exists.
func TestWorktreeCleanup_PreservesDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available — skipping worktree cleanup test")
	}

	repoDir := t.TempDir()
	runGit(t, repoDir, "init", "--initial-branch=main", "-q")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	runGit(t, repoDir, "config", "user.name", "test")
	runGit(t, repoDir, "config", "commit.gpgsign", "false")

	// Seed commit (worktree add requires a HEAD).
	seedPath := filepath.Join(repoDir, "seed.txt")
	if err := os.WriteFile(seedPath, []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGit(t, repoDir, "add", "seed.txt")
	runGit(t, repoDir, "commit", "-q", "-m", "seed")

	m := NewManager(repoDir, Config{
		BaseDir: filepath.Join(repoDir, ".worktrees"),
	})

	state, err := m.CreateWorktree(KindIsolated, "uc093-dirty", "test-agent")
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}

	// Inject uncommitted change so isDirty returns true.
	dirtyFile := filepath.Join(state.Path, "uncommitted.txt")
	if err := os.WriteFile(dirtyFile, []byte("WIP\n"), 0o644); err != nil {
		t.Fatalf("dirty file: %v", err)
	}

	// RemoveWorktree(force=false) must refuse with a clear error.
	err = m.RemoveWorktree(state.Slug, false)
	if err == nil {
		t.Fatal("UC-093: RemoveWorktree on dirty worktree should fail (force=false)")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("UC-093: error %q should mention uncommitted changes", err.Error())
	}

	// Worktree dir is still on disk — confirming the preservation guarantee.
	if _, statErr := os.Stat(state.Path); errors.Is(statErr, os.ErrNotExist) {
		t.Error("UC-093: worktree dir was deleted despite force=false + dirty state")
	}

	// force=true cleans up so the test leaves the temp dir tidy.
	_ = m.RemoveWorktree(state.Slug, true)
}

// runGit invokes git in the given dir; fatals on error to keep test bodies clean.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, string(out))
	}
}
