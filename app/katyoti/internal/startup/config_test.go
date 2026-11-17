// SPDX-License-Identifier: Apache-2.0

package startup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TestStartupConfig drives the whole KatyotiConfig surface against a temp global
// root: first-run defaults (via ProcessConfig, no file), the global-only path,
// a set→save→reload round-trip, field preservation across sets, and validation.
func TestStartupConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	// path resolves under the GLOBAL config layer: <root>/config/Katyoti.yaml
	want := filepath.Join(root, "config", "Katyoti.yaml")
	if Path() != want {
		t.Fatalf("Path() = %q, want %q", Path(), want)
	}

	// first run: no file yet, defaults materialize
	if _, err := os.Stat(Path()); !os.IsNotExist(err) {
		t.Fatalf("no file should exist on first run")
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LaunchMode != LaunchGUI {
		t.Fatalf("default launch_mode = %q, want %q", cfg.LaunchMode, LaunchGUI)
	}

	// set → save → reload
	if _, err := Set("launch_mode", LaunchCLI); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(Path()); err != nil {
		t.Fatalf("file should be written after set: %v", err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LaunchMode != LaunchCLI {
		t.Fatalf("reloaded launch_mode = %q, want %q", cfg.LaunchMode, LaunchCLI)
	}

	// a second set preserves the earlier field
	if _, err := Set("default_universe", "acme"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultUniverse != "acme" {
		t.Fatalf("default_universe not persisted: %+v", cfg)
	}
	if cfg.LaunchMode != LaunchCLI {
		t.Fatalf("launch_mode lost across a second set: %+v", cfg)
	}

	// validation: bad mode rejected on both Set and Save
	if _, err := Set("launch_mode", "banana"); err == nil {
		t.Fatal("bad launch_mode should be rejected by Set")
	}
	if err := Save(KatyotiConfig{LaunchMode: "banana"}); err == nil {
		t.Fatal("bad launch_mode should be rejected by Save")
	}
	// unknown key rejected
	if _, err := Set("nope", "x"); err == nil {
		t.Fatal("unknown key should be rejected")
	}
}
