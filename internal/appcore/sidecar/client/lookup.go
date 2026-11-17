// SPDX-License-Identifier: Apache-2.0

package client

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// SidecarLookup discovers sidecar binaries available on the system.
// Different implementations support different discovery strategies.
type SidecarLookup interface {
	// Lookup returns all discovered sidecar binaries as name→path pairs.
	// Only binaries that actually exist on disk are returned.
	Lookup() []SidecarBinary
}

// --- Implementation 1: scan alongside the current binary ---

// NeighborLookup discovers sidecars by scanning the directory of a reference
// binary for other ktw-* executables. This is useful when sidecars are
// installed together in the same directory (e.g., bin/) but the exact set
// isn't known at compile time.
type NeighborLookup struct {
	// ReferenceBinary is the path to the binary whose directory to scan.
	// Typically os.Executable() — the running binary itself.
	ReferenceBinary string
}

// NewNeighborLookup creates a lookup that scans alongside the current executable.
func NewNeighborLookup() (*NeighborLookup, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, err
	}
	return &NeighborLookup{ReferenceBinary: exe}, nil
}

// Lookup scans the reference binary's directory for ktw-* executables,
// excluding well-known non-sidecar binaries.
func (l *NeighborLookup) Lookup() []SidecarBinary {
	dir := filepath.Dir(l.ReferenceBinary)
	self := filepath.Base(l.ReferenceBinary)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var found []SidecarBinary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Skip self
		if name == self {
			continue
		}

		// Must start with "ktw-"
		baseName := stripExeSuffix(name)
		if !strings.HasPrefix(baseName, "ktw-") {
			continue
		}

		// Skip well-known non-sidecar binaries
		if isExcluded(baseName) {
			continue
		}

		// Must be executable
		fullPath := filepath.Join(dir, name)
		if !isExecutable(fullPath) {
			continue
		}

		found = append(found, SidecarBinary{
			Name: baseName,
			Path: fullPath,
		})
	}

	sort.Slice(found, func(i, j int) bool {
		return found[i].Name < found[j].Name
	})
	return found
}

// --- Implementation 2: composite (merge multiple lookups) ---

// CompositeLookup merges results from multiple lookups. Later lookups
// override earlier ones (by name). This allows combining neighbor scan
// with explicit additions.
type CompositeLookup struct {
	Lookups []SidecarLookup
}

// NewCompositeLookup creates a lookup that merges multiple sources.
func NewCompositeLookup(lookups ...SidecarLookup) *CompositeLookup {
	return &CompositeLookup{Lookups: lookups}
}

// Lookup merges results from all sub-lookups, deduplicating by name
// (last wins).
func (c *CompositeLookup) Lookup() []SidecarBinary {
	seen := make(map[string]SidecarBinary)
	var order []string

	for _, l := range c.Lookups {
		for _, sb := range l.Lookup() {
			if _, exists := seen[sb.Name]; !exists {
				order = append(order, sb.Name)
			}
			seen[sb.Name] = sb
		}
	}

	result := make([]SidecarBinary, 0, len(order))
	for _, name := range order {
		result = append(result, seen[name])
	}
	return result
}

// --- Types ---

// SidecarBinary describes a discovered sidecar binary on the system.
type SidecarBinary struct {
	Name string // binary name without extension (e.g., "ktw-docs")
	Path string // resolved filesystem path
}

// --- Helpers ---

// excludedBinaries are ktw-* binaries that are NOT sidecars.
// They don't expose the sidecar contract (no ConfigService, no Info).
var excludedBinaries = map[string]bool{
	"ktw-agents": true, // the orchestrator, not a sidecar
	"ktw-utils":  true, // CLI utilities (tui, session inspection)
	"ktw-gui":    true, // desktop GUI client
	"ktw-forms":  true, // Wails forms app
}

func isExcluded(name string) bool {
	return excludedBinaries[name]
}

func stripExeSuffix(name string) string {
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(name, ".exe")
	}
	return name
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		// On Windows, any file is "executable" — check for .exe suffix
		return strings.HasSuffix(strings.ToLower(path), ".exe")
	}
	// On Unix, check executable bit
	return info.Mode()&0111 != 0
}
