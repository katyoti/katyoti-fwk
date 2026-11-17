// SPDX-License-Identifier: Apache-2.0

package admin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// TestEnsure_FreshBoot — no folder, no registry entry → both created.
func TestEnsure_FreshBoot(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	registry, err := catalog.LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("catalog.LoadRegistry: %v", err)
	}

	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Folder exists with manifest + skeleton (post-cutover layout).
	if _, err := os.Stat(filepath.Join(application.EnvOf(adminHome).ConfigDir(), "Univers.yaml")); err != nil {
		t.Fatalf("Univers.yaml not created: %v", err)
	}
	// User-facing subfolders stay at the universe root; config moves under .katyoti/.
	for _, sub := range []string{"skills", "tasks", "sync"} {
		if _, err := os.Stat(filepath.Join(adminHome, sub)); err != nil {
			t.Errorf("subfolder %s not created: %v", sub, err)
		}
	}
	if _, err := os.Stat(application.EnvOf(adminHome).ConfigDir()); err != nil {
		t.Errorf("config dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(adminHome).HomeDir(), ".env")); err != nil {
		t.Errorf(".env not created: %v", err)
	}
	// Registry entry exists with Protected=true.
	entry, err := registry.Find("admin")
	if err != nil {
		t.Fatalf("admin not in registry: %v", err)
	}
	if !entry.Protected {
		t.Errorf("admin.Protected = false, want true")
	}
	if entry.HomePath != adminHome {
		t.Errorf("admin.HomePath = %q, want %q", entry.HomePath, adminHome)
	}

	// Univers.yaml sanity check. Identity + sidecars all live in Univers.yaml.
	m, err := catalog.LoadManifest(adminHome)
	if err != nil {
		t.Fatalf("catalog.LoadManifest: %v", err)
	}
	if m.Name != "admin" {
		t.Errorf("manifest.Name = %q, want admin", m.Name)
	}
	if m.DisplayName != "Admin" {
		t.Errorf("manifest.DisplayName = %q, want Admin", m.DisplayName)
	}
	if len(m.Sidecars) != 1 || m.Sidecars[0].Name != "ktw-univers" {
		t.Errorf("admin manifest sidecars = %+v, want [ktw-univers]", m.Sidecars)
	}
	if m.Sidecars[0].Transport != "nats" {
		t.Errorf("admin ktw-univers transport = %q, want nats", m.Sidecars[0].Transport)
	}
	if m.Color != "gray" {
		t.Errorf("manifest.Color = %q, want gray", m.Color)
	}
	if m.Version != 1 {
		t.Errorf("manifest.Version = %d, want 1", m.Version)
	}
}

// withoutUniversInstalled isolates the control-plane root on a temp folder and
// empties PATH, so no ktw-univers resolves - the state of a fresh machine (a CI
// runner), independent of the person's installation.
func withoutUniversInstalled(t *testing.T) {
	t.Helper()
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	if got := resolveUniversBinary(); got != "ktw-univers" {
		t.Fatalf("resolveUniversBinary = %q, want the bare name with nothing installed", got)
	}
}

// TestEnsure_Idempotent — running twice on a happy state is a no-op.
func TestEnsure_Idempotent(t *testing.T) {
	ensureIsIdempotent(t)
}

// TestEnsure_IdempotentWithoutUnivers — a re-run is a no-op also when no
// ktw-univers is installed: the bare name it wrote is not rewritten.
func TestEnsure_IdempotentWithoutUnivers(t *testing.T) {
	withoutUniversInstalled(t)
	ensureIsIdempotent(t)
}

func ensureIsIdempotent(t *testing.T) {
	t.Helper()
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	registry, _ := catalog.LoadRegistry(registryPath)
	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("first ensure: %v", err)
	}

	// Capture state.
	listBefore := registry.List()
	statBefore, _ := os.Stat(filepath.Join(application.EnvOf(adminHome).ConfigDir(), "Univers.yaml"))

	// Re-run.
	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	// Registry entries unchanged.
	listAfter := registry.List()
	if len(listAfter) != len(listBefore) {
		t.Errorf("registry size changed: %d -> %d", len(listBefore), len(listAfter))
	}

	// Manifest file not rewritten (mtime unchanged).
	statAfter, _ := os.Stat(filepath.Join(application.EnvOf(adminHome).ConfigDir(), "Univers.yaml"))
	if !statBefore.ModTime().Equal(statAfter.ModTime()) {
		t.Errorf("manifest mtime changed on idempotent re-run: %v -> %v", statBefore.ModTime(), statAfter.ModTime())
	}
}

// TestEnsure_SelfHealsFolder — registry has admin entry but folder is missing.
func TestEnsure_SelfHealsFolder(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	registry, _ := catalog.LoadRegistry(registryPath)
	// Manually add admin to the registry without creating the folder.
	if err := registry.Add(catalog.RegistryEntry{Name: "admin", HomePath: adminHome, Protected: true}); err != nil {
		t.Fatalf("registry.Add: %v", err)
	}
	// Sanity: folder absent.
	if _, err := os.Stat(adminHome); !os.IsNotExist(err) {
		t.Fatalf("folder should not exist before ensure: %v", err)
	}

	// Ensure should recreate the folder.
	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(application.EnvOf(adminHome).ConfigDir(), "Univers.yaml")); err != nil {
		t.Fatalf("Univers.yaml not recreated: %v", err)
	}

	// Registry entry still has Protected=true (preserved).
	entry, _ := registry.Find("admin")
	if !entry.Protected {
		t.Errorf("Protected flag should be preserved")
	}
}

// TestEnsure_SelfHealsRegistry — folder exists but registry entry is missing.
func TestEnsure_SelfHealsRegistry(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	// Create folder skeleton manually (without registry entry).
	if err := WriteFolderSkeleton(adminHome); err != nil {
		t.Fatalf("WriteFolderSkeleton: %v", err)
	}

	registry, _ := catalog.LoadRegistry(registryPath)
	// Sanity: admin not in registry yet.
	if _, err := registry.Find("admin"); err == nil {
		t.Fatalf("admin should not be in registry before ensure")
	}

	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Registry entry now exists with Protected=true.
	entry, err := registry.Find("admin")
	if err != nil {
		t.Fatalf("admin not in registry after self-heal: %v", err)
	}
	if !entry.Protected {
		t.Errorf("admin.Protected should be true")
	}
}

// TestEnsure_RepairsLostUniversSidecar — the manifest EXISTS but no longer wires
// ktw-univers. Ensure must put it back.
//
// This is the shape a release bootstrap aimed at admin left behind: Univers.yaml
// rewritten with the release's own members, ktw-univers dropped. Because the file
// still existed, Ensure's file-absent branch saw nothing to do and every later
// boot served no management slots — the control plane was dead with no recovery
// path but hand-editing YAML.
func TestEnsure_RepairsLostUniversSidecar(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	registry, err := catalog.LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("catalog.LoadRegistry: %v", err)
	}
	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure (fresh): %v", err)
	}

	// Clobber it the way the bootstrap did: a valid manifest whose sidecar list
	// holds someone else entirely.
	m, err := catalog.LoadManifestOrRepair(adminHome, "admin")
	if err != nil {
		t.Fatalf("LoadManifestOrRepair: %v", err)
	}
	m.Sidecars = []api.SidecarSpec{{Name: "ktw-notify", Transport: "nats", Binary: "ktw-notify"}}
	if err := catalog.SaveManifest(adminHome, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure (repair): %v", err)
	}

	got, err := catalog.LoadManifestOrRepair(adminHome, "admin")
	if err != nil {
		t.Fatalf("LoadManifestOrRepair after repair: %v", err)
	}
	var haveUnivers, haveNotify bool
	for _, sc := range got.Sidecars {
		switch sc.Name {
		case "ktw-univers":
			haveUnivers = true
			if sc.Transport != "nats" {
				t.Errorf("restored ktw-univers transport = %q, want nats", sc.Transport)
			}
			// The hub stats Binary before exec, so a bare name is unusable.
			if base := filepath.Base(sc.Binary); base != "ktw-univers" && base != "ktw-univers.exe" {
				t.Errorf("restored ktw-univers Binary = %q, want a path to ktw-univers", sc.Binary)
			}
		case "ktw-notify":
			haveNotify = true
		}
	}
	if !haveUnivers {
		t.Error("ktw-univers was not restored — admin still serves no management slots")
	}
	// Repair appends; it must not evict sidecars the user legitimately added.
	if !haveNotify {
		t.Error("repair dropped the unrelated ktw-notify sidecar")
	}
}

// TestEnsure_RepairsWrongUniversTransport — ktw-univers is PRESENT but wired
// over stdio, so it never publishes its slots on the bus and the GUI sees no
// management surface at all. Ensure must correct the transport.
func TestEnsure_RepairsWrongUniversTransport(t *testing.T) {
	tempDir := t.TempDir()
	registryPath := filepath.Join(tempDir, "universes.yaml")
	adminHome := filepath.Join(tempDir, "ktw-universes", "admin")

	registry, err := catalog.LoadRegistry(registryPath)
	if err != nil {
		t.Fatalf("catalog.LoadRegistry: %v", err)
	}
	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure (fresh): %v", err)
	}

	m, err := catalog.LoadManifestOrRepair(adminHome, "admin")
	if err != nil {
		t.Fatalf("LoadManifestOrRepair: %v", err)
	}
	// A Binary that really resolves — the repair must keep an operator's
	// deliberate pin to a specific build and correct only the transport.
	pinned := filepath.Join(tempDir, "pinned-ktw-univers")
	if err := os.WriteFile(pinned, []byte("stub"), 0o755); err != nil {
		t.Fatalf("write pinned binary: %v", err)
	}
	m.Sidecars = []api.SidecarSpec{{
		Name:      "ktw-univers",
		Transport: "stdio",
		Binary:    pinned,
		Args:      []string{"sc", "ktsp", "stdio"},
	}}
	if err := catalog.SaveManifest(adminHome, m); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	if err := Ensure(registry, adminHome); err != nil {
		t.Fatalf("Ensure (repair): %v", err)
	}

	got, err := catalog.LoadManifestOrRepair(adminHome, "admin")
	if err != nil {
		t.Fatalf("LoadManifestOrRepair after repair: %v", err)
	}
	if len(got.Sidecars) != 1 {
		t.Fatalf("sidecars = %+v, want exactly one (repair must not duplicate)", got.Sidecars)
	}
	sc := got.Sidecars[0]
	if sc.Transport != "nats" {
		t.Errorf("transport = %q, want nats", sc.Transport)
	}
	if len(sc.Args) == 0 || sc.Args[len(sc.Args)-1] != "nats" {
		t.Errorf("args = %v, want the nats subcommand", sc.Args)
	}
	// An absolute path that resolves must survive the repair.
	if sc.Binary != pinned {
		t.Errorf("Binary = %q, want the pinned path %q preserved", sc.Binary, pinned)
	}
}
