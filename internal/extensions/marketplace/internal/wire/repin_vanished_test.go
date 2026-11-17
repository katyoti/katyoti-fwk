// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// : a universe member pinned to a program that is gone (its
// store folder deleted by hand) is rewired to the installed version of its
// item - only its program moves, what the universe gave it stays; a member
// nothing installed replaces is kept in its universe and reported missing; a
// member whose program is there (a development build, a store version) is
// never touched.
func TestAMemberPinnedToAProgramThatIsGoneIsRewiredToTheInstalledOne(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "katyoti")
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("KTW_WORKSPACE", filepath.Join(base, "no-workspace"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	exe := func(name, version string) string {
		return filepath.Join(root, "bin", "store", name, version, name+".exe")
	}
	place := func(p string) string {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	rc5 := place(exe("ktw-methods", "v2026.10-rc5"))
	dev := place(filepath.Join(root, "bin", "dev", "ktw-agents.exe"))
	old := place(exe("ktw-docs", "v1")) // an older store version still there

	home := filepath.Join(base, "universes", "archifactory")
	members := []api.SidecarSpec{
		{Name: "ktw-agents", Transport: "nats", Binary: dev, Args: []string{"sc", "ktsp", "nats"}, ConfigDir: filepath.Join("sidecars", "ktw-agents")},
		{Name: "ktw-methods", Transport: "nats", Binary: exe("ktw-methods", "v0.2.0-test.5"), Args: []string{"sc", "ktsp", "nats"},
			ConfigDir: filepath.Join("sidecars", "ktw-methods"), Env: map[string]string{"X": "1"}, Lifecycle: "eager"},
		{Name: "ktw-sync", Transport: "nats", Binary: exe("ktw-sync", "v0.2.0-test.5"), Args: []string{"sc", "ktsp", "nats"}},
		{Name: "ktw-docs", Transport: "nats", Binary: old, Args: []string{"sc", "ktsp", "nats"}},
	}
	if err := catalog.SaveManifest(home, &catalog.Manifest{Version: 1, Name: "archifactory", Sidecars: members}); err != nil {
		t.Fatal(err)
	}
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(catalog.RegistryEntry{Name: "archifactory", HomePath: home}); err != nil {
		t.Fatal(err)
	}

	installed := []mktapi.Installed{
		{Name: "ktw-methods", Kind: "sidecar", Version: "v2026.10-rc5", BinaryPath: rc5},
		{Name: "ktw-docs", Kind: "sidecar", Version: "v2", BinaryPath: exe("ktw-docs", "v2")}, // its files gone too: replaces nothing
	}
	moved, missing, err := RepinVanished(installed)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved[0].Member != "ktw-methods" || moved[0].From != "v0.2.0-test.5" {
		t.Fatalf("moved %+v, want ktw-methods off v0.2.0-test.5", moved)
	}
	if len(missing) != 1 || missing[0].Member != "ktw-sync" {
		t.Fatalf("missing %+v, want ktw-sync", missing)
	}

	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Sidecars) != len(members) {
		t.Fatalf("the universe holds %d members, want %d: a member is never dropped", len(m.Sidecars), len(members))
	}
	by := map[string]api.SidecarSpec{}
	for _, sc := range m.Sidecars {
		by[sc.Name] = sc
	}
	if sc := by["ktw-methods"]; sc.Binary != rc5 || sc.ConfigDir != filepath.Join("sidecars", "ktw-methods") || sc.Env["X"] != "1" || sc.Lifecycle != "eager" {
		t.Fatalf("ktw-methods = %+v, want the installed rc5 with its config dir, env and lifecycle kept", sc)
	}
	if sc := by["ktw-sync"]; sc.Binary != exe("ktw-sync", "v0.2.0-test.5") {
		t.Fatalf("ktw-sync = %+v, want kept as it was (nothing replaces it)", sc)
	}
	if by["ktw-agents"].Binary != dev || by["ktw-docs"].Binary != old {
		t.Fatalf("a member whose program is there moved: %+v %+v", by["ktw-agents"], by["ktw-docs"])
	}
}
