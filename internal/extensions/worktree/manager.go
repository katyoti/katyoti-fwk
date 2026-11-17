// SPDX-License-Identifier: Apache-2.0

// Package worktree provides git worktree isolation for sub-agents.
// Each worktree gets a dedicated git checkout with a separate branch,
// enabling parallel file modifications without conflicts.
package worktree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Worktree kinds — used as the slug discriminator. Add new kinds here when
// new isolation flavors arrive; ValidateSlug enforces the format.
const (
	KindIsolated = "iso"
	KindFork     = "fork"
)

// WorktreeState tracks a worktree's lifecycle.
type WorktreeState struct {
	Slug        string `json:"slug"`
	Kind        string `json:"kind,omitempty"`
	DisplayName string `json:"displayName,omitempty"` // human-readable label (e.g. task name); never used in filesystem paths or git refs
	Path        string `json:"path"`
	BranchName  string `json:"branchName"`
	OriginalCwd string `json:"originalCwd,omitempty"`
	CreatedAt   int64  `json:"createdAt"`
	AgentID     string `json:"agentId,omitempty"`
}

// WorktreeResult reports what happened in a worktree after the agent finished.
type WorktreeResult struct {
	Path         string      `json:"path"`
	BranchName   string      `json:"branchName"`
	HasChanges   bool        `json:"hasChanges"`
	ChangedFiles []string    `json:"changedFiles,omitempty"`
	DiffStats    DiffSummary `json:"diffStats"`
}

// DiffSummary holds git diff statistics.
type DiffSummary struct {
	FilesChanged int `json:"filesChanged"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// Config for WorktreeManager.
type Config struct {
	BaseDir             string   `json:"baseDir"`
	SymlinkDirs         []string `json:"symlinkDirs,omitempty"`
	StaleThresholdHours int      `json:"staleThresholdHours"`
}

// Manager manages git worktrees.
type Manager struct {
	cfg     Config
	repoDir string // root of the git repo
}

// slugPattern enforces the hash-derived slug format `wt-<kind>-<8-hex>-<seq>`.
// LLM-supplied input never reaches the slug — only the DisplayName field, which
// is never written to a filesystem path or git ref (R8 / audit high-finding-4).
var slugPattern = regexp.MustCompile(`^wt-[a-z]{2,8}-[a-f0-9]{8}-\d+$`)

// slugSeq monotonically increments to disambiguate worktrees created within
// the same nanosecond. Cheap, unique, no global lock.
var slugSeq atomic.Uint64

// NewManager creates a WorktreeManager.
// repoDir is the root of the git repository.
func NewManager(repoDir string, cfg Config) *Manager {
	if cfg.BaseDir == "" {
		cfg.BaseDir = filepath.Join(repoDir, ".worktrees")
	}
	if cfg.StaleThresholdHours == 0 {
		cfg.StaleThresholdHours = 24
	}
	return &Manager{cfg: cfg, repoDir: repoDir}
}

// ValidateSlug checks that a slug is safe for use as a directory name and a
// git ref. Only the hash-derived format produced by deriveSlug() is accepted —
// this is defence-in-depth so LLM-supplied input cannot reach git argv even
// if a future caller bypasses CreateWorktree (R8 / high-finding-4).
func (m *Manager) ValidateSlug(slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: must match wt-<kind>-<8-hex>-<seq> (slug is internally generated; do not pass arbitrary strings)", slug)
	}
	if strings.Contains(slug, "..") {
		return fmt.Errorf("invalid slug %q: path traversal not allowed", slug)
	}
	return nil
}

// deriveSlug builds the canonical worktree slug from inputs that include
// neither the LLM-supplied DisplayName nor any caller-controlled string.
// Hash inputs: kind + agentID + nanosecond timestamp + monotonic counter.
// Result format: `wt-<kind>-<8-hex>-<seq>`.
func deriveSlug(kind, agentID string) string {
	seq := slugSeq.Add(1)
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%d", kind, agentID, time.Now().UnixNano(), seq)
	hash := hex.EncodeToString(h.Sum(nil))[:8]
	return fmt.Sprintf("wt-%s-%s-%d", kind, hash, seq)
}

// CreateWorktree creates a new git worktree with a unique hash-derived slug.
// kind is one of the package constants (KindIsolated, KindFork, ...) and
// controls the slug discriminator. displayName is a human-readable label
// (e.g. an LLM-supplied task name) — preserved on WorktreeState.DisplayName
// for UI but never used in any filesystem path or git ref.
func (m *Manager) CreateWorktree(kind, displayName, agentID string) (*WorktreeState, error) {
	if kind == "" {
		return nil, fmt.Errorf("create worktree: kind is required")
	}
	slug := deriveSlug(kind, agentID)
	if err := m.ValidateSlug(slug); err != nil {
		// Should be impossible — deriveSlug always produces a valid format.
		return nil, fmt.Errorf("create worktree: derived slug rejected by validator: %w", err)
	}

	worktreePath := filepath.Join(m.cfg.BaseDir, slug)
	branchName := fmt.Sprintf("worktree/%s", slug)

	// Ensure base dir exists
	if err := os.MkdirAll(m.cfg.BaseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create base dir: %w", err)
	}

	// Check if worktree already exists
	if _, err := os.Stat(worktreePath); err == nil {
		return nil, fmt.Errorf("worktree %q already exists at %s", slug, worktreePath)
	}

	// Create git worktree with new branch
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath)
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git worktree add: %s: %w", string(out), err)
	}

	// Symlink shared directories
	for _, dir := range m.cfg.SymlinkDirs {
		src := filepath.Join(m.repoDir, dir)
		dst := filepath.Join(worktreePath, dir)
		if _, err := os.Stat(src); err == nil {
			_ = os.Symlink(src, dst)
		}
	}

	state := &WorktreeState{
		Slug:        slug,
		Kind:        kind,
		DisplayName: displayName,
		Path:        worktreePath,
		BranchName:  branchName,
		CreatedAt:   time.Now().Unix(),
		AgentID:     agentID,
	}

	// Persist state
	_ = m.SaveState(state)

	return state, nil
}

// RemoveWorktree removes a worktree. If force is false, blocks on dirty worktrees.
func (m *Manager) RemoveWorktree(slug string, force bool) error {
	if err := m.ValidateSlug(slug); err != nil {
		return err
	}

	worktreePath := filepath.Join(m.cfg.BaseDir, slug)
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return fmt.Errorf("worktree %q not found", slug)
	}

	// Check for uncommitted changes (unless force)
	if !force {
		dirty, err := m.isDirty(worktreePath)
		if err != nil {
			return fmt.Errorf("check dirty: %w", err)
		}
		if dirty {
			return fmt.Errorf("cannot remove worktree %q: uncommitted changes exist (use force=true to override)", slug)
		}
	}

	// Remove symlinks first (before git worktree remove)
	for _, dir := range m.cfg.SymlinkDirs {
		link := filepath.Join(worktreePath, dir)
		if info, err := os.Lstat(link); err == nil && info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(link)
		}
	}

	// Git worktree remove
	cmd := exec.Command("git", "worktree", "remove", worktreePath, "--force")
	cmd.Dir = m.repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		// Fallback: manual remove if git command fails
		_ = os.RemoveAll(worktreePath)
		_ = m.pruneWorktrees()
		if _, statErr := os.Stat(worktreePath); statErr == nil {
			return fmt.Errorf("git worktree remove: %s: %w", string(out), err)
		}
	}

	// Delete branch (best effort)
	branchName := fmt.Sprintf("worktree/%s", slug)
	delCmd := exec.Command("git", "branch", "-D", branchName)
	delCmd.Dir = m.repoDir
	_ = delCmd.Run()

	// Remove state file
	_ = os.Remove(m.stateFile(slug))

	return nil
}

// GetWorktreeResult reports changes in a worktree.
func (m *Manager) GetWorktreeResult(slug string) (*WorktreeResult, error) {
	state, err := m.LoadState(slug)
	if err != nil {
		return nil, err
	}

	result := &WorktreeResult{
		Path:       state.Path,
		BranchName: state.BranchName,
	}

	// Check for changes
	changedFiles, err := m.changedFiles(state.Path)
	if err != nil {
		return result, nil // Return partial result
	}

	result.HasChanges = len(changedFiles) > 0
	result.ChangedFiles = changedFiles

	// Get diff stats
	result.DiffStats, _ = m.diffStats(state.Path)

	return result, nil
}

// CleanupStaleWorktrees removes worktrees older than the configured threshold.
func (m *Manager) CleanupStaleWorktrees() (int, error) {
	if _, err := os.Stat(m.cfg.BaseDir); os.IsNotExist(err) {
		return 0, nil
	}

	entries, err := os.ReadDir(m.cfg.BaseDir)
	if err != nil {
		return 0, err
	}

	threshold := time.Now().Add(-time.Duration(m.cfg.StaleThresholdHours) * time.Hour)
	removed := 0

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(threshold) {
			if err := m.RemoveWorktree(entry.Name(), true); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}

// SaveState persists worktree state as JSON.
func (m *Manager) SaveState(state *WorktreeState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(m.stateFile(state.Slug), data, 0o644)
}

// LoadState reads worktree state from disk.
func (m *Manager) LoadState(slug string) (*WorktreeState, error) {
	data, err := os.ReadFile(m.stateFile(slug))
	if err != nil {
		return nil, fmt.Errorf("worktree %q: state not found", slug)
	}
	var state WorktreeState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (m *Manager) stateFile(slug string) string {
	return filepath.Join(m.cfg.BaseDir, "."+slug+".json")
}

func (m *Manager) isDirty(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return false, err
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

func (m *Manager) changedFiles(worktreePath string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, line := range lines {
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

func (m *Manager) diffStats(worktreePath string) (DiffSummary, error) {
	cmd := exec.Command("git", "diff", "--stat", "HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return DiffSummary{}, err
	}

	// Parse last line: " N files changed, M insertions(+), K deletions(-)"
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return DiffSummary{}, nil
	}

	summary := lines[len(lines)-1]
	ds := DiffSummary{}

	if parts := regexp.MustCompile(`(\d+) files? changed`).FindStringSubmatch(summary); len(parts) > 1 {
		ds.FilesChanged, _ = strconv.Atoi(parts[1])
	}
	if parts := regexp.MustCompile(`(\d+) insertions?`).FindStringSubmatch(summary); len(parts) > 1 {
		ds.Insertions, _ = strconv.Atoi(parts[1])
	}
	if parts := regexp.MustCompile(`(\d+) deletions?`).FindStringSubmatch(summary); len(parts) > 1 {
		ds.Deletions, _ = strconv.Atoi(parts[1])
	}

	return ds, nil
}

func (m *Manager) pruneWorktrees() error {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = m.repoDir
	return cmd.Run()
}
