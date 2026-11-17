// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// WorkspaceConfig is the typed view of <root>/config/workspace.yaml,
// the marker a CWM store is anchored on. Only Version is CWM's; any
// other top-level key lands in Extras (which is also what keeps a
// file written with the retired embedding / storage / index /
// budgets / types / git blocks decodable).
type WorkspaceConfig struct {
	Version string         `yaml:"version"   json:"version"`
	Extras  map[string]any `yaml:",inline"   json:"-"`
}

// LoadWorkspace reads <root>/workspace.yaml and returns the typed
// view. Goes through config.DecodeBytes for variable expansion +
// ProcessConfig.
//
// Returns a clear, wrapped error (NOT a panic) when the file is
// missing. Callers may treat os.IsNotExist(err) as "no workspace
// initialized yet" and trigger the bootstrap path.
func LoadWorkspace(root string) (*WorkspaceConfig, error) {
	p := NewPaths(root)
	path := p.WorkspaceYAMLPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cwm/storage: read workspace.yaml at %s: %w", path, err)
	}
	var cfg WorkspaceConfig
	if err := config.DecodeBytes(data, marshal.YAML, &cfg); err != nil {
		return nil, fmt.Errorf("cwm/storage: decode workspace.yaml: %w", err)
	}
	return &cfg, nil
}

// WriteWorkspace serializes cfg to <root>/config/workspace.yaml
// atomically. Used by the bootstrap path (first-run write) and by
// config-mutating service operations.
func WriteWorkspace(root string, cfg *WorkspaceConfig) error {
	p := NewPaths(root)
	data, err := marshal.Marshal(cfg, marshal.YAML)
	if err != nil {
		return fmt.Errorf("cwm/storage: marshal workspace.yaml: %w", err)
	}
	if err := os.MkdirAll(p.ConfigDir(), 0o755); err != nil {
		return fmt.Errorf("cwm/storage: mkdir config: %w", err)
	}
	return atomicio.WriteAtomic(p.WorkspaceYAMLPath(), data, 0o644)
}

// WriteWorkspaceRaw writes raw bytes to <root>/config/workspace.yaml.
// Test helper for fixtures that need to pin a specific on-disk shape
// without round-tripping through WorkspaceConfig. Production code
// should call WriteWorkspace instead.
func WriteWorkspaceRaw(root string, data []byte) error {
	p := NewPaths(root)
	if err := os.MkdirAll(p.ConfigDir(), 0o755); err != nil {
		return fmt.Errorf("cwm/storage: mkdir config: %w", err)
	}
	return atomicio.WriteAtomic(p.WorkspaceYAMLPath(), data, 0o644)
}
