// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// : after make install, a universe of the installation runs the
// installation's own programs - a member pinned to a checkout's bin/ is moved
// to the same-named program of the control plane or bin/dev; a member already
// in the installation, or whose program the installation lacks, is kept.
func TestAnInstallationsUniversesAreRepinnedToItsOwnPrograms(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "katyoti")
	home := filepath.Join(base, "dev-universe")
	t.Setenv("KTW_KATYOTI_ROOT", root)
	// The process's own workspace is a universe the repin visits too: the
	// test's, never the checkout's.
	t.Setenv("KTW_WORKSPACE", home)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := cenv.AdoptRoot(root, "katyoti-dev", filepath.Join(base, "cache")); err != nil {
		t.Fatal(err)
	}
	exe := func(dir, name string) string {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		return filepath.Join(dir, name)
	}
	place := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	checkout := filepath.Join(base, "checkout", "bin")
	store := exe(filepath.Join(root, "bin", "store", "ktw-kb", "1.0.0"), "ktw-kb")
	for _, n := range []string{"ktw-agents", "ktw-univers", "ktw-gone"} {
		place(exe(checkout, n))
	}
	place(store)
	agents, _ := controlplane.ProgramPath("ktw-agents")
	univers, _ := controlplane.ProgramPath("ktw-univers")
	place(agents)
	place(univers)

	m := &catalog.Manifest{Version: 1, Name: "dev", Sidecars: []api.SidecarSpec{
		{Name: "ktw-agents", Transport: "nats", Binary: exe(checkout, "ktw-agents"), Args: []string{"sc", "ktsp", "nats"}},
		{Name: "ktw-univers", Transport: "nats", Binary: exe(checkout, "ktw-univers")},
		{Name: "ktw-gone", Transport: "nats", Binary: exe(checkout, "ktw-gone")},
		{Name: "ktw-kb", Transport: "nats", Binary: store},
	}}
	if err := catalog.SaveManifest(home, m); err != nil {
		t.Fatal(err)
	}
	regPath, _ := catalog.DefaultRegistryPath()
	reg, err := catalog.LoadRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.Add(catalog.RegistryEntry{Name: "dev", HomePath: home}); err != nil {
		t.Fatal(err)
	}
	pins := func() map[string]string {
		got, err := catalog.LoadManifest(home)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, sc := range got.Sidecars {
			out[sc.Name] = sc.Binary
		}
		return out
	}

	moved, missing, err := RepinToPrograms(controlplane.ProgramPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 2 || len(missing) != 1 || missing[0].Member != "ktw-gone" {
		t.Fatalf("dry run: moved %+v, missing %+v", moved, missing)
	}
	if pins()["ktw-agents"] != exe(checkout, "ktw-agents") {
		t.Fatal("the dry run moved a member")
	}

	if _, _, err := RepinToPrograms(controlplane.ProgramPath, false); err != nil {
		t.Fatal(err)
	}
	got := pins()
	for name, want := range map[string]string{"ktw-agents": agents, "ktw-univers": univers,
		"ktw-gone": exe(checkout, "ktw-gone"), "ktw-kb": store} {
		if got[name] != want {
			t.Errorf("%s pins %s, want %s", name, got[name], want)
		}
	}
	if filepath.Dir(univers) != filepath.Join(root, "control-plane") || filepath.Dir(agents) != filepath.Join(root, "bin", "dev") {
		t.Errorf("programs: %s, %s", univers, agents)
	}
	if after, _ := catalog.LoadManifest(home); len(after.Sidecars[0].Args) != 3 {
		t.Errorf("the repin lost the member's args: %+v", after.Sidecars[0])
	}
}
