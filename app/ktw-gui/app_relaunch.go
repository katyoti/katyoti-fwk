// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// RelaunchResult reports why a relaunch could not start. An empty Error means
// the handoff began and this process is on its way out.
type RelaunchResult struct {
	Error string `json:"error,omitempty"`
}

// WHY THE GUI DOES NOT MOVE THE CONTROL PLANE ITSELF. `selfUpdate` replaces ktw-gui.exe and
// ktw-hub.exe. Rename-aside makes that safe ON DISK while they run, but the
// running processes keep executing the OLD code until they restart — so a GUI
// that applied an infra update and reported success would be telling the truth
// about the files and a lie about itself.
//
// Handing off to katyoti instead makes the swap happen with nothing running,
// which is the only moment it is unambiguously safe, and the user lands in the
// version that was just installed rather than being told to restart.
//
// This is a Wails binding rather than a KTSP call ON PURPOSE: it is process
// lifecycle — spawn a sibling, exit — which is precisely the category KTSP
// cannot express. Every other operation in this panel goes over the generic
// dispatcher.
//
// RelaunchWithInfra quits this GUI and starts `katyoti gui` with the control
// plane moved first — to `version` (update or roll back), or reinstalled
// (`repair`) — and nothing else: sidecars are untouched (they update from the
// Marketplace's Sidecars tab). `universe` reopens the universe on screen.
func (a *App) RelaunchWithInfra(version string, repair bool, universe string) RelaunchResult {
	args := []string{"gui"}
	if version != "" {
		args = append(args, "--infra-version", version)
	}
	if repair {
		args = append(args, "--repair")
	}
	if universe != "" {
		args = append(args, "--universe", universe)
	}
	if len(args) == 1 {
		return RelaunchResult{Error: "nothing to do: give a version or ask a repair"}
	}
	return a.relaunch(args...)
}

// relaunch starts katyoti with args, detached, then quits this GUI.
func (a *App) relaunch(args ...string) RelaunchResult {
	katyoti, err := controlplane.TargetPath("katyoti")
	if err != nil {
		return RelaunchResult{Error: errString(err)}
	}
	if _, err := os.Stat(katyoti); err != nil {
		// A GUI running from a dev tree may have no control-plane katyoti at
		// all. Say which path was expected: "not found" without the path sends
		// the user looking in the wrong place.
		return RelaunchResult{Error: fmt.Sprintf("no katyoti at %s — update from a terminal with `katyoti gui --update`", katyoti)}
	}

	cmd := exec.Command(katyoti, args...)
	publish.NoConsoleWindow(cmd) // a GUI has no console: katyoti would open a window of its own
	// Detach: the child must outlive this process, since this process is about
	// to exit and the child is what replaces it.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return RelaunchResult{Error: errString(err)}
	}
	// Do NOT Wait: we are handing over, not supervising. Release so the child is
	// not left a zombie of a parent that is exiting.
	_ = cmd.Process.Release()

	// Quit AFTER the child is started, so a failed spawn leaves a working GUI
	// rather than no GUI at all.
	wailsRuntime.Quit(a.ctx)
	return RelaunchResult{}
}
