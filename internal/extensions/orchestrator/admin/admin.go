// SPDX-License-Identifier: Apache-2.0

// Package admin owns the bootstrap of the protected "admin" universe — the
// control-plane universe that hosts the universe-registry + marketplace slots
// ((modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)).
//
// It is the SINGLE home for admin-ensure logic. Two callers share it:
//   - the ktw-univers core (Service construction) calls Ensure to self-heal the
//     admin universe on every sidecar boot;
//   - the katyoti launcher calls EnsureDefault BEFORE spawning ktw-gui — the
//     pre-hub bootstrap that guarantees admin exists on disk + in the registry
//     so the GUI can activate it to reach the management services.
//
// The GUI no longer bootstraps admin itself: it is a pure KTSP client
// ((modules/univers/rules/R-001-the-hub-owns-management-state-clients-are-pure.cwm.md)). Being an exported, non-internal package
// keeps it reachable from app/katyoti (the repo-root internal/ rule permits it;
// the orchestrator's own internal/core does not).
package admin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// UniverseName is the reserved name of the control-plane universe
// (controlplane.AdminUniverse).
const UniverseName = controlplane.AdminUniverse

// Home returns the admin universe home path (controlplane.AdminHome).
func Home() string { return controlplane.AdminHome() }

// EnsureDefault resolves the default registry + admin home and ensures the admin
// universe exists. This is the launcher-facing pre-hub bootstrap entrypoint —
// direct, in-process, no hub spawn (the launcher owns control-plane bootstrap).
func EnsureDefault() error {
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		return fmt.Errorf("resolve registry path: %w", err)
	}
	registry, err := catalog.LoadRegistry(regPath)
	if err != nil {
		return fmt.Errorf("load registry: %w", err)
	}
	return Ensure(registry, Home())
}

// Ensure guarantees the admin universe exists in both the registry and on disk.
// Self-healing: re-creates whichever half is missing.
//
// Four-quadrant matrix:
//   - both present:        no-op
//   - folder absent:       recreate folder skeleton + manifest
//   - registry absent:     add registry entry (preserving any existing folder content)
//   - both absent:         create folder skeleton + manifest + registry entry
//
// homePath is the absolute admin home (production callers pass Home(); tests pass
// a TempDir-rooted path).
//
// See modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md.
func Ensure(registry *catalog.Registry, homePath string) error {
	// Ensure folder + manifest are in place. Always check; recreate if absent.
	manifestPath := filepath.Join(application.EnvOf(homePath).ConfigDir(), "Univers.yaml")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		if err := WriteFolderSkeleton(homePath); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("stat admin manifest: %w", err)
	} else if err := ensureUniversSidecar(homePath); err != nil {
		return err
	}

	// Ensure registry entry. If missing, add it (Add persists to disk).
	if _, err := registry.Find("admin"); err != nil {
		if err := registry.Add(catalog.RegistryEntry{
			Name:      "admin",
			HomePath:  homePath,
			Protected: true,
		}); err != nil {
			return fmt.Errorf("register admin in catalog: %w", err)
		}
	}

	return nil
}

// universSidecar is the one sidecar admin cannot live without: it serves the
// universe-registry + marketplace slots the whole control plane is reached
// through. Kept as the single definition used by both create and repair.
// Transport is "nats", not "stdio". This is the canonical bus-split convention
// (DO.3) every other wiring path already follows — orchestrator CreateUniverse,
// AddSidecar and wire.Into all emit nats. Admin had been the lone holdout, and
// for admin it is not a detail: a stdio sidecar is spawned on demand over pipes
// and never publishes its slots on the bus, while the GUI reaches management
// services ONLY as a KTSP client over NATS (management-plane-hub-authority).
// A stdio admin therefore comes up "healthy" with no management slots at all —
// the GUI shows an empty marketplace and no universe surface, with nothing in
// the log to say why.
func universSidecar() api.SidecarSpec {
	return api.SidecarSpec{
		Name:      "ktw-univers",
		Transport: "nats",
		Binary:    resolveUniversBinary(),
		Args:      []string{"sc", "ktsp", "nats"},
		ConfigDir: filepath.Join("sidecars", "ktw-univers"),
	}
}

// resolveUniversBinary returns an ABSOLUTE path to the ktw-univers executable.
//
// A bare command name does not work here even though exec.Command would resolve
// it: the hub stats spec.Binary on its spawn path ("cache check: stat binary")
// before ever exec'ing, and a bare name fails that stat. Admin then came up with
// no management slots and the only trace was one hub log line.
//
// The control-plane location is the ANSWER, not a preference: ktw-univers is an
// infra binary (controlplane.InfraBinaries), installed and updated as one set
// with katyoti + ktw-hub + ktw-gui. Resolving it there keeps a KTW_KATYOTI_ROOT
// instance genuinely self-contained — a PATH lookup would let it borrow a
// binary from outside its own root, which is how a stale copy from a dev
// install ended up serving a supposedly isolated instance.
//
// The fallbacks exist for the dev loop only, where the control plane is a
// `make build` tree rather than an installed one.
func resolveUniversBinary() string {
	if p, err := controlplane.TargetPath("ktw-univers"); err == nil {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	name := "ktw-univers"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	// Dev loop: the sibling of the running executable — katyoti and ktw-univers
	// are built into the same bin/, so the pair stays matched.
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(sibling); err == nil {
			return sibling
		}
	}
	if p, err := exec.LookPath("ktw-univers"); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			return abs
		}
		return p
	}
	// Nothing found: emit the bare name rather than a wrong absolute path. The
	// spawn still fails, but the manifest stays honest and a later repair (once
	// install-infra has run) resolves it properly.
	return "ktw-univers"
}

// ensureUniversSidecar repairs an admin manifest that EXISTS but has lost
// ktw-univers from its sidecar list.
//
// The file-absent branch above is not enough. A manifest can survive on disk yet
// no longer wire the control plane — that is exactly what a release bootstrap
// aimed at admin did: it rewrote Univers.yaml with the release's own members,
// dropping ktw-univers. The manifest still existed, so Ensure saw nothing to do,
// and every subsequent boot came up with a hub serving no management slots. The
// control plane was permanently dead with no recovery path short of hand-editing
// YAML.
//
// A PRESENT entry on the wrong transport is repaired too. An admin ktw-univers
// wired over stdio is as dead as a missing one: it is spawned on demand over
// pipes and never publishes its slots on the bus, so the GUI — a pure KTSP
// client over NATS — sees no management surface. Both halves of that failure
// look identical from the outside, so both are healed here.
//
// Admin is infrastructure, not a user universe: ktw-univers is not a preference
// anyone may drop, so putting it back is always correct. Other sidecars are left
// untouched — only the ktw-univers entry is appended or corrected.
func ensureUniversSidecar(homePath string) error {
	name := filepath.Base(homePath)
	m, err := catalog.LoadManifestOrRepair(homePath, name)
	if err != nil {
		return fmt.Errorf("load admin manifest: %w", err)
	}
	want := universSidecar()
	for i, sc := range m.Sidecars {
		if sc.Name != want.Name {
			continue
		}
		// A Binary the hub cannot stat is as fatal as a wrong transport — the
		// spawn dies on the cache check. Treat "does not resolve to a file" as
		// broken and re-resolve it.
		binaryOK := false
		if sc.Binary != "" {
			if _, err := os.Stat(sc.Binary); err == nil {
				binaryOK = true
			}
		}
		// A Binary equal to what resolution answers now has nothing better to
		// be replaced by: on a machine with no ktw-univers installed both are
		// the bare name, and rewriting it would rewrite Univers.yaml on every
		// Ensure for the same content.
		if sc.Transport == want.Transport && (binaryOK || sc.Binary == want.Binary) {
			return nil
		}
		fixed := want
		// Keep a Binary that actually resolves: an operator may deliberately
		// point admin at a specific build.
		if binaryOK {
			fixed.Binary = sc.Binary
		}
		m.Sidecars[i] = fixed
		if err := catalog.SaveManifest(homePath, m); err != nil {
			return fmt.Errorf("repair admin manifest (ktw-univers wiring): %w", err)
		}
		return nil
	}
	m.Sidecars = append(m.Sidecars, want)
	if err := catalog.SaveManifest(homePath, m); err != nil {
		return fmt.Errorf("repair admin manifest (restore ktw-univers): %w", err)
	}
	return nil
}

// WriteFolderSkeleton writes the admin workspace's folder structure + minimal
// Univers.yaml + empty .env / Settings.yaml under .katyoti/. Admin ships with
// ktw-univers as a sidecar so the workspace's KTSP catalog can expose
// universe-management slots over the bus.
//
// Sidecars live inside Univers.yaml, in its sidecars: block.
func WriteFolderSkeleton(homePath string) error {
	env := application.EnvOf(homePath)
	katyoti := env.HomeDir()
	configDir := env.ConfigDir()
	// User-facing subdirs (skills/tasks/sync) stay at the workspace root by
	// convention — they're authored content, not katyoti-private config.
	for _, sub := range []string{"skills", "tasks", "sync"} {
		if err := os.MkdirAll(filepath.Join(homePath, sub), 0o755); err != nil {
			return fmt.Errorf("create %s/%s: %w", homePath, sub, err)
		}
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", configDir, err)
	}
	manifest := &catalog.Manifest{
		Version:     1,
		Name:        "admin",
		DisplayName: "Admin",
		Description: "Universe management — auto-created and protected from delete.",
		Icon:        "A",
		Color:       "gray",
		Sidecars:    []api.SidecarSpec{universSidecar()},
	}
	if err := catalog.SaveManifest(homePath, manifest); err != nil {
		return err
	}
	// 0o600: a .env holds secrets.
	if err := os.WriteFile(filepath.Join(katyoti, ".env"), []byte{}, 0o600); err != nil {
		return fmt.Errorf("write admin .env: %w", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "Settings.yaml"), []byte("# Admin universe Settings.yaml\n"), 0o644); err != nil {
		return fmt.Errorf("write admin Settings.yaml: %w", err)
	}
	return nil
}
