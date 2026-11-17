// SPDX-License-Identifier: Apache-2.0

package main

import (
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/katyoti/katyoti-fwk/app/ktw-gui/internal/supervisor"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// UniverseActivate spawns a ktw-agents core for the named universe (with
// KTW_WORKSPACE injected). Idempotent: returns success without re-spawning if
// the universe is already active in this ktw-gui process.
//
// Wails-bound: window.go.main.App.UniverseActivate(name).
func (a *App) UniverseActivate(name string) UniverseActivateResult {
	if _, err := a.master.Activate(a.ctx, name); err != nil {
		return UniverseActivateResult{Error: err.Error()}
	}
	return UniverseActivateResult{
		Name:    name,
		Running: true,
	}
}

// UniverseDeactivate terminates the named universe's core (SIGTERM → 5s
// grace → SIGKILL) and removes it from the active-universes map.
// Idempotent: no-op if the universe is not active.
//
// Wails-bound: window.go.main.App.UniverseDeactivate(name).
func (a *App) UniverseDeactivate(name string) UniverseDeactivateResult {
	a.panels.stopUniverse(name) // its bus goes: no follow left on it
	if err := a.master.Deactivate(name); err != nil {
		return UniverseDeactivateResult{Error: err.Error()}
	}
	return UniverseDeactivateResult{Name: name, Running: false}
}

// UniverseRestart restarts the named universe in one action (« Redémarrer
// l'univers »): its hub and every member ended, even a hub this GUI
// found running, then the universe activated again - each program reads its
// keys, config and pinned version anew.
//
// Wails-bound: window.go.main.App.UniverseRestart(name).
func (a *App) UniverseRestart(name string) UniverseActivateResult {
	a.panels.stopUniverse(name) // the restart ends its bus: no follow left on it
	if _, err := a.master.Restart(a.ctx, name); err != nil {
		return UniverseActivateResult{Name: name, Error: err.Error()}
	}
	return UniverseActivateResult{Name: name, Running: true}
}

// UniverseListActive returns runtime info for every currently-active
// universe in this ktw-gui process. Used by the frontend to refresh
// busRunning indicators after lifecycle changes.
//
// Wails-bound: window.go.main.App.UniverseListActive().
func (a *App) UniverseListActive() UniverseListActiveResult {
	return UniverseListActiveResult{
		Active: a.master.ListActive(),
	}
}

// PickWorkspaceFolder opens an OS folder-picker dialog so the user can
// choose a folder: a new universe's home (the creation modal) or a project
// to attach (the projects panel). Returns the absolute path of the chosen
// folder, or empty string if the user cancelled. Errors only on
// dialog-spawn failure (rare). See modules/gui/universe-workspace-management.md §3.
//
// Wails-bound: window.go.main.App.PickWorkspaceFolder().
func (a *App) PickWorkspaceFolder() PickFolderResult {
	path, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                "Choose workspace folder for the new universe",
		CanCreateDirectories: true,
	})
	if err != nil {
		return PickFolderResult{Error: err.Error()}
	}
	return PickFolderResult{Path: path}
}

// PickExistingUniverseFolder opens an OS folder-picker dialog so the user
// can locate a workspace that's ALREADY been initialized (one that already
// contains `.katyoti/config/Univers.yaml`). The dialog title makes the
// expectation explicit; validation happens server-side in UniverseAddExisting.
//
// Wails-bound: window.go.main.App.PickExistingUniverseFolder().
func (a *App) PickExistingUniverseFolder() PickFolderResult {
	path, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                "Pick an existing workspace folder (must contain .katyoti/)",
		CanCreateDirectories: false,
	})
	if err != nil {
		return PickFolderResult{Error: err.Error()}
	}
	return PickFolderResult{Path: path}
}

// PickProjectsFolder opens an OS folder-picker dialog so the person chooses
// the installation's projects folder (the Ready to start screen's Profil et
// premiers pas tab, which records it through the hub's SetProjectsFolder —
// a folder inside the installation is refused there). Empty path: cancelled.
//
// Wails-bound: window.go.main.App.PickProjectsFolder().
func (a *App) PickProjectsFolder() PickFolderResult {
	path, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title:                "Choose the folder where your projects live",
		CanCreateDirectories: true,
	})
	if err != nil {
		return PickFolderResult{Error: err.Error()}
	}
	return PickFolderResult{Path: path}
}

// PickKeysFile opens an OS file-picker dialog so the person chooses a sealed
// keys file their team gave them (the Ready to start screen's Keys tab, which
// imports it through the hub with its password). Empty path: cancelled.
//
// Wails-bound: window.go.main.App.PickKeysFile().
func (a *App) PickKeysFile() PickFolderResult {
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Choose the keys file your team gave you",
	})
	if err != nil {
		return PickFolderResult{Error: err.Error()}
	}
	return PickFolderResult{Path: path}
}

// ── Result wrappers — keep error surfaces consistent across all bindings ──

// listUniverses reads the universe registry through the hub-served orchestrator,
// as the hub returns it.
//
// UNEXPORTED on purpose. It used to be the `UniverseList` Wails binding; the
// frontend now reads the registry through the generated typed KTSP client
// (src/ktsp), so this is no longer part of the GUI's API surface. It survives
// because several internal bindings (sessions, journals, task trees) need to
// resolve a universe's home path by name, and that is a Go-side concern.
func (a *App) listUniverses() UniverseListResult {
	svc, err := a.orchSvc()
	if err != nil {
		return UniverseListResult{Error: errString(err)}
	}
	out, err := svc.ListUniverses(a.ctx, orchapi.ListUniversesInput{})
	if err != nil {
		return UniverseListResult{Error: errString(err)}
	}
	return UniverseListResult{Universes: out.Universes}
}

type UniverseListResult struct {
	Universes []orchapi.UniverseSummary `json:"universes,omitempty"`
	Error     string                    `json:"error,omitempty"`
}

// UniverseDeleteResult reports the archive path on success so the GUI can
// surface "archived to <path>" feedback.
type UniverseDeleteResult struct {
	ArchivePath string `json:"archivePath,omitempty"`
	Error       string `json:"error,omitempty"`
}

// UniverseActivateResult reports the outcome of an activation attempt.
type UniverseActivateResult struct {
	Name    string `json:"name,omitempty"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

// UniverseDeactivateResult reports the outcome of a deactivation.
type UniverseDeactivateResult struct {
	Name    string `json:"name,omitempty"`
	Running bool   `json:"running"`
	Error   string `json:"error,omitempty"`
}

// UniverseListActiveResult lists all currently-active universes in this
// ktw-gui process.
type UniverseListActiveResult struct {
	Active []supervisor.ActiveUniverseInfo `json:"active,omitempty"`
	Error  string                          `json:"error,omitempty"`
}

// PickFolderResult is returned by PickWorkspaceFolder. Path is empty
// when the user cancels the OS dialog; Error is populated only on
// spawn failure (rare).
type PickFolderResult struct {
	Path  string `json:"path,omitempty"`
	Error string `json:"error,omitempty"`
}
