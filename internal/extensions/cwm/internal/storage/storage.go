// SPDX-License-Identifier: Apache-2.0

// Package storage implements CWM's disk layer: the workspace, the store's
// lock, and the pack declarations (object types, capabilities, edge types,
// panels, workflows, policies, hooks) read from each pack's folders.
//
// Every YAML read goes through config.DecodeBytes (variable expansion +
// ProcessConfig); raw yaml.Unmarshal is forbidden.
//
// The atomic-io primitives live in the shared `internal/core/atomicio`
// package (also used by config.SaveAtomic).
package storage

import (
	"context"
	"io/fs"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/concurrency"
)

// Storage is the orchestrator for CWM's disk layer. Holds the type
// registry, the path resolver, and the workspace's process lockfile.
type Storage struct {
	paths Paths
	types *TypeRegistry
	lock  *concurrency.ProcessLockfile
}

// New constructs a Storage anchored at root with NO embedded kind
// sources — the workspace's own `types/_kinds/` + `_edge_kinds/`
// are the only sources. Used by substrate tests that seed kinds
// directly to disk. Production callers should prefer
// `NewWithTypes` so the binary-shipped canonical kinds are
// available without disk-side seeding.
func New(ctx context.Context, root string) (*Storage, error) {
	return NewWithTypes(ctx, root)
}

// NewWithTypes constructs a Storage anchored at root, layering
// the workspace's `types/_kinds/` + `_edge_kinds/` on top of the
// supplied embedded FS sources. Later sources overwrite earlier
// ones by kind name (closer-wins).
//
// Typical production wiring (from cwmlocal):
//
//	storage.NewWithTypes(ctx, root, cwmembed.FS)
//
// Checks workspace.yaml is present, loads the kind registry, and
// acquires the multi-process workspace lockfile.
//
// Returns api.ErrLockHeldByOtherProcess if another live process
// already holds the workspace. Returns a wrapped error if
// workspace.yaml is missing — callers should ensure the marker
// exists (cwmlocal.ensureFirstRunWorkspace) before calling.
func NewWithTypes(ctx context.Context, root string, embedded ...fs.FS) (*Storage, error) {
	p := NewPaths(root)
	if _, err := LoadWorkspace(root); err != nil {
		return nil, err
	}
	// Layer the workspace's user-overridable dir on top of the
	// embedded canonical kinds. Disk dir wins on collision.
	sources := append([]fs.FS{}, embedded...)
	sources = append(sources, os.DirFS(p.TypesDir()))
	kinds, err := LoadTypesLayered(sources...)
	if err != nil {
		return nil, err
	}
	lock := concurrency.NewProcessLockfile(root)
	if err := lock.Acquire(ctx); err != nil {
		return nil, err
	}
	return &Storage{
		paths: p,
		types: kinds,
		lock:  lock,
	}, nil
}

// Close releases the multi-process lockfile. Must be called when
// the workspace is no longer needed; otherwise the lockfile remains
// on disk until the heartbeat lapses and another process detects
// the wedged state.
func (s *Storage) Close() error {
	if s.lock == nil {
		return nil
	}
	return s.lock.Release()
}

// CheckLease refuses a write once this process lost the workspace lock to
// another process (lease fencing): nil while it holds it.
func (s *Storage) CheckLease() error {
	if s.lock == nil {
		return nil
	}
	return s.lock.Check()
}

// Paths exposes the canonical layout helpers.
func (s *Storage) Paths() Paths { return s.paths }

// Kinds returns the loaded kind registry.
func (s *Storage) Types() *TypeRegistry { return s.types }
