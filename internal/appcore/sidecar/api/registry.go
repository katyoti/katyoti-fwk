// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	"gopkg.in/yaml.v3"
)

// This file holds the sidecar-membership DTOs — the shapes that describe what a
// sidecar IS and how it is activated. They live here rather than in appcore
// because they are consumed by packages that have no business linking the
// application framework: the orchestrator and marketplace APIs, and the remote
// clients built on them.
//
// The behaviour that READS and WRITES these shapes (path resolution, load, save)
// stays in appcore, where it belongs — it needs the resolved environment.

// UniverseSidecars is the canonical in-memory shape of a workspace's sidecar
// membership. Sidecars live as the `sidecars:` block inside
// `<HomeDir>/config/Univers.yaml`, alongside universe identity; this type is the
// runtime projection that callers (the hub's connection manager, sidecar
// discovery, GUI writers) consume.
//
// The full Univers.yaml schema is owned by the catalog package; the loader reads
// and writes ONLY the `sidecars:` block, preserving everything else on save.
type UniverseSidecars struct {
	Version     int           `yaml:"version,omitempty"`
	Sidecars    []SidecarSpec `yaml:"sidecars,omitempty"`
	GeneratedAt time.Time     `yaml:"generated_at,omitempty"`
}

// SidecarSpec describes how to activate one sidecar within a workspace.
type SidecarSpec struct {
	Name      string            `yaml:"name"                 json:"name"`
	Transport string            `yaml:"transport"            json:"transport"`
	Binary    string            `yaml:"binary,omitempty"     json:"binary,omitempty"`
	Args      []string          `yaml:"args,omitempty"       json:"args,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"        json:"env,omitempty"`
	ConfigDir string            `yaml:"configdir,omitempty"  json:"configDir,omitempty"`

	// ContentRoot is the folder a generic host serves as this sidecar.
	//
	// DECLARED here rather than recovered from Args. The folder used to exist
	// only as `--folder <path>` inside the spawn arguments, so anything that
	// needed it — the version resolver, the configure path, the host itself —
	// re-parsed argv to find it, and the aggregator could not cheaply know a
	// member served a folder at all. That matters because a served folder is
	// the one source whose changes the binary's own stat cannot show: the host
	// executable is identical whatever the folder holds.
	//
	// Empty for a compiled sidecar, which carries its content.
	ContentRoot string            `yaml:"content_root,omitempty" json:"contentRoot,omitempty"`
	HealthzURL  string            `yaml:"healthzurl,omitempty" json:"healthzUrl,omitempty"`
	Manifest    *EndpointManifest `yaml:"manifest,omitempty"   json:"manifest,omitempty"`

	IdleTimeout time.Duration `yaml:"idle_timeout,omitempty" json:"idleTimeout,omitempty"`
	Pin         bool          `yaml:"pin,omitempty"          json:"pin,omitempty"`
	Lifecycle   string        `yaml:"lifecycle,omitempty"    json:"lifecycle,omitempty"`

	MaxRestarts      int           `yaml:"max_restarts,omitempty"        json:"maxRestarts,omitempty"`
	MaxRestartWindow time.Duration `yaml:"max_restart_window,omitempty"  json:"maxRestartWindow,omitempty"`
	GracefulShutdown time.Duration `yaml:"graceful_shutdown,omitempty"   json:"gracefulShutdown,omitempty"`
	// ReadyTimeout is how long the hub waits for a started member to answer
	// its readiness ping before the start counts as failed. Zero keeps the
	// default (connmgr's DefaultReadyTimeout, 30 s).
	ReadyTimeout time.Duration `yaml:"ready_timeout,omitempty"       json:"readyTimeout,omitempty"`

	// Grants is what the universe grants this sidecar beyond what it owns —
	// today, where its gate entries may refuse writes (cenv.SidecarGrants).
	// Carried here so a save of the membership keeps it.
	Grants *cenv.SidecarGrants `yaml:"grants,omitempty" json:"grants,omitempty"`
}

// IsFolder reports whether the member is a FOLDER member: a content root the
// hub reads itself, with no process to start — declared with no binary, or
// with the generic host (ktw-host) for a folder that has no scripts/ (a
// folder's scripts need the host's process). Any other binary with a content
// root runs it (a producer in another language reads its folder itself).
func (s SidecarSpec) IsFolder() bool {
	if s.ContentRoot == "" {
		return false
	}
	if s.Binary == "" {
		return true
	}
	return isGenericHost(s.Binary) && !hasScripts(s.ContentRoot)
}

// isGenericHost says whether a binary is the generic host, whatever its
// folder or extension.
func isGenericHost(binary string) bool {
	base := filepath.Base(binary)
	return strings.EqualFold(strings.TrimSuffix(base, filepath.Ext(base)), "ktw-host")
}

// hasScripts says whether a content root ships scripts (a scripts/ folder).
func hasScripts(root string) bool {
	fi, err := os.Stat(filepath.Join(root, "scripts"))
	return err == nil && fi.IsDir()
}

// Validate says what makes the member unreachable: it names a binary to run,
// a folder the hub reads, or the remote transport (already on the bus).
func (s SidecarSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("a member has no name")
	}
	if s.Binary == "" && s.ContentRoot == "" && s.Transport != "remote" {
		return fmt.Errorf("member %s names neither a binary nor a content_root", s.Name)
	}
	return nil
}

// FindByName returns the spec for the named sidecar, or nil if not present.
func (r *UniverseSidecars) FindByName(name string) *SidecarSpec {
	for i := range r.Sidecars {
		if r.Sidecars[i].Name == name {
			return &r.Sidecars[i]
		}
	}
	return nil
}

// EndpointManifest is the top-level structure returned by `ktsp describe --json`.
// It wraps the KTSP ContainerInfo with sidecar identity and capabilities.
type EndpointManifest struct {
	Name         string               `json:"name"                  yaml:"name"`
	Version      string               `json:"version"               yaml:"version"`
	Description  string               `json:"description,omitempty" yaml:"description,omitempty"`
	Transports   []string             `json:"transports"            yaml:"transports"`
	Capabilities ManifestCapabilities `json:"capabilities"          yaml:"capabilities"`
	KTSP         ktsapi.ContainerInfo `json:"ktsp"                  yaml:"ktsp"`
}

// ManifestCapabilities describes what transport/integration capabilities the sidecar has.
type ManifestCapabilities struct {
	CLI  bool `json:"cli"`
	KTSP bool `json:"ktsp"`
	HTTP bool `json:"http"`
}

// SaveTo writes the registry's sidecars block into Univers.yaml at `path`,
// preserving every other field of the existing file. If the file doesn't
// exist, a new one is created with ONLY `sidecars:` populated — callers who
// need the full identity must use `catalog.SaveManifest` instead.
func (r *UniverseSidecars) SaveTo(path string) error {
	if r.Version == 0 {
		r.Version = 1
	}
	r.GeneratedAt = time.Now().UTC()

	// Read existing file (if any) so we can merge the sidecars block in
	// without clobbering identity fields.
	var rootMap map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &rootMap)
	}
	if rootMap == nil {
		rootMap = map[string]any{}
	}

	// Marshal-then-unmarshal the sidecars list to convert []SidecarSpec into
	// the generic map/slice form yaml.Marshal will emit consistently.
	encoded, err := yaml.Marshal(r.Sidecars)
	if err != nil {
		return fmt.Errorf("encode sidecars list: %w", err)
	}
	var sidecarsGeneric any
	if err := yaml.Unmarshal(encoded, &sidecarsGeneric); err != nil {
		return fmt.Errorf("re-decode sidecars list: %w", err)
	}
	rootMap["sidecars"] = sidecarsGeneric

	out, err := yaml.Marshal(rootMap)
	if err != nil {
		return fmt.Errorf("encode Univers.yaml: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Univers.yaml dir: %w", err)
	}
	return os.WriteFile(path, out, 0o644)
}

// PinnedVersion is the version a member is wired at, read from where it is
// wired: its path in the versioned store. A content member's folder
// (`…/store-content/<name>/<version>`), else its `--folder` argument, else the
// folder of an absolute binary (`…/store/<name>/<version>/<name>[.exe]`). Empty
// when it is not wired from the store (a dev or PATH binary).
func (s SidecarSpec) PinnedVersion() string {
	// The DECLARED root first; argv remains only for membership written
	// before content_root existed.
	if s.ContentRoot != "" {
		return storeVersion(s.ContentRoot, s.Name)
	}
	for i, a := range s.Args {
		if a == "--folder" && i+1 < len(s.Args) {
			return storeVersion(s.Args[i+1], s.Name)
		}
	}
	if s.Binary != "" && filepath.IsAbs(s.Binary) {
		return storeVersion(filepath.Dir(s.Binary), s.Name)
	}
	return ""
}

// storeVersion reads the version out of a versioned store path,
// `<…>/store[-content]/<name>/<version>`, and returns "" for anything else:
// the version is the last segment only when the one before it is the member's
// own name (a dev binary at `…/bin/ktw-notify.exe` is pinned to nothing, not to
// "bin").
func storeVersion(dir, name string) string {
	dir = filepath.Clean(dir)
	if filepath.Base(filepath.Dir(dir)) != name {
		return ""
	}
	return filepath.Base(dir)
}
