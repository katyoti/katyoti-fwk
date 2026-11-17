// SPDX-License-Identifier: Apache-2.0

package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/app/ktw-gui/internal/supervisor"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// `describe` must NOT open a window: the marketplace indexer runs it on every
	// catalog build to read this binary's metadata. Short-circuit before any
	// logging or wails.Run so ktw-gui can be a first-class marketplace item.
	if describeRequested(os.Args[1:]) {
		emitDescribe()
		return
	}

	// Always write logs to a file under the user's data dir so diagnostics
	// survive Wails detaching from the parent console AND don't clutter the
	// install directory. NOTE: writing to file ONLY — using
	// io.MultiWriter(os.Stderr, file) silently drops everything once stderr
	// is dead (which it is in detached Wails GUI builds).
	if logFile, err := openLogFile(); err == nil {
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
		log.Printf("[main] log file opened: %s", logFile.Name())
		currentLogPath = logFile.Name()
	}

	// A universe's hub found running at another version is reported.
	supervisor.ClientVersion = appcore.ResolveVersion(version)
	// An address to open names its universe when --universe
	// does not.
	open := openArg(os.Args[1:])
	universe := universeArg(os.Args[1:])
	if universe == "" {
		universe = universeOfAddress(open)
	}
	app := NewApp(universe)
	app.initialOpen = open
	app.noWelcome = startNoWelcome()
	// One window per universe: a second launch on it (a katyoti:// link)
	// hands its address over; no universe, no lock (the picker).
	var single *options.SingleInstanceLock
	if universe != "" {
		single = &options.SingleInstanceLock{UniqueId: singleInstanceID(universe, appcore.ResolveVersion(version)), OnSecondInstanceLaunch: app.onSecondLaunch}
	}

	// The proof's debugging port, when its environment asks one.
	openDebugPort()

	err := wails.Run(&options.App{
		SingleInstanceLock: single,
		Title:              windowTitle(),
		// Opens maximised, so it fits any screen; restored, it is 1024×640.
		Width:            1024,
		Height:           640,
		WindowStartState: options.Maximised,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 26, G: 27, B: 30, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
	})
	// Production builds run without `-windowsconsole`, so stdout is
	// detached and `println` vanishes. Route any startup error
	// through the file logger so we don't lose it silently.
	if err != nil {
		log.Printf("[main] wails.Run failed: %v", err)
	}
}

// universeArg extracts the `--universe <name>` (or `--universe=<name>`) launch
// flag from the process args, ignoring anything else Wails may pass. The katyoti
// launcher sets it (resolved from its own flag or the startup default) so the
// GUI opens straight into that universe. Returns "" when absent — the GUI then
// boots to the welcome picker.
func universeArg(args []string) string {
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--universe="); ok {
			return v
		}
		if a == "--universe" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// openLogFile creates a timestamped log file under `<DataDir>/logs/` so
// diagnostics survive Wails detaching from the parent console AND don't
// land next to the binary (which would mix per-user log state with a
// shared install dir, and lose logs across binary upgrades). Returns the
// file handle ready for tee'ing log output.
//
// Resolution: via `application.Env().DataDir()` — honours `KTW_DATA` env
// override, then `XDG_DATA_HOME/katyoti`, then the platform default
// (`%LOCALAPPDATA%\katyoti` on Windows).
//
// Fallback: if the data dir cannot be created (read-only home, permission
// denied), drop into `os.TempDir()` rather than the binary folder, so the
// install dir stays clean even in degraded environments.
func openLogFile() (*os.File, error) {
	name := fmt.Sprintf("ktw-gui-%s.log", time.Now().Format("20060102-150405"))

	logsDir := filepath.Join(application.Env().DataDir(), "logs")
	if err := os.MkdirAll(logsDir, 0o755); err == nil {
		if f, err := os.Create(filepath.Join(logsDir, name)); err == nil {
			return f, nil
		}
	}

	// Degraded fallback: temp dir. Still avoids polluting the install dir.
	tmpDir := filepath.Join(os.TempDir(), "ktw-agents", "logs")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return nil, err
	}
	return os.Create(filepath.Join(tmpDir, name))
}

// windowTitle names the installation the GUI runs in when it is not the
// default one: `katyoti instance gui <name>` sets KTW_INSTANCE.
func windowTitle() string {
	if inst := strings.TrimSpace(os.Getenv("KTW_INSTANCE")); inst != "" {
		return "ktw-gui — " + inst
	}
	return "ktw-gui"
}
