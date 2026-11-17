// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Agent memory is kept behind a wall: one folder per tenant
// universe and person, under the data root (outside every working tree), so a
// run's prompt joins only the memory of its own tenant, universe and person.
//
//	<root>/<tenant>/<universe>/<person>/MEMORY.md   — joined into the prompt
//	<root>/_quarantine/<stamp>-<source>/…           — memory written before the wall
//
// The root is <TenantDataDir>/agent-memory. The tenant and the universe are
// path segments of their own, not only the data root, so a data root shared
// by two universes (a KTW_DATA override) still keeps them apart.

const memoryEntrypoint = "MEMORY.md"

// quarantineDir is the folder, under the memory root, holding the memory
// written before the wall: never joined into a prompt, kept until the person
// assigns it to a tenant and universe or deletes it (quarantine.go).
const quarantineDir = "_quarantine"

// MemoryKey is whose memory a run reads: its tenant, its universe and its
// person. A key missing any of them reads no memory.
type MemoryKey struct {
	Tenant   string
	Universe string
	Person   string
}

// complete reports a key naming its tenant, universe and person.
func (k MemoryKey) complete() bool {
	return k.Tenant != "" && k.Universe != "" && k.Person != ""
}

// MemoryManager reads and writes the agent memory of one key.
type MemoryManager struct {
	root string // <TenantDataDir>/agent-memory; "" = no memory
	key  MemoryKey
}

// NewMemoryManager is the memory of key under root.
func NewMemoryManager(root string, key MemoryKey) *MemoryManager {
	return &MemoryManager{root: root, key: key}
}

// segment makes one key part a safe single path segment: a part that would
// climb out of its folder or split into several is refused ("").
var unsafeSegment = regexp.MustCompile(`[^A-Za-z0-9._@+-]`)

func segment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." || strings.HasPrefix(s, "_") {
		return ""
	}
	return unsafeSegment.ReplaceAllString(s, "_")
}

// MemoryDir is the folder of the key's memory, "" when the manager has no
// root or the key does not name a tenant, a universe and a person.
func (m *MemoryManager) MemoryDir() string {
	if m == nil || m.root == "" || !m.key.complete() {
		return ""
	}
	t, u, p := segment(m.key.Tenant), segment(m.key.Universe), segment(m.key.Person)
	if t == "" || u == "" || p == "" {
		return ""
	}
	return filepath.Join(m.root, t, u, p)
}

// LoadMemoryPrompt is the key's memory as a prompt section, "" when it has
// none.
func (m *MemoryManager) LoadMemoryPrompt() string {
	dir := m.MemoryDir()
	if dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, memoryEntrypoint))
	if err != nil || len(data) == 0 {
		return ""
	}
	return fmt.Sprintf("## Agent Memory\n\n%s", data)
}

// SaveMemory writes the key's memory, readable by its owner only.
func (m *MemoryManager) SaveMemory(content string) error {
	dir := m.MemoryDir()
	if dir == "" {
		return fmt.Errorf("agent memory: no tenant, universe and person to keep it for")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	return os.WriteFile(filepath.Join(dir, memoryEntrypoint), []byte(content), 0o600)
}

// ListMemoryFiles lists the files of the key's memory folder.
func (m *MemoryManager) ListMemoryFiles() ([]string, error) {
	dir := m.MemoryDir()
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// LegacyMemoryDirs are the folders memory was written to before the wall:
// the home folder's (joined into every prompt whatever the tenant) and the
// working folder's two (inside a checkout).
func LegacyMemoryDirs(homeDir, workDir string) []string {
	var dirs []string
	if homeDir != "" {
		dirs = append(dirs, filepath.Join(homeDir, ".ktw", "agent-memory"))
	}
	if workDir != "" {
		dirs = append(dirs,
			filepath.Join(workDir, ".agent-memory"),
			filepath.Join(workDir, ".agent-memory-local"))
	}
	return dirs
}

// QuarantineLegacyMemory moves each legacy folder that exists into the memory
// root's quarantine, where no prompt reads it. It answers the folders it
// moved to. A folder it cannot move is left where it is - it is never read
// any more either way.
func QuarantineLegacyMemory(root string, legacy []string, now time.Time) ([]string, error) {
	if root == "" {
		return nil, nil
	}
	var moved []string
	var errs []string
	for _, src := range legacy {
		info, err := os.Stat(src)
		if err != nil || !info.IsDir() {
			continue
		}
		name := now.UTC().Format("20060102T150405Z") + "-" + unsafeSegment.ReplaceAllString(src, "_")
		dst := filepath.Join(root, quarantineDir, name)
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if err := moveDir(src, dst); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", src, err))
			continue
		}
		// Where it came from and when, for the card that asks the person
		// to keep it or delete it; a folder without one is named by its name.
		_ = writeQuarantineMarker(dst, quarantineMarker{Source: src, At: now.UTC().Format(time.RFC3339)})
		moved = append(moved, dst)
	}
	if len(errs) > 0 {
		return moved, fmt.Errorf("quarantine agent memory: %s", strings.Join(errs, "; "))
	}
	return moved, nil
}

// moveDir renames src to dst, copying then removing when they sit on two
// volumes (a home folder and a data root on different drives).
func moveDir(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o600)
	}); err != nil {
		return err
	}
	return os.RemoveAll(src)
}
