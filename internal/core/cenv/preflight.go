// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// PreflightStatus records the outcome of workspace-selection preflight.
// Used by `ktw-agents check` and similar diagnostics to explain how
// KTW_WORKSPACE was resolved.
type PreflightStatus struct {
	// Source identifies which mechanism resolved the workspace. One of:
	// "flag-workspace", "flag-universe", "env-workspace", "env-universe", "fallback".
	Source string
	// WorkspacePath is the resolved workspace folder (always absolute, cleaned).
	WorkspacePath string
	// Err is non-nil if preflight failed; WorkspacePath in that case is
	// the best-effort fallback path.
	Err error
}

// preflightStatus is set by Preflight; read by callers via PreflightInfo.
var preflightStatus PreflightStatus

// PreflightInfo returns the recorded preflight outcome (for diagnostics).
func PreflightInfo() PreflightStatus { return preflightStatus }

// Preflight scans os.Args + KTW_WORKSPACE / KTW_UNIVERSE env vars and resolves
// the active workspace folder. If a path is resolved, it is set as
// KTW_WORKSPACE so downstream resolvers pick it up.
//
// Called explicitly by the application package's Env() initializer — the
// application owns the appName ("katyoti") and passes it through.
//
// Resolution order (first-match-wins):
//
//  1. --workspace <path>    CLI flag (mutually exclusive with --universe)
//  2. --universe <name>     CLI flag → registry → workspace path
//     ─── if no flag, load global .env so it can drive selection ───
//  3. KTW_WORKSPACE env var after .env load
//  4. KTW_UNIVERSE env var  after .env load → registry → workspace path
//  5. Fallback              <KatyotiRoot>/universes/default/ (NO auto-create)
//
// Returns the resolved workspace path. Errors (e.g., universe-name not in
// registry) are recorded in preflightStatus.Err but do NOT fail the
// preflight — the returned path is always usable (best-effort fallback).
//
// Idempotent: repeated calls re-resolve.
func Preflight(appName string, args []string) string {
	CurrentInstallation()
	flagWorkspace, flagUniverse, flagErr := scanWorkspaceFlags(args)

	if flagErr != nil {
		preflightStatus = PreflightStatus{Source: "fallback", WorkspacePath: defaultWorkspaceHome(appName), Err: flagErr}
		_ = os.Setenv("KTW_WORKSPACE", preflightStatus.WorkspacePath)
		return preflightStatus.WorkspacePath
	}

	switch {
	case flagWorkspace != "":
		path := absClean(flagWorkspace)
		_ = os.Setenv("KTW_WORKSPACE", path)
		preflightStatus = PreflightStatus{Source: "flag-workspace", WorkspacePath: path}
		return path

	case flagUniverse != "":
		path, err := resolveUniverseName(appName, flagUniverse)
		if err != nil {
			fallback := defaultWorkspaceHome(appName)
			_ = os.Setenv("KTW_WORKSPACE", fallback)
			preflightStatus = PreflightStatus{Source: "fallback", WorkspacePath: fallback, Err: err}
			return fallback
		}
		_ = os.Setenv("KTW_WORKSPACE", path)
		preflightStatus = PreflightStatus{Source: "flag-universe", WorkspacePath: path}
		return path
	}

	// No CLI flag — let the .env chain influence selection.
	// <KatyotiRoot>/.env (global) may set KTW_WORKSPACE / KTW_UNIVERSE — only
	// those are read here: the rest loads after the universe's own .env.
	if root, err := globalAgentsRoot(appName); err == nil {
		loadEnvKeys(filepath.Join(root, ".env"), "KTW_WORKSPACE", "KTW_UNIVERSE")
	}

	// Step 3: CWD/.env early-pass - never for an installation's process: the
	// current folder's .env would choose its universe from outside it.
	if envWS := strings.TrimSpace(os.Getenv("KTW_WORKSPACE")); envWS == "" && CurrentInstallation() == nil {
		if envUniverse := strings.TrimSpace(os.Getenv("KTW_UNIVERSE")); envUniverse == "" {
			if cwd, err := os.Getwd(); err == nil {
				recordEnvCandidate(filepath.Join(cwd, ".env"))
			}
		}
	}

	envWS := strings.TrimSpace(os.Getenv("KTW_WORKSPACE"))
	envUniverse := strings.TrimSpace(os.Getenv("KTW_UNIVERSE"))

	switch {
	case envWS != "":
		path := absClean(envWS)
		_ = os.Setenv("KTW_WORKSPACE", path)
		preflightStatus = PreflightStatus{Source: "env-workspace", WorkspacePath: path}
		return path

	case envUniverse != "":
		path, err := resolveUniverseName(appName, envUniverse)
		if err != nil {
			fallback := defaultWorkspaceHome(appName)
			_ = os.Setenv("KTW_WORKSPACE", fallback)
			preflightStatus = PreflightStatus{Source: "fallback", WorkspacePath: fallback, Err: err}
			return fallback
		}
		_ = os.Setenv("KTW_WORKSPACE", path)
		preflightStatus = PreflightStatus{Source: "env-universe", WorkspacePath: path}
		return path

	default:
		path := defaultWorkspaceHome(appName)
		_ = os.Setenv("KTW_WORKSPACE", path)
		preflightStatus = PreflightStatus{Source: "fallback", WorkspacePath: path}
		return path
	}
}

// scanWorkspaceFlags extracts the universe home (--home, or its older name
// --workspace) and --universe values from raw args. Returns mutex error if a
// home AND a universe are both present. `--home` is the flag the binaries
// declare ("Universe home folder"); reading only `--workspace` left it
// silently ignored — the process ran on the default universe.
func scanWorkspaceFlags(args []string) (workspace string, universe string, err error) {
	for i := 0; i < len(args); i++ {
		switch {
		case (args[i] == "--workspace" || args[i] == "--home") && i+1 < len(args):
			workspace = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--workspace="):
			workspace = strings.TrimPrefix(args[i], "--workspace=")
		case strings.HasPrefix(args[i], "--home="):
			workspace = strings.TrimPrefix(args[i], "--home=")
		case args[i] == "--universe" && i+1 < len(args):
			universe = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--universe="):
			universe = strings.TrimPrefix(args[i], "--universe=")
		}
	}
	if workspace != "" && universe != "" {
		return "", "", fmt.Errorf("--home and --universe are mutually exclusive")
	}
	return workspace, universe, nil
}

// ResolveUniverseHome returns the workspace folder registered for `name` in
// `<KatyotiRoot>/universes.yaml`, or "" when the name is not registered or the
// registry cannot be read.
//
// EXPORTED because the central-universe convention is not the whole truth. A
// universe's `home_path` is recorded PER UNIVERSE precisely so one can live
// anywhere, and a repository checkout is the ordinary case — the software and
// the universe that configures it committed together. Composing
// `<GlobalHomeDir>/universes/<name>/` instead made every non-central universe
// unreachable: attaching to one was refused with a path nobody had chosen, and
// the runtime would have resolved to that same non-existent folder.
//
// It returns "" rather than an error so the caller can fall back to the
// convention. An unregistered name is not necessarily a fault — the
// self-contained, pre-split layout has no registry entry at all.
func ResolveUniverseHome(appName, name string) string {
	home, err := resolveUniverseName(appName, name)
	if err != nil {
		return ""
	}
	return home
}

// resolveUniverseName looks up a universe by name in <KatyotiRoot>/universes.yaml
// and returns its workspace path. Errors if the name is not registered or the
// registry is unreadable.
func resolveUniverseName(appName, name string) (string, error) {
	regPath, err := defaultRegistryPath(appName)
	if err != nil {
		return "", fmt.Errorf("resolve registry path: %w", err)
	}
	bytes, err := os.ReadFile(regPath)
	if err != nil {
		return "", fmt.Errorf("read universe registry %s: %w", regPath, err)
	}
	var raw struct {
		Universes []struct {
			Name     string `yaml:"name"`
			HomePath string `yaml:"home_path"`
		} `yaml:"universes"`
	}
	if err := yaml.Unmarshal(bytes, &raw); err != nil {
		return "", fmt.Errorf("decode registry %s: %w", regPath, err)
	}
	for _, e := range raw.Universes {
		if e.Name == name {
			if e.HomePath == "" {
				return "", fmt.Errorf("universe %q in registry has empty home_path", name)
			}
			return absClean(e.HomePath), nil
		}
	}
	return "", fmt.Errorf("universe %q not found in registry %s", name, regPath)
}

// defaultRegistryPath returns the cross-workspace registry path
// (`<KatyotiRoot>/universes.yaml`). Honors KTW_KATYOTI_ROOT (an installation's
// katyoti.yml sets it; tests too).
func defaultRegistryPath(appName string) (string, error) {
	CurrentInstallation()
	if root := os.Getenv("KTW_KATYOTI_ROOT"); root != "" {
		return filepath.Join(KeepTestsOut(filepath.Clean(root)), "universes.yaml"), nil
	}
	root, err := globalAgentsRoot(appName)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "universes.yaml"), nil
}

// configHomeRoot returns `~/.config/` (or XDG_CONFIG_HOME if set).
func configHomeRoot() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Clean(v)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

// globalAgentsRoot returns the cross-workspace global root — `<XDG>/<appName>`.
// Honors KTW_KATYOTI_ROOT (an installation's katyoti.yml sets it; tests too).
func globalAgentsRoot(appName string) (string, error) {
	CurrentInstallation()
	if root := os.Getenv("KTW_KATYOTI_ROOT"); root != "" {
		return KeepTestsOut(filepath.Clean(root)), nil
	}
	return KeepTestsOut(filepath.Join(configHomeRoot(), appName)), nil
}

// universesRoot returns the cross-workspace system-universes parent —
// `<KatyotiRoot>/universes/`.
func universesRoot(appName string) string {
	root, err := globalAgentsRoot(appName)
	if err != nil {
		return ""
	}
	return filepath.Join(root, "universes")
}

// defaultWorkspaceHome returns `<universes-root>/default/`. A test process's
// default universe is its own, under TestRoot (KeepTestsOut): it is made, as
// the person's exists, so a test that runs a command in it finds it.
func defaultWorkspaceHome(appName string) string {
	home := filepath.Join(universesRoot(appName), "default")
	if testing.Testing() && isUnder(home, TestRoot()) {
		_ = os.MkdirAll(home, 0o755)
	}
	return home
}

// absClean cleans + makes absolute the given path. Returns the input on error.
func absClean(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

// GlobalAgentsRoot is the public accessor for the cross-workspace global
// root. Callers pass the app name (e.g. "katyoti") — cenv embeds no literal.
func GlobalAgentsRoot(appName string) string {
	root, err := globalAgentsRoot(appName)
	if err != nil {
		return ""
	}
	return root
}

// UniversesRoot is the public accessor for the system-universes parent
// (`<KatyotiRoot>/universes/`).
func UniversesRoot(appName string) string {
	return universesRoot(appName)
}
