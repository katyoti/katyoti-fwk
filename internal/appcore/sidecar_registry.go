// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	sidecarapi "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"gopkg.in/yaml.v3"
)

// The membership DTOs live in sidecar/api so that packages describing sidecars
// (orchestrator, marketplace and the remote clients over them) need not link the
// application framework. This file keeps the behaviour that reads and writes
// them, which does need a resolved environment.
//
// Aliases point DOWNWARD, at a lighter package — the direction that keeps weight
// out of consumers.
type (
	UniverseSidecars = sidecarapi.UniverseSidecars
	SidecarSpec      = sidecarapi.SidecarSpec
)

// UniverseSidecarsPath returns the on-disk file from which the sidecars
// block is read: `<UniverseDir>/config/Univers.yaml`. Watchers (ktw-hub's
// membership watcher) pin to this path.
func UniverseSidecarsPath() string {
	return filepath.Join(application.Env().UniverseDir(), "config", "Univers.yaml")
}

// LoadUniverseSidecars reads `<HomeDir>/config/Univers.yaml` and returns
// its sidecars block as an UniverseSidecars projection. Returns an empty
// registry (NOT an error) when the file is missing — bare workspaces have no
// sidecars yet.
func LoadUniverseSidecars() (*UniverseSidecars, error) {
	return LoadUniverseSidecarsFrom(UniverseSidecarsPath())
}

// pureHostBinaries are ktw-* binaries that must NEVER act as sidecars
// — they're pure host apps (the GUI desktop shell, the CLI multi-tool, the
// ktw-display bun server that displaybridge spawns on demand and that answers
// no `sc` verb).
// Mirrors `catalog.HostBinaries` but kept inline here to avoid an
// `appcore → orchestrator/catalog` import cycle. Keep the two lists in
// sync. ktw-agents / ktw-univers are deliberately absent: both can run
// as sidecars.
var pureHostBinaries = map[string]bool{
	"ktw-gui":     true,
	"ktw-utils":   true,
	"ktw-display": true,
}

// LoadUniverseSidecarsFrom reads Univers.yaml from `path` and projects its
// sidecars block into an UniverseSidecars. Other Univers.yaml fields
// (name / icon / color / etc.) are ignored at this layer — see
// `catalog.LoadManifest` for the full read.
//
// Defense-in-depth: any sidecar whose name is in `pureHostBinaries` is
// silently stripped — a hand-edited Univers.yaml listing ktw-gui as a
// sidecar would still be ignored by the hub catalog.
func LoadUniverseSidecarsFrom(path string) (*UniverseSidecars, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &UniverseSidecars{Version: 1}, nil
		}
		return nil, fmt.Errorf("read Univers.yaml at %s: %w", path, err)
	}

	// Decode only the fields we care about; ignore the rest.
	var probe struct {
		Version  int           `yaml:"version,omitempty"`
		Sidecars []SidecarSpec `yaml:"sidecars,omitempty"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("parse sidecars block from %s: %w", path, err)
	}
	filtered := probe.Sidecars[:0]
	for _, s := range probe.Sidecars {
		if pureHostBinaries[s.Name] {
			continue
		}
		if err := s.Validate(); err != nil {
			// Kept, as an unreachable member always was: the hub skips it and
			// says so; the line names it at load.
			slog.Warn("Univers.yaml: unreachable member", "path", path, "error", err)
		}
		filtered = append(filtered, s)
	}
	reg := &UniverseSidecars{
		Version:  probe.Version,
		Sidecars: filtered,
	}
	if reg.Version == 0 {
		reg.Version = 1
	}
	return reg, nil
}

// SaveUniverseSidecars writes r back into the active workspace's Univers.yaml.
//
// A function rather than a method: the type now lives in sidecar/api, which must
// not link the environment resolver that supplies the path. SaveTo(path) is the
// method, and it lives with the type.
func SaveUniverseSidecars(r *UniverseSidecars) error {
	return r.SaveTo(UniverseSidecarsPath())
}
