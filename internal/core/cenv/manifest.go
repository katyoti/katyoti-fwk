// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// UniverseManifest is the minimum subset of <HomeDir>/config/Univers.yaml
// that the cenv layer needs (just the name, used to scope DataDir).
//
// HomeDir = <KTW_WORKSPACE>/.katyoti, so the manifest path is
// `<KTW_WORKSPACE>/.katyoti/config/Univers.yaml`.
//
// The full manifest grammar (sidecars, icon, color, displayName, …) lives in
// the orchestrator package's `catalog.Manifest` — duplicating just the Name
// here is intentional to avoid a layering violation (core/cenv →
// extensions/orchestrator).
type UniverseManifest struct {
	Name string `yaml:"name"`
	// AllowOverwrites opts this universe into workspace resource overrides
	// (`.katyoti/overwrites/<sidecar>/…` shadowing a sidecar's embedded
	// resources). Default false = prod-safe: a missing manifest or missing key
	// disables overlays. Set true in dev universes only.
	AllowOverwrites bool `yaml:"allow_overwrites"`
	// Sidecars is read for what the universe GRANTS each member beyond what it
	// owns (`sidecars[].grants`); the membership itself is the sidecar
	// registry's to read (appcore.LoadUniverseSidecarsFrom).
	Sidecars []UniverseSidecarGrants `yaml:"sidecars,omitempty"`
	// GUI is what the universe's GUI shows; absent, it shows everything.
	GUI *UniverseGUI `yaml:"gui,omitempty"`
	// Tags name what the universe is for: each a `universe-tag` a member
	// pack declares, an allow-list of what the GUI shows and at which
	// display level. The view is their union, adjusted by GUI; a universe
	// with no tag shows everything. Written by the hub (rule).
	Tags []string `yaml:"tags,omitempty"`
	// FirstGuide is the guide the universe starts with: a guide-recipe URI
	// among its sidecars' (`<sidecar>:/guide-recipe/<id>`), chosen when it is
	// created. Written by the hub (rule).
	FirstGuide string `yaml:"first_guide,omitempty"`
	// Solution is the solution that installed the universe
	// (<solution>@<version>), so what it needs — its programs, its key sets —
	// is found again from the universe (the Ready to start screen). Written by
	// the hub when it installs the solution (rule); empty for a
	// universe made by hand.
	Solution string `yaml:"solution,omitempty"`
}

// UniverseGUI is what a universe declares of its GUI in Univers.yaml, the
// section an architect's universe uses to show only what an architect needs:
//
//	gui:
//	  hide: [home:planning-deck, home:person-queue, home:runs, home:errors, rail:debug, rail:dev-console]
//	  first: [archifactory]
//
// Hide names the GUI elements left out unless the person's level is Expert,
// each by its element id `<place>:<name>`: Home's strip
// (`home:runs`, a global panel `home:<its stem>`), the rail (`rail:debug`),
// the quick actions, the badges, an object's tabs. First names task
// hashtags: Home lists the tasks carrying one before the others. The hub
// carries it in the universe summary; the GUI reads it and never writes it.
type UniverseGUI struct {
	Hide  []string `yaml:"hide,omitempty"  json:"hide,omitempty"`
	First []string `yaml:"first,omitempty" json:"first,omitempty"`
	// Levels adjusts, for this universe, the lowest display level that sees
	// an element its tags allow (element id -> essentiel, standard or expert).
	Levels map[string]string `yaml:"levels,omitempty" json:"levels,omitempty"`
}

// UniverseSidecarGrants is one member as the grants read it: its name and
// what the universe grants it.
type UniverseSidecarGrants struct {
	Name   string         `yaml:"name"`
	Grants *SidecarGrants `yaml:"grants,omitempty"`
}

// SidecarGrants is what a universe grants one sidecar beyond what it owns.
// Written in Univers.yaml on the sidecar's entry:
//
//	sidecars:
//	  - name: ktw-review
//	    grants:
//	      gate: {types: [milestone], capabilities: [checkable]}
type SidecarGrants struct {
	// Gate widens where the sidecar's gate entries (before_*, on_transition:
	// the points that may refuse a write) apply. By default they apply only to
	// the objects whose type the sidecar declares or that carry, by their own
	// choice, a capability it declares.
	Gate GateGrant `yaml:"gate,omitempty" json:"gate,omitempty"`
}

// GateGrant names the object types (`*` = every type) and the capabilities
// whose objects a sidecar's gate entries may also refuse writes on.
type GateGrant struct {
	Types        []string `yaml:"types,omitempty"        json:"types,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

// IsZero reports a grant naming nothing.
func (g GateGrant) IsZero() bool { return len(g.Types) == 0 && len(g.Capabilities) == 0 }

// GateGrants is the gate grant of every member that has one, by sidecar name.
func (m *UniverseManifest) GateGrants() map[string]GateGrant {
	if m == nil {
		return nil
	}
	var out map[string]GateGrant
	for _, s := range m.Sidecars {
		if s.Grants == nil || s.Grants.Gate.IsZero() {
			continue
		}
		if out == nil {
			out = map[string]GateGrant{}
		}
		out[s.Name] = s.Grants.Gate
	}
	return out
}

// GateGrantsAt is the gate grants of the universe at homePath (the .katyoti
// config root); nil when its manifest cannot be read.
func GateGrantsAt(homePath string) map[string]GateGrant {
	m, err := LoadUniverseManifest(homePath)
	if err != nil {
		return nil
	}
	return m.GateGrants()
}

var (
	universeManifestCache = map[string]universeManifestEntry{}
	universeManifestMu    sync.RWMutex
)

type universeManifestEntry struct {
	manifest *UniverseManifest
	err      error
}

// LoadUniverseManifest reads `<homePath>/config/Univers.yaml` and returns the
// parsed manifest (just the Name field). `homePath` is the .katyoti config
// root — i.e. `<KTW_WORKSPACE>/.katyoti`. Caches the result per homePath.
func LoadUniverseManifest(homePath string) (*UniverseManifest, error) {
	homePath = filepath.Clean(homePath)
	universeManifestMu.RLock()
	if cached, ok := universeManifestCache[homePath]; ok {
		universeManifestMu.RUnlock()
		return cached.manifest, cached.err
	}
	universeManifestMu.RUnlock()

	path := filepath.Join(homePath, "config", "Univers.yaml")
	bytes, err := os.ReadFile(path)
	var entry universeManifestEntry
	switch {
	case os.IsNotExist(err):
		entry.err = fmt.Errorf("Univers.yaml not found at %s — this workspace is not initialized; run `ktw-univers init` (or pass --workspace <path>)", path)
	case err != nil:
		entry.err = fmt.Errorf("read Univers.yaml at %s: %w", path, err)
	default:
		var m UniverseManifest
		if err := yaml.Unmarshal(bytes, &m); err != nil {
			entry.err = fmt.Errorf("decode Univers.yaml at %s: %w", path, err)
		} else if m.Name == "" {
			entry.err = fmt.Errorf("Univers.yaml at %s: missing required field 'name'", path)
		} else {
			entry.manifest = &m
		}
	}

	universeManifestMu.Lock()
	universeManifestCache[homePath] = entry
	universeManifestMu.Unlock()
	return entry.manifest, entry.err
}

// OverwritesDir returns the workspace override tree for one sidecar:
// `<homePath>/overwrites/<sidecar>`, where homePath is the .katyoti config root.
// Single source for the path so the sidecar loader and the hub cache-digest
// agree. Override files mirror the embed layout below this (e.g.
// `<dir>/embed/skills/<name>.md`).
func OverwritesDir(homePath, sidecar string) string {
	return filepath.Join(homePath, "overwrites", sidecar)
}

// AllowOverwrites reports whether the universe at homePath opts into workspace
// resource overrides. Gated false on any error (missing/invalid manifest) — the
// safe default. `homePath` is the .katyoti config root (`<KTW_WORKSPACE>/.katyoti`).
func AllowOverwrites(homePath string) bool {
	m, err := LoadUniverseManifest(homePath)
	return err == nil && m != nil && m.AllowOverwrites
}

// ResetUniverseManifestCache clears the cache. Tests use this to force
// re-resolution after changing KTW_WORKSPACE or Univers.yaml content.
func ResetUniverseManifestCache() {
	universeManifestMu.Lock()
	defer universeManifestMu.Unlock()
	universeManifestCache = map[string]universeManifestEntry{}
}

// ResolveTenantUniverse returns the canonical (tenantID, universeID)
// pair for NATS subject construction: tenant from KTW_TENANT_ID
// (default "local"), universe from the workspace's Univers.yaml name
// (default "default" when the manifest is missing/unnamed).
//
// This is THE single source for scope tokens — the bus sender, the
// CWM event publisher, and the EventService subject fallback must all
// resolve through it. If any side hardcodes its own values, publish
// and subscribe subjects drift apart (e.g. publisher on
// `universe.default`, subscriber on `universe.acme`) and events are
// silently never delivered.
func ResolveTenantUniverse(homePath string) (tenantID, universeID string) {
	tenantID = os.Getenv("KTW_TENANT_ID")
	if tenantID == "" {
		tenantID = "local"
	}
	universeID = "default"
	if m, err := LoadUniverseManifest(homePath); err == nil && m != nil && m.Name != "" {
		universeID = m.Name
	}
	return tenantID, universeID
}
