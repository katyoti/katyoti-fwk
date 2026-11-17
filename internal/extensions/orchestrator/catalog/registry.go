// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// RegistryEntry is one row in ~/ktw-agents/universes.yaml.
// Maps a friendly universe name to its home folder path.
//
// There is intentionally NO `default` field. Default-displayed is a GUI
// state preference (lives in ~/.config/katyoti/config/GuiSettings.yaml).
// CLI fallback is the hardcoded path ~/.config/katyoti/universes/default/.
// A universe doesn't "know" whether it's anyone's default.
type RegistryEntry struct {
	Name     string `yaml:"name"`
	HomePath string `yaml:"home_path"`
	// Protected prevents DeleteUniverse from succeeding. Only the admin
	// universe carries this flag in v1 (set to true by the orchestrator's
	// auto-creation logic per the admin-universe ADR).
	Protected bool `yaml:"protected,omitempty"`
}

// Registry is the cross-universe catalog backed by ~/ktw-agents/universes.yaml.
// Thread-safe.
type Registry struct {
	mu      sync.RWMutex
	path    string
	entries map[string]*RegistryEntry // keyed by name
	order   []string                  // preserves yaml file order
}

// LoadRegistry reads the registry from path. If the file is missing, returns
// an empty (in-memory) registry — the file will be created on first Save.
func LoadRegistry(path string) (*Registry, error) {
	r := &Registry{
		path:    path,
		entries: map[string]*RegistryEntry{},
	}
	bytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Empty registry — first run; not an error.
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read registry %s: %w", path, err)
	}
	var raw struct {
		Universes []RegistryEntry `yaml:"universes"`
	}
	if err := yaml.Unmarshal(bytes, &raw); err != nil {
		return nil, fmt.Errorf("decode registry %s: %w", path, err)
	}
	for i := range raw.Universes {
		e := raw.Universes[i]
		r.entries[e.Name] = &e
		r.order = append(r.order, e.Name)
	}
	return r, nil
}

// DefaultRegistryPath returns the cross-universe registry path —
// `<GlobalHomeDir>/universes.yaml`.
func DefaultRegistryPath() (string, error) {
	root := application.Env().GlobalHomeDir()
	if root == "" {
		return "", fmt.Errorf("resolve global agents root")
	}
	return filepath.Join(root, "universes.yaml"), nil
}

// Path returns the registry's backing file path.
func (r *Registry) Path() string {
	return r.path
}

// List returns all entries in their yaml file order.
func (r *Registry) List() []RegistryEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]RegistryEntry, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, *r.entries[name])
	}
	return out
}

// Find returns the entry matching name, or (nil, error) if not present.
func (r *Registry) Find(name string) (*RegistryEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.entries[name]; ok {
		copy := *e
		return &copy, nil
	}
	return nil, fmt.Errorf("universe %q not found in registry %s", name, r.path)
}

// Add registers a new entry and saves to disk. Errors if the name already exists.
//
// There is no "default" concept at the registry layer: default-displayed is a
// GUI state preference (~/ktw-agents/config/GuiSettings.yaml); CLI fallback is
// a hardcoded path. A universe doesn't carry a default flag.
func (r *Registry) Add(entry RegistryEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.entries[entry.Name]; exists {
		return fmt.Errorf("universe %q already in registry", entry.Name)
	}
	cp := entry
	r.entries[entry.Name] = &cp
	r.order = append(r.order, entry.Name)
	return r.saveLocked()
}

// Remove deletes an entry by name and saves to disk.
// Returns true if the entry existed, false if it didn't (idempotent).
func (r *Registry) Remove(name string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.entries[name]; !ok {
		return false, nil
	}
	delete(r.entries, name)
	for i, n := range r.order {
		if n == name {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return true, r.saveLocked()
}

// saveLocked persists the registry. Caller must hold r.mu.
func (r *Registry) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	out := struct {
		Universes []RegistryEntry `yaml:"universes"`
	}{Universes: make([]RegistryEntry, 0, len(r.order))}
	for _, name := range r.order {
		out.Universes = append(out.Universes, *r.entries[name])
	}
	bytes, err := yaml.Marshal(&out)
	if err != nil {
		return fmt.Errorf("encode registry: %w", err)
	}
	if err := os.WriteFile(r.path, bytes, 0o644); err != nil {
		return fmt.Errorf("write registry %s: %w", r.path, err)
	}
	return nil
}
