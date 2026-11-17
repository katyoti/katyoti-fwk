// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
)

// SidecarsYaml is the on-disk shape of ~/ktw-agents/sidecars.yaml — the global
// catalog of sidecars discovered on the system. Populated by ScanInventory;
// read by the universe-creation flow to drive the sidecar selection wizard.
//
// This catalog answers "what is INSTALLED on this machine". It is NOT the
// per-universe membership (the `sidecars:` block of <home>/config/Univers.yaml).
type SidecarsYaml struct {
	Version  int                 `yaml:"version,omitempty"`
	Sidecars []SidecarsYamlEntry `yaml:"sidecars"`
}

// SidecarsYamlEntry is one row in sidecars.yaml.
type SidecarsYamlEntry struct {
	Name         string    `yaml:"name"`
	BinaryPath   string    `yaml:"binary_path"`
	Version      string    `yaml:"version,omitempty"`
	Capabilities []string  `yaml:"capabilities,omitempty"`
	Source       string    `yaml:"source,omitempty"` // PATH, project-local, configured, marketplace
	LastSeen     time.Time `yaml:"last_seen,omitempty"`

	// Marketplace-installed items carry provenance + kind (katyoti). A
	// files-only "content" item is a folder run on ktw-host, so it records a
	// FolderPath instead of a PATH binary. All omitempty — a plain PATH scan
	// leaves them blank.
	Kind        string `yaml:"kind,omitempty"`        // sidecar | content
	FolderPath  string `yaml:"folder_path,omitempty"` // content items: ktw-host --folder target
	Marketplace string `yaml:"marketplace,omitempty"` // source marketplace name
	Sha256      string `yaml:"sha256,omitempty"`
	Signature   string `yaml:"signature,omitempty"`
}

// inventoryMu serializes concurrent reads/writes of sidecars.yaml.
var inventoryMu sync.Mutex

// DefaultSidecarsPath returns the cross-universe sidecars.yaml path —
// `<GlobalHomeDir>/sidecars.yaml` (the katyoti global root).
func DefaultSidecarsPath() (string, error) {
	root := application.Env().GlobalHomeDir()
	if root == "" {
		return "", fmt.Errorf("resolve global agents root")
	}
	return filepath.Join(root, "sidecars.yaml"), nil
}

// InstalledBinary is the binary a marketplace installed for the sidecar name -
// its managed row in `<GlobalHomeDir>/sidecars.yaml` whose file exists - or ""
// when there is none. In an installation each binary lives in its own store
// folder, never beside another program: a program that runs another one by
// name asks here after looking beside itself.
func InstalledBinary(name string) string {
	path, err := DefaultSidecarsPath()
	if err != nil {
		return ""
	}
	return installedBinaryIn(path, name)
}

// installedBinaryIn is InstalledBinary read from the registry at path.
func installedBinaryIn(path, name string) string {
	inv, err := LoadSidecarsYaml(path)
	if err != nil || inv == nil {
		return ""
	}
	for _, e := range inv.Sidecars {
		if e.Name != name || e.BinaryPath == "" || !IsManagedSource(e.Source) {
			continue
		}
		if _, err := os.Stat(e.BinaryPath); err == nil {
			return e.BinaryPath
		}
	}
	return ""
}

// LoadSidecarsYaml reads sidecars.yaml. Missing file returns an empty struct
// with no error (first-run case). An entry whose program or folder is gone is
// left out, the log saying so: it is never listed nor
// checked, and the next write of the registry no longer holds it
// (PruneVanishedSidecars writes it at once).
func LoadSidecarsYaml(path string) (*SidecarsYaml, error) {
	s, gone, err := loadSidecarsYaml(path)
	if err != nil {
		return nil, err
	}
	logVanished(path, gone)
	return s, nil
}

// VanishedSidecar is whether a registry entry's files are gone: its binary
// (a compiled sidecar) or its folder (a content item) - an absolute path that
// no longer exists. A bare name found on PATH is never vanished.
func VanishedSidecar(e SidecarsYamlEntry) bool {
	return cenv.Vanished(e.BinaryPath) || cenv.Vanished(e.FolderPath)
}

// PruneVanishedSidecars removes from the registry at path every entry whose
// files are gone and writes it - only for the installation it belongs to
// (as PersistInventory), so a program of no installation never writes it.
// Returns the entries removed (or that would be, when not written).
func PruneVanishedSidecars(path string) ([]SidecarsYamlEntry, error) {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()
	s, gone, err := loadSidecarsYaml(path)
	if err != nil || len(gone) == 0 {
		return gone, err
	}
	logVanished(path, gone)
	if owner := cenv.RootInstallation(filepath.Dir(path)); owner != nil && !owner.Is(cenv.CurrentInstallation()) {
		return gone, nil
	}
	return gone, SaveSidecarsYaml(path, s)
}

// loggedVanished keeps each forgotten entry said once per process: a
// registry read often is not a log written often.
var loggedVanished sync.Map

func logVanished(path string, gone []SidecarsYamlEntry) {
	for _, e := range gone {
		if _, said := loggedVanished.LoadOrStore(path+"|"+e.Name+"|"+e.BinaryPath+"|"+e.FolderPath, true); said {
			continue
		}
		slog.Info("sidecars.yaml: an installed item whose files are gone is forgotten", "path", path, "name", e.Name, "version", e.Version, "binary", e.BinaryPath, "folder", e.FolderPath)
	}
}

// loadSidecarsYaml reads sidecars.yaml, the entries whose files are gone
// apart.
func loadSidecarsYaml(path string) (*SidecarsYaml, []SidecarsYamlEntry, error) {
	s, err := readSidecarsYaml(path)
	if err != nil {
		return nil, nil, err
	}
	var gone []SidecarsYamlEntry
	kept := s.Sidecars[:0]
	for _, e := range s.Sidecars {
		if VanishedSidecar(e) {
			gone = append(gone, e)
			continue
		}
		kept = append(kept, e)
	}
	s.Sidecars = kept
	return s, gone, nil
}

// readSidecarsYaml reads sidecars.yaml as it is written.
func readSidecarsYaml(path string) (*SidecarsYaml, error) {
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &SidecarsYaml{Version: 1}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read sidecars.yaml at %s: %w", path, err)
	}
	var s SidecarsYaml
	if err := yaml.Unmarshal(bytes, &s); err != nil {
		return nil, fmt.Errorf("decode sidecars.yaml at %s: %w", path, err)
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return &s, nil
}

// SaveSidecarsYaml writes sidecars.yaml atomically (write-temp + rename).
// Creates parent directory if missing.
func SaveSidecarsYaml(path string, s *SidecarsYaml) error {
	if s.Version == 0 {
		s.Version = 1
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create sidecars.yaml dir: %w", err)
	}
	bytes, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode sidecars.yaml: %w", err)
	}
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		return fmt.Errorf("write sidecars.yaml at %s: %w", path, err)
	}
	return nil
}

// MergeInventory merges a fresh scan into the existing sidecars.yaml.
//
// Strategy:
//   - Entries present in the fresh scan: replace existing entry; bump LastSeen
//   - Entries in existing but not in fresh: keep, do NOT bump LastSeen
//     (the binary may simply be off-PATH right now; we don't want to lose
//     the record on a transient PATH change).
//   - Returns the merged SidecarsYaml + the list of newly-discovered names
//     (entries that were not in the previous catalog).
//
// The merge sorts the final list by name for stable on-disk output.
func MergeInventory(existing *SidecarsYaml, fresh []SidecarsYamlEntry) (*SidecarsYaml, []string) {
	now := time.Now().UTC()
	byName := make(map[string]SidecarsYamlEntry, len(existing.Sidecars))
	for _, e := range existing.Sidecars {
		byName[e.Name] = e
	}
	var newlyDiscovered []string
	for _, f := range fresh {
		f.LastSeen = now
		if existing, existed := byName[f.Name]; existed {
			// A live scan must NEVER demote a MANAGED row to a bare scan row
			// (losing version/sha256/marketplace provenance). This matches the
			// read-side rule in available.List ("recorded wins"); only refresh
			// LastSeen on the preserved row.
			//
			// "Managed" is decided negatively — anything that is not a scan
			// source — rather than by listing managed sources. The rule used to
			// match "marketplace" only, so the control plane's "infra-channel"
			// rows (install-infra records katyoti + ktw-hub + ktw-gui +
			// ktw-univers) were demoted by the very scan the GUI triggers: it
			// runs inside ktw-univers, whose own folder IS the control plane.
			// Every guard keyed on that source then failed — the Marketplace tab
			// listed ktw-hub/ktw-univers as discovered dev sidecars, `status`
			// counted them as unmanaged, doctor lost its never-exec-infra rule.
			if IsManagedSource(existing.Source) {
				existing.LastSeen = now
				byName[f.Name] = existing
				continue
			}
		} else {
			newlyDiscovered = append(newlyDiscovered, f.Name)
		}
		byName[f.Name] = f
	}
	merged := &SidecarsYaml{Version: existing.Version}
	if merged.Version == 0 {
		merged.Version = 1
	}
	for _, e := range byName {
		merged.Sidecars = append(merged.Sidecars, e)
	}
	sort.Slice(merged.Sidecars, func(i, j int) bool {
		return merged.Sidecars[i].Name < merged.Sidecars[j].Name
	})
	sort.Strings(newlyDiscovered)
	return merged, newlyDiscovered
}

// PersistInventory loads the existing sidecars.yaml at path, merges in the
// fresh scan results, and writes back atomically. Returns the merged catalog
// and the list of newly-discovered names.
//
// Thread-safe via inventoryMu — concurrent ScanInventory calls in the same
// process serialize on this mutex.
func PersistInventory(path string, fresh []SidecarsYamlEntry) (*SidecarsYaml, []string, error) {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()
	existing, err := LoadSidecarsYaml(path)
	if err != nil {
		return nil, nil, err
	}
	merged, newly := MergeInventory(existing, fresh)
	if owner := cenv.RootInstallation(filepath.Dir(path)); owner != nil {
		// The registry of an installation lists the installation's programs: a
		// program of no installation (a checkout's bin/, a test) or of another
		// one does not record what it discovered there, and a scan of the
		// installation's own drops the discovered rows outside it.
		if !owner.Is(cenv.CurrentInstallation()) {
			return merged, newly, nil
		}
		merged.Sidecars = KeepScanRowsWithin(merged.Sidecars, owner.Dir())
	}
	if err := SaveSidecarsYaml(path, merged); err != nil {
		return nil, nil, err
	}
	return merged, newly, nil
}

// KeepScanRowsWithin drops the discovered rows (IsScanSource, or a legacy row
// with no source) whose binary lies outside dir; an installed row is kept.
func KeepScanRowsWithin(rows []SidecarsYamlEntry, dir string) []SidecarsYamlEntry {
	kept := make([]SidecarsYamlEntry, 0, len(rows))
	for _, r := range rows {
		if !IsManagedSource(r.Source) && r.BinaryPath != "" && !cenv.Within(r.BinaryPath, dir) {
			continue
		}
		kept = append(kept, r)
	}
	return kept
}

// UpsertSidecarEntry inserts or replaces one entry by name. It serializes through
// inventoryMu — the SINGLE sidecars.yaml write lock shared with PersistInventory —
// so the marketplace-install writer and the inventory-scan writer never race on
// the file. (The marketplace `available` package routes its writes here instead of
// its own load/modify/save.)
func UpsertSidecarEntry(path string, e SidecarsYamlEntry) error {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()
	sy, err := LoadSidecarsYaml(path)
	if err != nil {
		return err
	}
	for i := range sy.Sidecars {
		if sy.Sidecars[i].Name == e.Name {
			sy.Sidecars[i] = e
			return SaveSidecarsYaml(path, sy)
		}
	}
	sy.Sidecars = append(sy.Sidecars, e)
	return SaveSidecarsYaml(path, sy)
}

// RemoveSidecarEntry drops the entry named `name` whose Source matches `source`
// (empty source = match any). Serialized through inventoryMu. Returns whether an
// entry was removed.
func RemoveSidecarEntry(path, name, source string) (bool, error) {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()
	sy, err := LoadSidecarsYaml(path)
	if err != nil {
		return false, err
	}
	kept := sy.Sidecars[:0]
	removed := false
	for _, e := range sy.Sidecars {
		if e.Name == name && (source == "" || e.Source == source) {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	sy.Sidecars = kept
	if !removed {
		return false, nil
	}
	return true, SaveSidecarsYaml(path, sy)
}
