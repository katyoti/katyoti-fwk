// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// CleanAllSessionsResult reports the outcome of a bulk session cleanup:
// how many GUI-tracked runs were signalled to stop, how many on-disk session
// directories were deleted, and how many could not be removed (typically
// because their files are still locked by a live run on Windows).
type CleanAllSessionsResult struct {
	Stopped int    `json:"stopped"`
	Deleted int    `json:"deleted"`
	Failed  int    `json:"failed"`
	Error   string `json:"error,omitempty"`
}

// stopAllRuns cancels every run this GUI is currently tracking. Returns the
// count cancelled. A cancel unwinds the agent loop, which ends the session and
// drops it from the core's Room. Runs NOT started by this GUI (an external
// `ktw-agents run`, or a stale Room entry left by a crashed core) are not
// tracked here and are unaffected — their on-disk dirs are still removable by
// CleanAllSessions because the owning process is gone.
func (a *App) stopAllRuns() int {
	a.mu.Lock()
	runs := make([]*guiRun, 0, len(a.runs))
	for _, r := range a.runs {
		runs = append(runs, r)
	}
	a.mu.Unlock()

	n := 0
	for _, r := range runs {
		if r != nil && r.cancel != nil {
			r.cancel()
			n++
		}
	}
	return n
}

// CleanAllSessions stops every GUI-tracked run, then deletes ALL on-disk
// session state for the named universe — each `<DataDir>/sessions/<id>` dir
// plus the sibling `<DataDir>/terminal/<id>` recording (best effort).
//
// A directory whose files are still open (a run that hasn't finished unwinding
// after the stop signal, on Windows) fails to delete and is counted in Failed
// rather than aborting the sweep — the user can run it again once the run has
// exited. Zombie sessions left by a dead process delete cleanly because nothing
// holds their files.
//
// Wails-bound: window.go.main.App.CleanAllSessions(universeName).
func (a *App) CleanAllSessions(universeName string) CleanAllSessionsResult {
	if universeName == "" {
		return CleanAllSessionsResult{Error: "universeName is required"}
	}

	stopped := a.stopAllRuns()

	listResult := a.listUniverses()
	if listResult.Error != "" {
		return CleanAllSessionsResult{Stopped: stopped, Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return CleanAllSessionsResult{Stopped: stopped, Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionsDir := application.EnvOf(workspaceRoot).SessionsDir()
	terminalDir := filepath.Join(filepath.Dir(sessionsDir), "terminal")

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Nothing recorded yet — a no-op success.
			return CleanAllSessionsResult{Stopped: stopped}
		}
		return CleanAllSessionsResult{Stopped: stopped, Error: err.Error()}
	}

	var deleted, failed int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if rmErr := os.RemoveAll(filepath.Join(sessionsDir, id)); rmErr != nil {
			failed++
			continue
		}
		// Best-effort: drop the sibling terminal recording dir.
		_ = os.RemoveAll(filepath.Join(terminalDir, id))
		deleted++
	}

	return CleanAllSessionsResult{Stopped: stopped, Deleted: deleted, Failed: failed}
}
