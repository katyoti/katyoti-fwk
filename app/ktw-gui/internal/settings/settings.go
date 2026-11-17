// SPDX-License-Identifier: Apache-2.0

// Package settings provides ktw-gui's persistent settings stored at
// ~/.config/katyoti/config/GuiSettings.yaml — the cross-universe global
// GUI state per the universes-and-config-scoping ADR.
//
// Two serialization layouts:
//   - YAML on disk:   snake_case keys, version field
//   - JSON over Wails: camelCase keys (matches the frontend AppSettings type)
//
// Each field carries both `yaml` and `json` tags so the Marshal direction
// chooses the right naming convention.
package settings

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
)

// GuiSettings is the on-disk + Wails-exposed GUI state.
//
// Lives at <GlobalHomeDir>/config/GuiSettings.yaml — i.e.
// ~/.config/katyoti/config/GuiSettings.yaml by default. Honors
// KTW_KATYOTI_ROOT for tests.
//
// Per-window state (last-displayed universe, panel widths) lives in the
// browser localStorage, NOT here. This file is for cross-window globals.
type GuiSettings struct {
	Version       int                  `yaml:"version,omitempty"      json:"version,omitempty"`
	Appearance    AppearanceSettings   `yaml:"appearance,omitempty"   json:"appearance"`
	General       GeneralSettings      `yaml:"general,omitempty"      json:"general"`
	Universes     UniverseSettings     `yaml:"universes,omitempty"    json:"universes"`
	Notifications NotificationSettings `yaml:"notifications,omitempty" json:"notifications"`
	Tree          TreeSettings         `yaml:"tree,omitempty"          json:"tree"`
}

// AppearanceSettings — theme + typography + colors.
type AppearanceSettings struct {
	ThemeMode      string `yaml:"theme_mode"       json:"themeMode"`      // dark | light | system
	AccentColor    string `yaml:"accent_color"     json:"accentColor"`    // hex e.g. "#228be6"
	FontSize       int    `yaml:"font_size"        json:"fontSize"`       // px, 10-20
	FontFamily     string `yaml:"font_family"      json:"fontFamily"`     // Inter | Roboto | Open Sans | Lato | System UI
	MonoFamily     string `yaml:"mono_family"      json:"monoFamily"`     // JetBrains Mono | Fira Code | …
	CodeThemeDark  string `yaml:"code_theme_dark"  json:"codeThemeDark"`  // github-dark | dracula | one-dark | nord
	CodeThemeLight string `yaml:"code_theme_light" json:"codeThemeLight"` // github-light | one-dark | nord
}

// The display levels a person picks, once for all universes:
// Essentiel shows a newcomer's surfaces, Standard the daily ones, Expert all.
const (
	LevelEssential = "essential"
	LevelStandard  = "standard"
	LevelExpert    = "expert"
)

// GeneralSettings — the display level and chat behavior toggles.
type GeneralSettings struct {
	// Level is the person's display level: essential (a new person's),
	// standard or expert.
	Level             string `yaml:"level,omitempty"     json:"level"`
	AutoScroll        bool   `yaml:"auto_scroll"         json:"autoScroll"`
	ShowToolInputs    bool   `yaml:"show_tool_inputs"    json:"showToolInputs"`
	CompactMessages   bool   `yaml:"compact_messages"    json:"compactMessages"`
	FullWidthMessages bool   `yaml:"full_width_messages" json:"fullWidthMessages"`
}

// UniverseSettings — preferences for the universe-management UX.
//
// PreferredDefault is the universe name to display on first launch when no
// localStorage key exists yet. Empty = fall back to the admin universe.
// Per-window last-displayed lives in localStorage, not here.
type UniverseSettings struct {
	PreferredDefault string `yaml:"preferred_default,omitempty" json:"preferredDefault,omitempty"`
	// Starred are the tasks the person starred, per universe (task URIs):
	// Home shows them in its quick actions beside the tagged ones.
	Starred map[string][]string `yaml:"starred,omitempty" json:"starred,omitempty"`
	// Sort is the order the person picked per list (tasks, skills): name,
	// category or source.
	Sort map[string]string `yaml:"sort,omitempty" json:"sort,omitempty"`
	// Removed are the quick actions the person removed, per universe (a
	// built-in's id or task:<uri>): Home leaves them out until starred again.
	Removed map[string][]string `yaml:"removed,omitempty" json:"removed,omitempty"`
	// Expert is the former per-universe « Mode expert » list, read only to
	// migrate: a person who had turned it on anywhere starts at Expert; it is
	// never written back (the display level replaces it).
	Expert []string `yaml:"expert,omitempty" json:"-"`
}

// TreeSettings — how the projects tree draws its rows, chosen
// in the tree's settings menu. Each is the zero value by default, so a file
// written before it keeps today's tree.
type TreeSettings struct {
	// HideTypes leaves the type badge out of the tree's rows (the type's
	// icon stays).
	HideTypes bool `yaml:"hide_types" json:"hideTypes"`
}

// NotificationSettings — reserved; not active in v1.
type NotificationSettings struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// DefaultGuiSettings returns the baseline settings used when no GuiSettings.yaml
// exists yet. Mirrors the frontend's DEFAULT_SETTINGS export.
func DefaultGuiSettings() GuiSettings {
	return GuiSettings{
		Version: 1,
		Appearance: AppearanceSettings{
			ThemeMode:      "dark",
			AccentColor:    "#228be6",
			FontSize:       14,
			FontFamily:     "Inter",
			MonoFamily:     "JetBrains Mono",
			CodeThemeDark:  "github-dark",
			CodeThemeLight: "github-light",
		},
		General: GeneralSettings{
			Level:             LevelEssential,
			AutoScroll:        true,
			ShowToolInputs:    true,
			CompactMessages:   false,
			FullWidthMessages: false,
		},
		Universes: UniverseSettings{
			PreferredDefault: "",
		},
		Notifications: NotificationSettings{
			Enabled: false,
		},
	}
}

// guiSettingsMu serializes concurrent reads/writes of GuiSettings.yaml.
var guiSettingsMu sync.Mutex

// configDir is the global typed-config layer — `<GlobalHomeDir>/config/` (per
// the config/-vs-root doctrine: typed user-editable settings in `config/`,
// machine-managed registries at the global root).
func configDir() string {
	return filepath.Join(application.Env().GlobalHomeDir(), "config")
}

// settingsName keeps the file literally `GuiSettings.yaml` (the
// ConfigName-derived name would be `Gui.yaml`).
const settingsName = "GuiSettings"

// SettingsPath returns the absolute path to GuiSettings.yaml —
// `<GlobalHomeDir>/config/GuiSettings.yaml`.
func SettingsPath() string {
	return filepath.Join(configDir(), settingsName+".yaml")
}

// Load reads GuiSettings.yaml from the global config layer through the standard
// ConfigService pipeline (variable expansion + ProcessConfig) — NOT a raw
// yaml.Unmarshal. It starts from DefaultGuiSettings() and overlays whatever the
// file sets, so an absent field keeps its default (a partial file never blanks
// out the theme). A missing file yields the defaults (first-run).
func Load() (GuiSettings, error) {
	guiSettingsMu.Lock()
	defer guiSettingsMu.Unlock()

	s := DefaultGuiSettings()
	s.General.Level = "" // a file without one is migrated below
	if err := config.LoadFromDir(configDir(), settingsName, &s); err != nil {
		return DefaultGuiSettings(), fmt.Errorf("load GuiSettings: %w", err)
	}
	if s.Version == 0 {
		s.Version = 1
	}
	migrateLevel(&s)
	return s, nil
}

// migrateLevel gives a settings file written before the display level one:
// Expert when the person had turned the former per-universe expert mode on
// anywhere, else Essentiel; an unknown level reads as Essentiel.
func migrateLevel(s *GuiSettings) {
	switch s.General.Level {
	case LevelEssential, LevelStandard, LevelExpert:
	default:
		s.General.Level = LevelEssential
		if len(s.Universes.Expert) > 0 {
			s.General.Level = LevelExpert
		}
	}
	s.Universes.Expert = nil
}

// Save writes GuiSettings to disk atomically (config.SaveAtomic).
func Save(s GuiSettings) error {
	guiSettingsMu.Lock()
	defer guiSettingsMu.Unlock()

	if s.Version == 0 {
		s.Version = 1
	}
	if err := config.SaveAtomic(configDir(), settingsName, &s); err != nil {
		return fmt.Errorf("save GuiSettings: %w", err)
	}
	return nil
}
