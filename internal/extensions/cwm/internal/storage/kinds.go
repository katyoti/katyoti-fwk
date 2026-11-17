// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TypeRegistry holds the parsed item + edge + node type registrations.
//
// Maps are populated at construction (LoadTypesLayered + Merge, single-threaded,
// pre-share) and thereafter set ONLY by Replace — the event-driven refresh that
// puts a catalog snapshot's edge types in force without a restart
// the RWMutex makes it safe against concurrent reads.
type TypeRegistry struct {
	mu    sync.RWMutex
	edges map[api.EdgeType]api.EdgeTypeRegistration
	// floor are the edge types loaded from the type sources (LoadTypesLayered):
	// structural, never replaced nor dropped by a catalog's (Replace) - save
	// by a project's override entry of one (Override).
	floor map[api.EdgeType]bool
	// floorRegs are the floor's own registrations, put back when an override
	// of one goes.
	floorRegs map[api.EdgeType]api.EdgeTypeRegistration
}

// LoadTypes walks <typesDir>/_kinds and <typesDir>/_edge_kinds,
// decoding each *.yaml file into the corresponding registration
// struct. Files prefixed with "_" (README, index files) are skipped.
//
// The `schema:` subtree of an item-kind YAML is re-marshaled to JSON
// to populate ItemTypeRegistration.Schema (json.RawMessage). This
// lets downstream validators consume the kind's JSON Schema without
// re-parsing YAML.
//
// Disk-only convenience: equivalent to
// `LoadTypesLayered(os.DirFS(typesDir))`. Production CWM uses
// LoadTypesLayered to layer the binary's embedded canonical
// kinds underneath the workspace's user-overridable types/ dir.
func LoadTypes(typesDir string) (*TypeRegistry, error) {
	return LoadTypesLayered(os.DirFS(typesDir))
}

// LoadTypesLayered walks every source's `_kinds/` + `_edge_kinds/`
// directories in order. Each source is an `fs.FS` rooted at a
// directory containing those two sub-folders. Later sources
// overwrite earlier ones on kind-name collision (closer-wins
// layering).
//
// Typical production wiring (from cwmlocal):
//
//	LoadTypesLayered(
//	    cwmembed.FS,             // binary-shipped canonical (floor)
//	    os.DirFS(typesDir),      // workspace user overrides (top)
//	)
//
// A source whose `_kinds/` or `_edge_kinds/` sub-folder is absent
// silently contributes nothing — workspaces legitimately ship
// without one of the two folders. Empty source list returns an
// empty registry.
func LoadTypesLayered(sources ...fs.FS) (*TypeRegistry, error) {
	reg := &TypeRegistry{edges: map[api.EdgeType]api.EdgeTypeRegistration{}}
	for i, src := range sources {
		if src == nil {
			continue
		}
		dir := folderIn(src, "edge-types", "edge_types")
		if err := loadEdgeTypesFS(reg, src, dir); err != nil {
			return nil, fmt.Errorf("cwm/storage: source %d %s: %w", i, dir, err)
		}
	}
	reg.floor = make(map[api.EdgeType]bool, len(reg.edges))
	reg.floorRegs = make(map[api.EdgeType]api.EdgeTypeRegistration, len(reg.edges))
	for name, er := range reg.edges {
		reg.floor[name] = true
		reg.floorRegs[name] = er
	}
	return reg, nil
}

// Replace sets the catalog's edge types to edges, whole: a changed one
// replaces the one in force, one edges no longer names goes.
// The floor (the type sources') stays as it is. Thread-safe.
func (r *TypeRegistry) Replace(edges []api.EdgeTypeRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := make(map[api.EdgeType]api.EdgeTypeRegistration, len(r.edges))
	for name, er := range r.edges {
		if r.floor[name] {
			next[name] = er
		}
	}
	for _, er := range edges {
		if er.Name == "" || r.floor[er.Name] {
			continue
		}
		if _, dup := next[er.Name]; !dup {
			next[er.Name] = er
		}
	}
	r.edges = next
}

// Override puts in force, over the floor, the floor types a project's
// override entry changed (the catalog's declaration carrying it): a floor
// type edges names is replaced by it, one it no longer names is the floor's
// again. A type edges names that is not a floor type is left to Merge and
// Replace. Thread-safe.
func (r *TypeRegistry) Override(edges []api.EdgeTypeRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, er := range r.floorRegs {
		r.edges[name] = er
	}
	for _, er := range edges {
		if r.floor[er.Name] {
			r.edges[er.Name] = er
		}
	}
}

// Merge overlays additional type registrations onto the registry —
// FLOOR-WINS: an entry whose name already exists is kept, the incoming one is
// skipped. Used to layer types resolved from the aggregate catalog (contributed
// by OTHER sidecars) onto this binary's in-binary floor, WITHOUT letting a
// remote contribution shadow a structural floor type (e.g. workspace / inbox).
// Construction-time only (single-threaded, pre-share) — unlocked. Runtime
// refresh goes through Replace.
func (r *TypeRegistry) Merge(edges []api.EdgeTypeRegistration) {
	r.mergeLocked(edges)
}

// mergeLocked is the add-if-absent body of Merge (construction, pre-share).
func (r *TypeRegistry) mergeLocked(edges []api.EdgeTypeRegistration) {
	for _, er := range edges {
		if er.Name == "" {
			continue
		}
		if _, exists := r.edges[er.Name]; exists {
			continue
		}
		r.edges[er.Name] = er
	}
}

// EdgeKind returns the registration for the named edge kind.
func (r *TypeRegistry) EdgeType(name api.EdgeType) (api.EdgeTypeRegistration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k, ok := r.edges[name]
	return k, ok
}

// EdgeKinds returns a copy of the edge-kind map.
func (r *TypeRegistry) EdgeTypes() map[api.EdgeType]api.EdgeTypeRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[api.EdgeType]api.EdgeTypeRegistration, len(r.edges))
	for k, v := range r.edges {
		out[k] = v
	}
	return out
}

func loadEdgeTypesFS(reg *TypeRegistry, src fs.FS, dir string) error {
	names, err := readTypeDirFS(src, dir)
	if err != nil {
		return err
	}
	for _, name := range names {
		fpath := path.Join(dir, name)
		raw, err := fs.ReadFile(src, fpath)
		if err != nil {
			return fmt.Errorf("read %s: %w", fpath, err)
		}
		var kind api.EdgeTypeRegistration
		if err := config.DecodeBytes(raw, marshal.YAML, &kind); err != nil {
			slog.Warn("cwm/types: skipping malformed edge type", "file", fpath, "error", err)
			continue
		}
		if err := kind.Validate(); err != nil {
			slog.Warn("cwm/types: skipping invalid edge type", "file", fpath, "error", err)
			continue
		}
		reg.edges[kind.Name] = kind
	}
	return nil
}

// readTypeDirFS lists *.yaml files in `dir` within an fs.FS,
// skipping files prefixed with "_" (README.md, _index.yaml, etc.)
// and any subdirectories. Returns an empty slice (not an error)
// if the directory doesn't exist — a workspace may legitimately
// ship without one of the two folders.
func readTypeDirFS(src fs.FS, dir string) ([]string, error) {
	entries, err := fs.ReadDir(src, dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		if strings.HasSuffix(name, ".i18n.yaml") {
			continue // a declaration's translations, carried as its layer — not a declaration
		}
		out = append(out, name)
	}
	return out, nil
}

// folderIn is a kind's folder in src: its name, or — when src holds none of
// that name — its former one (a source laid out before the rename).
func folderIn(src fs.FS, now, former string) string {
	if fi, err := fs.Stat(src, now); err == nil && fi.IsDir() {
		return now
	}
	if fi, err := fs.Stat(src, former); err == nil && fi.IsDir() {
		return former
	}
	return now
}

// LoadObjectTypesLayered reads every `object-types/*.yaml` from each source,
// later sources overwriting earlier ones on name collision — the same
// closer-wins layering LoadTypesLayered applies to node/item/edge types, so a
// workspace overlay can override a shipped declaration.
//
// It returns a SLICE rather than populating TypeRegistry. The registry holds
// the node/item/edge triple that the object model replaces; parking object
// types inside it would create a second, unread copy of the declarations whose
// only consumer (CWMServiceConfig.CatalogTypes.Objects) reaches them through
// the catalog. The producer reads the FS; the store reads the catalog; nothing
// in between needs a registry entry.
func LoadObjectTypesLayered(sources ...fs.FS) ([]api.ObjectType, error) {
	byName := map[string]api.ObjectType{}
	var order []string
	for i, src := range sources {
		if src == nil {
			continue
		}
		dir := folderIn(src, "object-types", "object_types")
		names, err := readTypeDirFS(src, dir)
		if err != nil {
			return nil, fmt.Errorf("cwm/storage: source %d %s: %w", i, dir, err)
		}
		for _, name := range names {
			fpath := path.Join(dir, name)
			raw, err := fs.ReadFile(src, fpath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", fpath, err)
			}
			var t api.ObjectType
			if err := config.DecodeBytes(raw, marshal.YAML, &t); err != nil {
				// Resilient load, matching the node/item/edge loaders: one
				// malformed file must not take the whole pack down.
				slog.Warn("cwm/types: skipping malformed object type", "file", fpath, "error", err)
				continue
			}
			if t.Name == "" {
				slog.Warn("cwm/types: skipping object type with no name", "file", fpath)
				continue
			}
			if _, seen := byName[t.Name]; !seen {
				order = append(order, t.Name)
			}
			byName[t.Name] = t
		}
	}
	out := make([]api.ObjectType, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// LoadCapabilitiesLayered reads every `capabilities/*.yaml` from each source,
// later sources overwriting earlier ones on name collision — the same
// closer-wins layering every other type folder gets, so a workspace overlay can
// override a shipped capability.
//
// It is a slice, not a registry entry, for the same reason object types are: the
// producer reads the FS, the store reads the catalog, and nothing in between
// needs a third copy.
func LoadCapabilitiesLayered(sources ...fs.FS) ([]api.Capability, error) {
	byName := map[string]api.Capability{}
	var order []string
	for i, src := range sources {
		if src == nil {
			continue
		}
		names, err := readTypeDirFS(src, "capabilities")
		if err != nil {
			return nil, fmt.Errorf("cwm/storage: source %d capabilities: %w", i, err)
		}
		for _, name := range names {
			fpath := path.Join("capabilities", name)
			raw, err := fs.ReadFile(src, fpath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", fpath, err)
			}
			var c api.Capability
			if err := config.DecodeBytes(raw, marshal.YAML, &c); err != nil {
				// Resilient load, matching every sibling loader: one malformed
				// file must not take the pack down.
				slog.Warn("cwm/types: skipping malformed capability", "file", fpath, "error", err)
				continue
			}
			if c.Name == "" {
				// Dropped, not registered: an unnamed capability can never be
				// matched by a type's list, so keeping it would inflate the
				// declared set while matching nothing.
				slog.Warn("cwm/types: skipping capability with no name", "file", fpath)
				continue
			}
			if _, seen := byName[c.Name]; !seen {
				order = append(order, c.Name)
			}
			byName[c.Name] = c
		}
	}
	out := make([]api.Capability, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// LoadAspectsLayered reads every `aspects/*.yaml` from each source, later
// sources overriding earlier ones by name — the same layering as capabilities.
func LoadAspectsLayered(sources ...fs.FS) ([]api.Aspect, error) {
	byName := map[string]api.Aspect{}
	var order []string
	for i, src := range sources {
		if src == nil {
			continue
		}
		names, err := readTypeDirFS(src, "aspects")
		if err != nil {
			return nil, fmt.Errorf("cwm/storage: source %d aspects: %w", i, err)
		}
		for _, name := range names {
			fpath := path.Join("aspects", name)
			raw, err := fs.ReadFile(src, fpath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", fpath, err)
			}
			var c api.Aspect
			if err := config.DecodeBytes(raw, marshal.YAML, &c); err != nil {
				// Resilient load, matching every sibling loader: one malformed
				// file must not take the pack down.
				slog.Warn("cwm/types: skipping malformed aspect", "file", fpath, "error", err)
				continue
			}
			if c.Name == "" {
				// Dropped, not registered: an unnamed aspect can never be
				// matched by a type's list, so keeping it would inflate the
				// declared set while matching nothing.
				slog.Warn("cwm/types: skipping aspect with no name", "file", fpath)
				continue
			}
			if _, seen := byName[c.Name]; !seen {
				order = append(order, c.Name)
			}
			byName[c.Name] = c
		}
	}
	out := make([]api.Aspect, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// LoadNamedLayered reads every `<dir>/*.yaml` from each source into T, later
// sources overriding earlier ones by name (the file's basename when the body
// names none). The store floor's loader of the named pack kinds (the catalog
// reads a pack through the kinds' source declarations).
func LoadNamedLayered[T any](dir, kind string, name func(*T) *string, sources ...fs.FS) ([]T, error) {
	byName := map[string]T{}
	var order []string
	for i, src := range sources {
		if src == nil {
			continue
		}
		names, err := readTypeDirFS(src, dir)
		if err != nil {
			return nil, fmt.Errorf("cwm/storage: source %d %s: %w", i, dir, err)
		}
		for _, file := range names {
			fpath := path.Join(dir, file)
			raw, err := fs.ReadFile(src, fpath)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", fpath, err)
			}
			var v T
			if err := config.DecodeBytes(raw, marshal.YAML, &v); err != nil {
				slog.Warn("cwm/types: skipping malformed "+kind, "file", fpath, "error", err)
				continue
			}
			n := name(&v)
			if *n == "" {
				*n = strings.TrimSuffix(file, path.Ext(file))
			}
			if _, seen := byName[*n]; !seen {
				order = append(order, *n)
			}
			byName[*n] = v
		}
	}
	out := make([]T, 0, len(order))
	for _, n := range order {
		out = append(out, byName[n])
	}
	return out, nil
}

// LoadWorkflowsLayered reads every `workflows/*.yaml`.
func LoadWorkflowsLayered(sources ...fs.FS) ([]api.Workflow, error) {
	return LoadNamedLayered("workflows", "workflow", func(w *api.Workflow) *string { return &w.Name }, sources...)
}

// LoadPoliciesLayered reads every `policies/*.yaml`.
func LoadPoliciesLayered(sources ...fs.FS) ([]api.Policy, error) {
	return LoadNamedLayered("policies", "policy", func(p *api.Policy) *string { return &p.Name }, sources...)
}

// LoadHooksLayered reads every `hooks/*.yaml`.
func LoadHooksLayered(sources ...fs.FS) ([]api.Hook, error) {
	return LoadNamedLayered("hooks", "hook", func(h *api.Hook) *string { return &h.Name }, sources...)
}

// LoadEventTypesLayered reads every `event-types/*.yaml`.
func LoadEventTypesLayered(sources ...fs.FS) ([]api.EventType, error) {
	return LoadNamedLayered("event-types", "event-type", func(t *api.EventType) *string { return &t.Name }, sources...)
}

// LoadHookTypesLayered reads every `hook-types/*.yaml`.
func LoadHookTypesLayered(sources ...fs.FS) ([]api.HookType, error) {
	return LoadNamedLayered("hook-types", "hook-type", func(t *api.HookType) *string { return (*string)(&t.Name) }, sources...)
}
