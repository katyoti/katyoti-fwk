// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// newDeterministicRNG returns a *rand.Rand seeded with a fixed value
// so property-style tests are reproducible. Replace with t.Name()
// hashing if a different seed per test is ever required.
func newDeterministicRNG(t *testing.T) *rand.Rand {
	t.Helper()
	return rand.New(rand.NewSource(0xC0FFEE))
}

// newTestStorage builds a Storage anchored at a temp directory
// seeded with a minimal workspace.yaml + the edge types ktw-agents
// ships (app/ktw-agents/embed/cwm; modules/cwm/concepts.md). Used by
// every Storage-level test.
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	root := t.TempDir()

	// Minimal workspace.yaml.
	wsYAML := []byte("version: \"1\"\nbudgets:\n  records_max_size_bytes: 2048\n")
	if err := WriteWorkspaceRaw(root, wsYAML); err != nil {
		t.Fatalf("seed workspace.yaml: %v", err)
	}

	// Symlink isn't portable on Windows; copy the shipped edge-type YAMLs
	// from the ktw-agents embed dir instead.
	srcRoot := filepath.Join("..", "..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm")
	dstTypes := NewPaths(root).TypesDir()
	if err := copyDir(filepath.Join(srcRoot, "edge-types"), filepath.Join(dstTypes, "edge-types")); err != nil {
		t.Fatalf("copy edge-types: %v", err)
	}

	s, err := New(context.Background(), root)
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// copyDir is a minimal recursive directory copy for test seeding.
// Skips subdirectories; only flat *.yaml + README.md need to land.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// TestPathsLayout confirms the canonical CWM layout is reproduced
// by the Paths helpers without any drift from the documented shape.
func TestPathsLayout(t *testing.T) {
	p := NewPaths(filepath.FromSlash("/ws"))

	cases := map[string]string{
		"WorkspaceYAMLPath": p.WorkspaceYAMLPath(),
		"TypesDir":          p.TypesDir(),
		"EdgeTypesDir":      p.EdgeTypesDir(),
	}

	want := map[string]string{
		"WorkspaceYAMLPath": filepath.FromSlash("/ws/config/workspace.yaml"),
		"TypesDir":          filepath.FromSlash("/ws/cwm/types"),
		"EdgeTypesDir":      filepath.FromSlash("/ws/cwm/types/edge-types"),
	}

	for name, got := range cases {
		if got != want[name] {
			t.Errorf("%s = %q want %q", name, got, want[name])
		}
	}
}

// TestLoadWorkspaceFromFixture asserts a workspace.yaml with every
// historical block populated still loads: Version is read, the rest
// lands in Extras.
func TestLoadWorkspaceFromFixture(t *testing.T) {
	root := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "workspace_canonical.yaml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := WriteWorkspaceRaw(root, src); err != nil {
		t.Fatalf("seed workspace.yaml: %v", err)
	}

	cfg, err := LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}

	if cfg.Version != "1" {
		t.Errorf("Version = %q want %q", cfg.Version, "1")
	}
	// The retired blocks still decode: they land in Extras.
	for _, k := range []string{"embedding", "storage", "index", "budgets", "types", "git"} {
		if _, ok := cfg.Extras[k]; !ok {
			t.Errorf("Extras[%q] missing; a workspace.yaml carrying it must still load", k)
		}
	}
}

// TestLoadWorkspaceMissingFile asserts a clean wrapped error (not a
// panic) when workspace.yaml is absent. Callers may use
// errors.Is(err, os.ErrNotExist) to detect "no workspace yet" and
// trigger the bootstrap path.
func TestLoadWorkspaceMissingFile(t *testing.T) {
	root := t.TempDir()
	_, err := LoadWorkspace(root)
	if err == nil {
		t.Fatalf("LoadWorkspace on empty dir: want error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("LoadWorkspace error = %v, want errors.Is os.ErrNotExist", err)
	}
}

// TestLoadKindsMissingDirsIsEmpty asserts that a workspace shipping
// no _item_types/ or _edge-types/ at all still loads cleanly with empty
// registries.
func TestLoadKindsMissingDirsIsEmpty(t *testing.T) {
	dir := t.TempDir()
	reg, err := LoadTypes(dir)
	if err != nil {
		t.Fatalf("LoadTypes on empty dir: %v", err)
	}
	if got := len(reg.EdgeTypes()); got != 0 {
		t.Errorf("EdgeKinds = %d want 0", got)
	}
}

// TestWriteWorkspaceRoundTrip writes a config, reads it back, and
// confirms the values survive.
func TestWriteWorkspaceRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := &WorkspaceConfig{Version: "1"}
	if err := WriteWorkspace(root, cfg); err != nil {
		t.Fatalf("WriteWorkspace: %v", err)
	}
	got, err := LoadWorkspace(root)
	if err != nil {
		t.Fatalf("LoadWorkspace after write: %v", err)
	}
	if got.Version != cfg.Version {
		t.Errorf("round-trip mismatch:\nwrote: %+v\nread:  %+v", cfg, got)
	}
}
