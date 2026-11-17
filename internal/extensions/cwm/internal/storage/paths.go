// SPDX-License-Identifier: Apache-2.0

package storage

import "path/filepath"

// Paths resolves CWM's on-disk layout relative to a workspace root (the
// project's `.katyoti/` folder). The committed objects live in the project's
// modules folder, not here; this root holds what is uncommitted:
//
//	<root>/                              ← the project's .katyoti folder
//	  config/
//	    workspace.yaml                   ← CWM budgets + settings
//	    CWMService.yaml                  ← CWM service config
//	  cwm/
//	    types/edge-types/{name}.yaml     # workspace-local edge kinds
//	    objects/{module}/…               # objects of a runtime type
//	    .index/                          # search index + vector caches
//	  .lock                              # the single-writer lock
type Paths struct {
	root string
}

// NewPaths constructs a Paths anchored at the given workspace root
// (the `.katyoti/` folder, not the `cwm/` subfolder).
func NewPaths(root string) Paths { return Paths{root: root} }

// Root returns the workspace root (the `.katyoti/` folder).
func (p Paths) Root() string { return p.root }

// CWMDataDir returns <root>/cwm — the folder all CWM data lives
// under. Callers iterating CWM-owned subtrees should anchor here
// instead of `Root()` so non-CWM katyoti subdirs (skills/, tasks/,
// sync/, config/) stay outside the walk.
func (p Paths) CWMDataDir() string {
	return filepath.Join(p.root, "cwm")
}

// ConfigDir returns <root>/config — the framework-standard config
// dir where every per-service YAML lives.
func (p Paths) ConfigDir() string {
	return filepath.Join(p.root, "config")
}

// WorkspaceYAMLPath returns <root>/config/workspace.yaml. CWM's
// budget + storage settings are framework-standard config, not data,
// so they live in the same `config/` dir as every other service's
// settings file.
func (p Paths) WorkspaceYAMLPath() string {
	return filepath.Join(p.ConfigDir(), "workspace.yaml")
}

// TypesDir returns <root>/cwm/types.
func (p Paths) TypesDir() string {
	return filepath.Join(p.CWMDataDir(), "types")
}

// EdgeTypesDir returns <root>/cwm/types/edge-types.
func (p Paths) EdgeTypesDir() string {
	return filepath.Join(p.TypesDir(), "edge-types")
}
