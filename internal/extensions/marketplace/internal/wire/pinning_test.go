// SPDX-License-Identifier: Apache-2.0

package wire

import (
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// A universe running a store version pins it — the version folder is in the
// path of its binary (or of the folder a content host is given) — and only that
// version: per-version uninstall refuses exactly the pinned one.
func TestAUniversePinsTheVersionItRuns(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	home := filepath.Join(t.TempDir(), "solution-uni")
	store := filepath.Join(root, "bin", "store")
	m := &catalog.Manifest{Version: 1, Name: "solution-uni", Sidecars: []api.SidecarSpec{
		{Name: "ktw-weather", Transport: "nats", Binary: filepath.Join(store, "ktw-weather", "1.0.0", "ktw-weather.exe")},
		{Name: "flowpack", Transport: "nats", Binary: filepath.Join(store, "ktw-host", "2.0.0", "ktw-host.exe"),
			Args: []string{"--folder", filepath.Join(root, "bin", "store-content", "flowpack", "3.0.0")}},
	}}
	if err := catalog.SaveManifest(home, m); err != nil {
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
	if err := reg.Add(catalog.RegistryEntry{Name: "solution-uni", HomePath: home}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name, version string
		pinned        bool
	}{
		{"ktw-weather", "1.0.0", true},
		{"ktw-weather", "1.1.0", false},
		{"flowpack", "3.0.0", true},
		{"flowpack", "2.0.0", false},
		// ktw-host is no member, yet the content member runs its 2.0.0.
		{"ktw-host", "2.0.0", true},
		{"ktw-host", "2.1.0", false},
	} {
		if got := len(UniversesPinning(c.name, c.version)) > 0; got != c.pinned {
			t.Errorf("%s@%s pinned = %v, want %v", c.name, c.version, got, c.pinned)
		}
	}
	if len(UniversesHosting("ktw-host")) != 1 {
		t.Errorf("the universe running content on the store's ktw-host should be found")
	}
	if len(UniversesHosting("ktw-notify")) != 0 {
		t.Errorf("a universe running no ktw-notify was found running it")
	}
}
