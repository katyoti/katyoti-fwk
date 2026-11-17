// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// (and RQ-062): when a sidecar's installed version moves, every
// universe running another store version of it is repinned to the installed
// one - an update never leaves a universe on a program the store may no longer
// hold. A member running a program outside the store (a development build) is
// left alone; a universe installed from a solution keeps a pinned version that
// still exists (reported behind) and is moved off one that is gone.
func TestAUniverseOnAnotherStoreVersionIsRepinnedToTheInstalledOne(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "katyoti")
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("KTW_WORKSPACE", filepath.Join(base, "no-workspace"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	exe := func(version string) string {
		name := "ktw-methods"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(root, "bin", "store", "ktw-methods", version, name)
	}
	place := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	place(exe("v2"))
	place(exe("v1")) // still in the store
	dev := filepath.Join(base, "checkout", "bin", "ktw-methods.exe")
	place(dev)

	universes := map[string]struct {
		solution, binary string
	}{
		"plain":        {"", exe("v0-gone")},  // a version deleted by hand
		"old":          {"", exe("v1")},       // an older version still there
		"released":     {"demo@1", exe("v1")}, // a solution's pin, still there
		"released-bad": {"demo@1", exe("v0-gone")},
		"dev":          {"", dev}, // the developer's build
		"current":      {"", exe("v2")},
	}
	regPath, err := catalog.DefaultRegistryPath()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	homes := map[string]string{}
	for name, u := range universes {
		home := filepath.Join(base, "universes", name)
		homes[name] = home
		m := &catalog.Manifest{Version: 1, Name: name, Solution: u.solution, Sidecars: []api.SidecarSpec{
			{Name: "ktw-methods", Transport: "nats", Binary: u.binary, Args: []string{"sc", "ktsp", "nats"}},
		}}
		if err := catalog.SaveManifest(home, m); err != nil {
			t.Fatal(err)
		}
		if err := reg.Add(catalog.RegistryEntry{Name: name, HomePath: home}); err != nil {
			t.Fatal(err)
		}
	}

	pins := PinsByItem()["ktw-methods"]
	if len(pins) != len(universes) {
		t.Fatalf("pins %+v: want one per universe", pins)
	}
	for _, p := range pins {
		want := map[string]string{"plain": "v0-gone", "old": "v1", "released": "v1", "released-bad": "v0-gone", "dev": "", "current": "v2"}[p.Universe]
		if p.Version != want {
			t.Errorf("%s pins %q, want %q", p.Universe, p.Version, want)
		}
	}

	ins := mktapi.Installed{Name: "ktw-methods", Kind: "sidecar", Version: "v2", BinaryPath: exe("v2")}
	moved, behind, err := RepinToInstalled(ins)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range moved {
		got[r.Universe.Name] = true
	}
	for _, name := range []string{"plain", "old", "released-bad"} {
		if !got[name] {
			t.Errorf("%s was not repinned (moved %+v)", name, moved)
		}
	}
	if len(moved) != 3 {
		t.Errorf("moved %+v: want plain, old and released-bad only", moved)
	}
	if len(behind) != 1 || behind[0].Name != "released" {
		t.Errorf("behind %+v: want the solution's universe whose pin still exists", behind)
	}
	for name, want := range map[string]string{
		"plain": exe("v2"), "old": exe("v2"), "released-bad": exe("v2"),
		"released": exe("v1"), "dev": dev, "current": exe("v2"),
	} {
		m, err := catalog.LoadManifest(homes[name])
		if err != nil {
			t.Fatal(err)
		}
		if b := m.Sidecars[0].Binary; b != want {
			t.Errorf("%s runs %s, want %s", name, b, want)
		}
	}
}

// A repin moves only what the member runs - its program, its folder, its
// arguments and the content root it is told - and keeps everything else its
// universe gave it: config dir, env, grants, lifecycle, timeouts, restarts.
func TestARepinKeepsWhatTheUniverseGaveTheMember(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "katyoti")
	t.Setenv("KTW_KATYOTI_ROOT", root)
	t.Setenv("KTW_DATA", filepath.Join(base, "data"))
	t.Setenv("KTW_CACHE", filepath.Join(base, "cache"))
	t.Setenv("KTW_WORKSPACE", filepath.Join(base, "no-workspace"))
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	bin := func(v string) string {
		return filepath.Join(root, "bin", "store", "ktw-sync", v, "ktw-sync.exe")
	}
	if err := os.MkdirAll(filepath.Dir(bin("v2")), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin("v2"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	given := api.SidecarSpec{
		Name: "ktw-sync", Transport: "nats", Binary: bin("v1"), Args: []string{"sc", "ktsp", "nats"},
		Env: map[string]string{"KTW_SYNC_MODE": "push"}, ConfigDir: filepath.Join(base, "sync-config"),
		Lifecycle: "eager", IdleTimeout: 5 * time.Minute, Pin: true, MaxRestarts: 3,
		ReadyTimeout: 40 * time.Second, GracefulShutdown: 7 * time.Second,
		Grants: &cenv.SidecarGrants{Gate: cenv.GateGrant{Types: []string{"*"}}},
	}
	home := filepath.Join(base, "universes", "kept")
	if err := catalog.SaveManifest(home, &catalog.Manifest{Version: 1, Name: "kept", Sidecars: []api.SidecarSpec{given}}); err != nil {
		t.Fatal(err)
	}
	regPath, _ := catalog.DefaultRegistryPath()
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(catalog.RegistryEntry{Name: "kept", HomePath: home}); err != nil {
		t.Fatal(err)
	}
	moved, _, err := RepinToInstalled(mktapi.Installed{Name: "ktw-sync", Kind: "sidecar", Version: "v2", BinaryPath: bin("v2")})
	if err != nil || len(moved) != 1 {
		t.Fatalf("moved %+v, err %v: want the one universe", moved, err)
	}
	m, err := catalog.LoadManifest(home)
	if err != nil {
		t.Fatal(err)
	}
	want := given
	want.Binary = bin("v2")
	if got := m.Sidecars[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("after the repin the member is\n%+v\nwant\n%+v", got, want)
	}

	// A content member told its folder by env keeps its other env.
	sc := api.SidecarSpec{Name: "flowpack", Env: map[string]string{"KTW_CONTENT_ROOT": "old", "LANG": "fr"}, ConfigDir: "c"}
	repoint(&sc, api.SidecarSpec{Binary: "host", ContentRoot: "new", Args: []string{"--folder", "new"}, Env: map[string]string{"KTW_CONTENT_ROOT": "new"}})
	if sc.Env["KTW_CONTENT_ROOT"] != "new" || sc.Env["LANG"] != "fr" || sc.ConfigDir != "c" || sc.ContentRoot != "new" {
		t.Errorf("content member after repoint: %+v", sc)
	}
}
