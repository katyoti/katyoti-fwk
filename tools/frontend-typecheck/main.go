// SPDX-License-Identifier: Apache-2.0

// frontend-typecheck runs the ktw-gui frontend's TypeScript compiler.
//
// # Why a Go program for one command
//
// The recipe it replaces was `cd … && if [ -d node_modules ]; then npx tsc
// --noEmit; else echo …; fi` — POSIX shell, which is what gomake happens to
// invoke on this machine and is a parse error from cmd.exe. A check that only
// runs under one shell is a check the next person quietly loses, and this one
// exists precisely because a contract was going unchecked.
//
// Everything else in `check` is `go run ./tools/…`; this now matches.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	dir := filepath.Join("app", "ktw-gui", "frontend")

	if _, err := os.Stat(filepath.Join(dir, "node_modules")); err != nil {
		// A Go-only checkout should still be able to run `make check`; it just
		// cannot check the half that needs a TypeScript toolchain. Said out
		// loud, because a silent skip is how the gap got here.
		fmt.Printf("frontend-typecheck: SKIPPED — no %s/node_modules "+
			"(run `npm install` there to include the GUI in this check)\n", dir)
		return
	}

	// npx is a shell script on Windows (npx.cmd); exec.LookPath finds the right
	// one via PATHEXT, but the bare name is not executable there.
	bin := "npx"
	if runtime.GOOS == "windows" {
		bin = "npx.cmd"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "frontend-typecheck: %s not found on PATH: %v\n", bin, err)
		os.Exit(1)
	}

	// The same compile `wails build` runs, minus the bundle.
	cmd := exec.Command(path, "tsc", "--noEmit")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// tsc already printed file:line:col and the message.
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "frontend-typecheck: run tsc: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("frontend-typecheck: OK")
}
