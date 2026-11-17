// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/wailsapp/wails/v2/pkg/options"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// An element's address (the study gui.element-addresses §3) is
// `<universe>/<place>/<path>`: `dev/object//links`
// `archi/screen/marketplace/sidecars`. ktw-gui starts on one with
// `--open <address>`; a link `katyoti://open/<address>` - the scheme the
// katyoti launcher registers - reaches it as a bare argument. The universe
// is the address's first segment; the frontend opens the rest
// (src/address.ts) once the universe is shown.

const addressLink = "katyoti://open/"

// openArg is the address the GUI was launched to open: `--open <address>`,
// `--open=<address>`, or a bare `katyoti://open/<address>`; "" when none.
func openArg(args []string) string {
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--open="); ok {
			return strings.TrimSpace(v)
		}
		if a == "--open" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1])
		}
		if strings.HasPrefix(a, addressLink) {
			return strings.TrimSpace(a)
		}
	}
	return ""
}

// universeOfAddress is the universe an address names: its first segment,
// once the link prefix is off; "" for no address.
func universeOfAddress(addr string) string {
	addr = strings.Trim(strings.TrimPrefix(strings.TrimSpace(addr), addressLink), "/")
	universe, _, _ := strings.Cut(addr, "/")
	return universe
}

// singleInstanceID keys the one window a universe has in this installation
// at this version: a second launch on the same universe hands its address to
// the window that runs (OnSecondInstanceLaunch) instead of opening another;
// another universe, installation or version opens its own. The version keeps
// a relaunch after an update clear of the lock its predecessor still holds.
func singleInstanceID(universe, version string) string {
	sum := sha256.Sum256([]byte(application.Env().GlobalHomeDir()))
	return "katyoti-gui-" + hex.EncodeToString(sum[:4]) + "-" + version + "-" + universe
}

// InitialOpen returns the address the GUI was launched to open, or "". The
// frontend asks once, after it showed the address's universe.
//
// Wails-bound: window.go.main.App.InitialOpen().
func (a *App) InitialOpen() string {
	return a.initialOpen
}

// onSecondLaunch is a second launch on this window's universe: the window
// comes to the front and opens the address the launch carried, if any.
func (a *App) onSecondLaunch(data options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	wailsRuntime.WindowUnminimise(a.ctx)
	wailsRuntime.WindowShow(a.ctx)
	if addr := openArg(data.Args); addr != "" {
		wailsRuntime.EventsEmit(a.ctx, "open.address", addr)
	}
}
