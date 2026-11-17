// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// Manifest is the on-disk shape of a workspace's
// `<workspaceHome>/.katyoti/config/Univers.yaml` — the single source of truth
// for universe identity AND sidecar membership: sidecars live alongside
// identity in Univers.yaml, in its sidecars: block.
//
// Field notes:
//   - `Name` is the slug-style identifier (immutable, registry key).
//   - `DisplayName` is the human-readable label shown in the IconRibbon.
//   - `Sidecars` is the per-workspace sidecar membership.
//   - `GUI` is what the universe's GUI hides and lists first.
//   - `Version` is a monotonic integer (1, 2, 3, …); 0 in YAML treated as 1.
type Manifest struct {
	Version     int               `yaml:"version,omitempty"`
	Name        string            `yaml:"name"`
	DisplayName string            `yaml:"display_name"`
	Description string            `yaml:"description,omitempty"`
	Icon        string            `yaml:"icon,omitempty"`
	Color       string            `yaml:"color,omitempty"`
	Sidecars    []api.SidecarSpec `yaml:"sidecars,omitempty"`
	// GUI is what the universe's GUI hides and lists first (cenv.UniverseGUI);
	// kept through every rewrite of the file.
	GUI *cenv.UniverseGUI `yaml:"gui,omitempty"`
	// Tags name what the universe is for (cenv.UniverseManifest.Tags), kept
	// through every rewrite like GUI.
	Tags []string `yaml:"tags,omitempty"`
	// FirstGuide is the guide the universe starts with
	// (cenv.UniverseManifest.FirstGuide), kept through every rewrite.
	FirstGuide string `yaml:"first_guide,omitempty"`
	// Solution is the solution that installed the universe
	// (cenv.UniverseManifest.Solution), kept through every rewrite.
	Solution string `yaml:"solution,omitempty"`
	// NoWelcome turns the universe's welcome screens off: its GUI opens
	// neither its first guide nor « Quoi de neuf » by itself (a test or a
	// screen capture's universe), kept through every rewrite.
	NoWelcome bool `yaml:"no_welcome,omitempty"`
}

// LoadManifest reads `<workspaceHome>/.katyoti/config/Univers.yaml` and
// decodes it into a Manifest, via the canonical ConfigService pipeline (so
// `envkey`/`default`/`required` tags and variable expansion fire).
//
// `workspaceHome` is the workspace root (the folder containing `.katyoti/`).
// Path composition is delegated to `application.Env().ConfigDirOf(root)`.
//
// Strict: returns an error if `name` is missing. Callers that have a known
// registry slug to use as the fallback should prefer `LoadManifestOrRepair`
// — it heals a stale on-disk file rather than crashing the boot.
func LoadManifest(workspaceHome string) (*Manifest, error) {
	configDir := application.EnvOf(workspaceHome).ConfigDir()
	cs := config.NewConfigService(loader.NewFSLoader(configDir))
	var m Manifest
	if err := cs.LoadConfig("Univers", &m); err != nil {
		return nil, fmt.Errorf("load Univers.yaml from %s: %w", configDir, err)
	}
	if m.Name == "" {
		return nil, fmt.Errorf(
			"Univers.yaml at %s is missing required field 'name' — delete the file and run `ktw-univers init` to start fresh",
			filepath.Join(configDir, "Univers.yaml"),
		)
	}
	return &m, nil
}

// LoadManifestOrRepair reads Univers.yaml from `<workspaceHome>/.katyoti/config/`
// like LoadManifest, but if the on-disk file is missing the required `name`
// field, it self-heals by setting `Name = fallbackName` and rewriting the
// file. A warning is logged so the operator notices.
//
// Use this from callers that already know the universe's registry slug
// (`ListUniverses`, `GetUniverse`, `ensureAdminUniverse`) — they have a
// trustworthy fallback name to use.
//
// `fallbackName` must be non-empty. If you don't have a fallback, use
// `LoadManifest` (strict) and surface the error to the operator.
func LoadManifestOrRepair(workspaceHome, fallbackName string) (*Manifest, error) {
	if fallbackName == "" {
		return nil, fmt.Errorf("LoadManifestOrRepair: fallbackName is required")
	}
	configDir := application.EnvOf(workspaceHome).ConfigDir()
	manifestPath := filepath.Join(configDir, "Univers.yaml")

	m, err := LoadManifest(workspaceHome)
	if err == nil {
		return m, nil
	}

	// Distinguish "missing name" (repairable) from "file missing / malformed
	// YAML / decode error" (NOT repairable here — caller should re-init).
	// We re-read the raw bytes; if the YAML decodes and just lacks `name`,
	// we heal. Anything else is propagated.
	raw, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		return nil, err
	}
	var repaired Manifest
	if decodeErr := yaml.Unmarshal(raw, &repaired); decodeErr != nil {
		return nil, err
	}
	if repaired.Name != "" {
		// LoadManifest rejected for some reason other than missing name —
		// propagate.
		return nil, err
	}
	repaired.Name = fallbackName
	if saveErr := SaveManifest(workspaceHome, &repaired); saveErr != nil {
		return nil, fmt.Errorf("repair Univers.yaml at %s: %w (original load error: %v)", manifestPath, saveErr, err)
	}
	slog.Warn("repaired Univers.yaml missing 'name' field",
		"path", manifestPath,
		"name", fallbackName,
	)
	return &repaired, nil
}

// SaveManifest writes a Manifest to `<workspaceHome>/.katyoti/config/Univers.yaml`.
// Creates the config directory tree if missing.
func SaveManifest(workspaceHome string, m *Manifest) error {
	if m == nil {
		return fmt.Errorf("catalog.SaveManifest: nil manifest")
	}
	if m.Name == "" {
		return fmt.Errorf("catalog.SaveManifest: missing required field 'name'")
	}
	configDir := application.EnvOf(workspaceHome).ConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir %s: %w", configDir, err)
	}
	path := filepath.Join(configDir, "Univers.yaml")
	bytes, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	// Atomic write: sibling tmp + rename, so a crash mid-write can never
	// leave a truncated Univers.yaml (the file every binary boots from).
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write Univers.yaml tmp at %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename Univers.yaml into place at %s: %w", path, err)
	}
	return nil
}

// EnsureSkeleton gives a universe what every binary needs beside Univers.yaml
// to boot in it: an empty `.katyoti/.env` (0o600: it will hold secrets) and
// `.katyoti/config/Settings.yaml` — ktw-agents refuses to serve a universe
// without the latter (« not configured yet »). An existing file is kept. Every
// path that creates a universe calls it.
func EnsureSkeleton(workspaceHome string) error {
	env := application.EnvOf(workspaceHome)
	if err := os.MkdirAll(env.ConfigDir(), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	for _, f := range []struct {
		path string
		body string
		mode os.FileMode
	}{
		{filepath.Join(env.HomeDir(), ".env"), "", 0o600},
		{filepath.Join(env.ConfigDir(), "Settings.yaml"), "# Universe-scoped Settings.yaml\n", 0o644},
	} {
		if _, err := os.Stat(f.path); err == nil {
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.body), f.mode); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Base(f.path), err)
		}
	}
	return nil
}

// ToUniverseSummary converts a Manifest + slug (from the registry) + protected
// flag into an api.UniverseSummary.
//
// `slug` is the registry key (catalog.RegistryEntry.Name); `m.Name` is what
// Univers.yaml stores (in current code they're often the same string, but the
// split lets a workspace get relocated while the slug stays stable).
func (m *Manifest) ToUniverseSummary(slug string, workspaceHome string, protected bool) api.UniverseSummary {
	return api.UniverseSummary{
		Version:     m.Version,
		Name:        slug,
		DisplayName: m.DisplayName,
		Description: m.Description,
		Icon:        m.Icon,
		Color:       m.Color,
		HomePath:    workspaceHome,
		Protected:   protected,
		GUI:         m.GUI,
		Tags:        m.Tags,
		FirstGuide:  m.FirstGuide,
		Solution:    m.Solution,
		NoWelcome:   m.NoWelcome,
	}
}

// ToUniverseInfo wraps a Manifest into an api.UniverseInfo with the
// registry-derived `slug` + optional layered-home paths. Sidecars come from
// the Manifest's own Sidecars field (= Univers.yaml's `sidecars:` block).
func (m *Manifest) ToUniverseInfo(slug string, workspaceHome string, protected bool, layeredHomes []string) api.UniverseInfo {
	// Never hand back a NIL slice: Go marshals nil as JSON `null`, but the method's
	// schema declares `sidecars` a required ARRAY. A universe with no sidecars would
	// otherwise send null and break any client that trusts the schema — which is
	// exactly what it did to the GUI edit form (`null.map is not a function`).
	// Empty means empty; it does not mean absent.
	sidecars := m.Sidecars
	if sidecars == nil {
		sidecars = []api.SidecarSpec{}
	}
	if layeredHomes == nil {
		layeredHomes = []string{}
	}
	return api.UniverseInfo{
		Summary:      m.ToUniverseSummary(slug, workspaceHome, protected),
		Sidecars:     sidecars,
		LayeredHomes: layeredHomes,
	}
}
