// SPDX-License-Identifier: Apache-2.0

package api

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// DisplayBinEnv names the environment variable that points at a ktw-display
// binary, ahead of every other place.
const DisplayBinEnv = "KTW_DISPLAY_BIN"

// DisplayBinary is where the compiled display server (ktw-display) was found,
// or the places looked at when it was not (Path empty).
type DisplayBinary struct {
	Path   string
	Looked []string
}

// LocateDisplayBinary finds the compiled ktw-display, in priority order:
//  1. KTW_DISPLAY_BIN (explicit; for tests and custom builds);
//  2. the one a marketplace installed (`<GlobalHomeDir>/sidecars.yaml`): in an
//     installation each binary lives in its own store folder, never beside
//     another program;
//  3. beside the running binary (`bin/ktw-display.exe` beside the programs
//     `make build` places in bin/);
//  4. app/ktw-display/dist/ under the working directory — the package's own
//     build output, for `bun run` development.
//
// The display bridge spawns what it finds; `ktw-docs deck export` starts it
// alone. Looked lists each place in that order, for a message that says where.
func LocateDisplayBinary() DisplayBinary {
	var looked []string
	name := "ktw-display"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if override := os.Getenv(DisplayBinEnv); override != "" {
		if _, err := os.Stat(override); err == nil {
			return DisplayBinary{Path: override}
		}
		looked = append(looked, DisplayBinEnv+"="+override+" (no such file)")
	} else {
		looked = append(looked, DisplayBinEnv+" (unset)")
	}

	if root := application.Env().GlobalHomeDir(); root != "" {
		yaml := filepath.Join(root, "sidecars.yaml")
		if p := catalog.InstalledBinary("ktw-display"); p != "" {
			return DisplayBinary{Path: p}
		}
		looked = append(looked, "the installation's record "+yaml+" (no installed ktw-display)")
	}

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		candidate := filepath.Join(filepath.Dir(exe), name)
		if _, err := os.Stat(candidate); err == nil {
			return DisplayBinary{Path: candidate}
		}
		looked = append(looked, candidate)
	}

	candidate := filepath.Join("app", "ktw-display", "dist", name)
	if _, err := os.Stat(candidate); err == nil {
		return DisplayBinary{Path: candidate}
	}
	if abs, err := filepath.Abs(candidate); err == nil {
		candidate = abs
	}
	looked = append(looked, candidate)
	return DisplayBinary{Looked: looked}
}
