// SPDX-License-Identifier: Apache-2.0

// Package startup owns katyoti's universe-independent startup configuration —
// KatyotiConfig: the `settings` of the installation's katyoti.yml when the
// program belongs to one (cenv.CurrentInstallation), else
// <GlobalHomeDir>/config/Katyoti.yaml (a development build, a test). It answers "what does the
// katyoti front door do before any universe is chosen" — notably the launch
// mode read by the bare-`katyoti` launcher.
//
// Reads go through the standard ConfigService pipeline (variable expansion +
// ProcessConfig, so `default:` tags fire on first run) against the GLOBAL config
// layer only — never a workspace override, and never a raw yaml.Unmarshal.
// Writes go through config.SaveAtomic (ConfigService has no Save).
package startup

import (
	"fmt"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// LaunchMode values for KatyotiConfig.LaunchMode.
const (
	LaunchGUI  = "gui"  // bare `katyoti` opens the GUI (default)
	LaunchCLI  = "cli"  // bare `katyoti` prints help / stays a CLI
	LaunchNone = "none" // bare `katyoti` does nothing special (headless/CI)
)

// KatyotiConfig is the cross-universe startup config. On-disk keys are the
// `yaml` tags; `default:` tags materialize first-run values via ProcessConfig.
type KatyotiConfig struct {
	Version         int    `yaml:"version,omitempty"          json:"version,omitempty"`
	LaunchMode      string `yaml:"launch_mode"                json:"launchMode"                default:"gui"`
	DefaultUniverse string `yaml:"default_universe,omitempty" json:"defaultUniverse,omitempty"`
	AutostartHub    bool   `yaml:"autostart_hub,omitempty"    json:"autostartHub,omitempty"`
	// UpdateCheck gates the launch-time "updates available" notice. On by
	// default: an update nobody is told about is an update nobody applies.
	// Set "off" for kiosk/CI machines that must never phone home.
	UpdateCheck string `yaml:"update_check,omitempty"     json:"updateCheck,omitempty"     default:"on"`
	// ProjectsFolder is the folder holding the person's projects, chosen at
	// first start (cenv.Installation.ProjectsFolder: <Documents>/Katyoti until
	// then), never inside the installation.
	ProjectsFolder string `yaml:"projects_folder,omitempty" json:"projectsFolder,omitempty"`
}

// ValidLaunchModes is the accepted set for LaunchMode.
var ValidLaunchModes = []string{LaunchGUI, LaunchCLI, LaunchNone}

func validLaunchMode(m string) bool {
	for _, v := range ValidLaunchModes {
		if m == v {
			return true
		}
	}
	return false
}

// Path is the absolute path to the config file: the installation's katyoti.yml,
// else <GlobalHomeDir>/config/Katyoti.yaml.
func Path() string {
	if inst := cenv.CurrentInstallation(); inst != nil {
		return inst.Path()
	}
	return filepath.Join(globalConfigDir(), config.ConfigName[KatyotiConfig]()+".yaml")
}

func globalConfigDir() string {
	return filepath.Join(application.Env().GlobalHomeDir(), "config")
}

// Load resolves KatyotiConfig from the global layer ONLY (no workspace overlay —
// a startup setting is resolved before any universe is active) through the full
// pipeline. A missing file yields the `default:`-tagged values (first-run).
func Load() (KatyotiConfig, error) {
	var cfg KatyotiConfig
	if inst := cenv.CurrentInstallation(); inst != nil {
		settings := inst.Settings
		if settings == nil {
			settings = map[string]any{}
		}
		if err := config.DecodeMap(settings, &cfg); err != nil {
			return cfg, fmt.Errorf("%s settings: %w", inst.Path(), err)
		}
		return cfg, nil
	}
	if err := config.LoadFromDir(globalConfigDir(), config.ConfigName[KatyotiConfig](), &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes KatyotiConfig to the installation's katyoti.yml (its settings),
// else to <GlobalHomeDir>/config/Katyoti.yaml atomically (config.SaveAtomic). Rejects an invalid launch mode so a bad `config set`
// can't strand the launcher.
func Save(cfg KatyotiConfig) error {
	if cfg.LaunchMode == "" {
		cfg.LaunchMode = LaunchGUI
	}
	if !validLaunchMode(cfg.LaunchMode) {
		return fmt.Errorf("invalid launch_mode %q (want one of %v)", cfg.LaunchMode, ValidLaunchModes)
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	if inst := cenv.CurrentInstallation(); inst != nil {
		// The installation's file is the start point: its settings are these.
		data, err := marshal.Marshal(cfg, marshal.YAML)
		if err != nil {
			return err
		}
		var settings map[string]any
		if err := config.DecodeBytes(data, marshal.YAML, &settings); err != nil {
			return err
		}
		delete(settings, "version")
		inst.Settings = settings
		return cenv.SaveInstallation(inst)
	}
	return config.SaveAtomic(globalConfigDir(), config.ConfigName[KatyotiConfig](), &cfg)
}

// Set applies one key=value to the stored config and saves it. Keys mirror the
// yaml field names (kebab/snake accepted). Returns the updated config.
func Set(key, value string) (KatyotiConfig, error) {
	cfg, err := Load()
	if err != nil {
		return cfg, err
	}
	switch key {
	case "launch_mode", "launch-mode":
		if !validLaunchMode(value) {
			return cfg, fmt.Errorf("invalid launch_mode %q (want one of %v)", value, ValidLaunchModes)
		}
		cfg.LaunchMode = value
	case "default_universe", "default-universe":
		cfg.DefaultUniverse = value
	case "autostart_hub", "autostart-hub":
		cfg.AutostartHub = value == "true" || value == "1" || value == "yes"
	case "projects_folder", "projects-folder":
		// Never inside the installation: an uninstall would take the projects.
		if err := cenv.CurrentInstallation().CheckProjectsFolder(value); err != nil {
			return cfg, err
		}
		cfg.ProjectsFolder = filepath.Clean(value)
	case "locked":
		// The installation's lock, beside its settings: no version moves by
		// itself while it is on (cenv.Installation.Locked).
		inst := cenv.CurrentInstallation()
		if inst == nil {
			return cfg, fmt.Errorf("locked: this katyoti belongs to no installation (a development build) - there is nothing to lock")
		}
		switch value {
		case "on", "true", "1", "yes":
			inst.Locked = true
		case "off", "false", "0", "no":
			inst.Locked = false
		default:
			return cfg, fmt.Errorf("invalid locked %q (want on | off)", value)
		}
		return cfg, cenv.SaveInstallation(inst)
	default:
		return cfg, fmt.Errorf("unknown key %q (want launch_mode | default_universe | autostart_hub | projects_folder | locked)", key)
	}
	if err := Save(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
