// SPDX-License-Identifier: Apache-2.0

package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// TestGuiSettings covers the ConfigService-backed migration: first-run defaults
// (no file), overlay-on-defaults for a PARTIAL file, the stable filename, and a
// Save→Load round-trip.
func TestGuiSettings(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	// filename stays `config/GuiSettings.yaml`
	want := filepath.Join(root, "config", "GuiSettings.yaml")
	if SettingsPath() != want {
		t.Fatalf("SettingsPath = %q, want %q", SettingsPath(), want)
	}

	// first run: no file → defaults
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Appearance.ThemeMode != "dark" || cfg.Appearance.FontSize != 14 {
		t.Fatalf("first-run defaults wrong: %+v", cfg.Appearance)
	}

	// a PARTIAL file (only theme_mode) must overlay on defaults, not blank the rest
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("appearance:\n  theme_mode: light\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Appearance.ThemeMode != "light" {
		t.Fatalf("partial file should set theme_mode=light: %+v", cfg.Appearance)
	}
	if cfg.Appearance.FontSize != 14 || cfg.Appearance.FontFamily != "Inter" {
		t.Fatalf("absent fields must keep defaults (overlay), got: %+v", cfg.Appearance)
	}

	// Save → Load round-trip
	cfg.Appearance.AccentColor = "#ff0000"
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Appearance.AccentColor != "#ff0000" || got.Appearance.ThemeMode != "light" {
		t.Fatalf("round-trip lost values: %+v", got.Appearance)
	}
}

// TestStarredAndSortRoundTrip: the tasks the person starred, the quick actions
// removed, per universe, and the sort order picked for the task and
// skill lists survive a save and a load.
func TestStarredAndSortRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	s := DefaultGuiSettings()
	s.Universes.Starred = map[string][]string{"dev": {"ktw-methods:tasks/generate-goal-features"}, "acme": {}}
	s.Universes.Sort = map[string]string{"tasks": "category", "skills": "source"}
	s.Universes.Removed = map[string][]string{"dev": {"term", "task:af:tasks/new-study"}}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if st := got.Universes.Starred["dev"]; len(st) != 1 || st[0] != "ktw-methods:tasks/generate-goal-features" {
		t.Fatalf("the starred tasks of dev were not kept: %+v", got.Universes.Starred)
	}
	if rm := got.Universes.Removed["dev"]; len(rm) != 2 || rm[0] != "term" || rm[1] != "task:af:tasks/new-study" {
		t.Fatalf("the removed quick actions of dev were not kept: %+v", got.Universes.Removed)
	}
	if got.Universes.Sort["tasks"] != "category" || got.Universes.Sort["skills"] != "source" {
		t.Fatalf("the sort orders were not kept: %+v", got.Universes.Sort)
	}
}

// TestTreeSettingsRoundTrip: the projects tree shows types by
// default, and « Show types » turned off survives a save and a load.
func TestTreeSettingsRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	if DefaultGuiSettings().Tree.HideTypes {
		t.Fatal("a new person's tree hides the types")
	}
	s := DefaultGuiSettings()
	s.Tree.HideTypes = true
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Tree.HideTypes {
		t.Fatalf("the tree's hidden types were not kept: %+v", got.Tree)
	}
}

// TestTheDisplayLevel: a new person starts at Essentiel; the
// level survives a save and a load; a file written with the former
// per-universe « Mode expert » turned on anywhere starts at Expert, and the
// old list is never written back.
func TestTheDisplayLevel(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KTW_KATYOTI_ROOT", root)
	application.ResetGlobals()
	t.Cleanup(application.ResetGlobals)

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.General.Level != LevelEssential {
		t.Fatalf("a new person: level %q; want %q", got.General.Level, LevelEssential)
	}
	got.General.Level = LevelStandard
	if err := Save(got); err != nil {
		t.Fatal(err)
	}
	if again, _ := Load(); again.General.Level != LevelStandard {
		t.Fatalf("after a save: level %q; want %q", again.General.Level, LevelStandard)
	}

	old := "version: 1\nuniverses:\n  expert: [archi]\n"
	if err := os.WriteFile(SettingsPath(), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	migrated, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if migrated.General.Level != LevelExpert || len(migrated.Universes.Expert) != 0 {
		t.Fatalf("an old expert list: level %q, list %v; want expert and no list", migrated.General.Level, migrated.Universes.Expert)
	}
	if err := Save(migrated); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(SettingsPath()); strings.Contains(string(raw), "expert:") {
		t.Fatalf("the old list was written back: %s", raw)
	}
}
