// SPDX-License-Identifier: Apache-2.0

package application

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

var (
	globalEnv     AppEnvironment
	globalEnvOnce sync.Once

	globalCfg     *config.ConfigService
	globalCfgOnce sync.Once
)

// Env returns the process-wide AppEnvironment singleton for the ACTIVE
// workspace. First call drives the cenv chain: Preflight resolves the
// workspace path from flags/env/registry/fallback, LoadDotenv loads the
// 5-step .env chain, then newAppEnvironment composes all paths.
//
// Subsequent calls return the cached value. Use EnvOf(workspaceRoot) for a
// non-singleton, per-workspace AppEnvironment (e.g. ktw-gui's per-active-
// universe path resolution).
func Env() AppEnvironment {
	globalEnvOnce.Do(func() {
		workspaceRoot := cenv.Preflight(appName, os.Args[1:])
		// LoadDotenv needs the HomeDir which is composed from the just-resolved
		// workspace root + the .katyoti suffix.
		homeDir := filepath.Join(workspaceRoot, katyotiFolderName)
		cenv.LoadDotenv(appName, homeDir)
		globalEnv = newAppEnvironment(workspaceRoot)
	})
	return globalEnv
}

// ProjectRoot is the folder the active universe's work happens in — where an
// agent reads and writes files and runs commands: the project attached to it,
// else the workspace the universe was resolved to (a universe that is its own
// project, the registry's `home_path` of `dev`). Never the process's working
// folder: a served ktw-agents runs in its installation folder, a sidecar in
// its universe folder.
func ProjectRoot() string {
	if p := cenv.AttachedProject(Env().HomeDir()); p != "" {
		return p
	}
	return Env().WorkspaceDir()
}

// ProjectBase is the folder a project-relative path is anchored on: the
// project root.
func ProjectBase() string {
	return ProjectRoot()
}

// ProjectPath resolves a path as the person and the agents write it: an
// absolute path is kept (cleaned); a relative one (either separator style)
// is joined under ProjectBase.
func ProjectPath(p string) (string, error) {
	if p == "" {
		p = "."
	}
	cleaned := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(p, "\\", "/")))
	if filepath.IsAbs(cleaned) {
		return cleaned, nil
	}
	return filepath.Join(ProjectBase(), cleaned), nil
}

// EnvOf returns a fresh AppEnvironment for an arbitrary workspace root. No
// preflight or .env loading — the caller is expected to know the root path
// already (e.g. from a registry entry). Used by ktw-gui's orchestrator,
// which hosts multiple workspaces concurrently and resolves paths for each
// without disturbing the global singleton.
func EnvOf(workspaceRoot string) AppEnvironment {
	return newAppEnvironment(workspaceRoot)
}

// Config returns the process-wide ConfigService singleton for the ACTIVE
// workspace.
//
// Built as a two-layer chain per the universe-aware ADR:
//   - parent: <GlobalHomeDir>/config/ (cross-workspace global)
//   - child:  <HomeDir>/config/        (per-workspace; reads first, falls back to parent)
//
// If the two layers resolve to the same path (binary running outside a
// workspace), only one layer is built — no harm, no double-load.
func Config() *config.ConfigService {
	globalCfgOnce.Do(func() {
		env := Env()
		workspaceConfigDir := env.ConfigDir()
		globalConfigDir := filepath.Join(env.GlobalHomeDir(), "config")
		if globalConfigDir == workspaceConfigDir || env.GlobalHomeDir() == "" {
			globalCfg = config.NewConfigService(loader.NewFSLoader(workspaceConfigDir))
			return
		}
		parentCfg := config.NewConfigService(loader.NewFSLoader(globalConfigDir))
		globalCfg = parentCfg.Sub(loader.NewFSLoader(workspaceConfigDir))
	})
	return globalCfg
}

// ResetGlobals clears Env and Config singletons. Use in tests only.
func ResetGlobals() {
	globalEnvOnce = sync.Once{}
	globalEnv = nil
	globalCfgOnce = sync.Once{}
	globalCfg = nil
}
