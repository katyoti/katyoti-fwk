// SPDX-License-Identifier: Apache-2.0

package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// A config-set is a THIRD store beside binaries and content. These pin that it is
// genuinely separate (so listing/pruning config-sets never walks runnable items)
// and that "installed" is keyed on the keys.yaml manifest, not on the folder
// merely existing — a half-extracted bundle must not read as installed.

func setupCfgRoot(t *testing.T) {
	t.Helper()
	t.Setenv("KTW_KATYOTI_ROOT", t.TempDir())
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)
}

func writeManifest(t *testing.T, folder string) {
	t.Helper()
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, RequiredKeysFile), []byte("apiVersion: 1"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigSetStore_PathsAreSeparate(t *testing.T) {
	setupCfgRoot(t)

	cfg, err := ConfigSetRoot()
	if err != nil {
		t.Fatal(err)
	}
	bin, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	content, err := ContentRoot()
	if err != nil {
		t.Fatal(err)
	}
	if cfg == bin || cfg == content {
		t.Fatalf("config-set store must be distinct: cfg=%q bin=%q content=%q", cfg, bin, content)
	}
	if filepath.Base(cfg) != "store-config" {
		t.Errorf("ConfigSetRoot = %q, want it to end in store-config", cfg)
	}

	folder, err := ConfigSetFolderPath("acme-base", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(cfg, "acme-base", "1.0.0"); folder != want {
		t.Errorf("ConfigSetFolderPath = %q, want %q", folder, want)
	}
}

func TestHasConfigSet_RequiresManifest(t *testing.T) {
	setupCfgRoot(t)
	folder, err := ConfigSetFolderPath("acme-base", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}

	if HasConfigSet("acme-base", "1.0.0") {
		t.Fatal("nothing installed yet")
	}

	// A folder with templates but NO manifest is an incomplete unpack.
	if err := os.MkdirAll(filepath.Join(folder, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if HasConfigSet("acme-base", "1.0.0") {
		t.Error("a bundle without keys.yaml must NOT read as installed")
	}

	writeManifest(t, folder)
	if !HasConfigSet("acme-base", "1.0.0") {
		t.Error("manifest present -> installed")
	}
}

// Removing an item must reach the config-set store too, or an uninstalled
// config-set would linger and still read as installed.
func TestUninstall_ReachesConfigSetStore(t *testing.T) {
	setupCfgRoot(t)
	folder, err := ConfigSetFolderPath("acme-base", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	writeManifest(t, folder)

	if !HasAnyVersion("acme-base", "1.0.0") {
		t.Fatal("HasAnyVersion must see a config-set version")
	}
	if err := UninstallVersion("acme-base", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if HasConfigSet("acme-base", "1.0.0") {
		t.Error("UninstallVersion must remove the config-set version")
	}
}
