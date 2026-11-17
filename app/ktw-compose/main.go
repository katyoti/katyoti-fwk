// SPDX-License-Identifier: Apache-2.0

// Command ktw-compose is the files-only authoring sidecar: universe
// composition, sidecar scaffolding and scenario cases.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-compose/cmd"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/sandbox"
)

func main() {
	// `scenario <verb> <case>` works inside the case's sandbox: entered before
	// anything resolves this installation's paths.
	if code, entered := sandbox.Enter(os.Args); entered {
		os.Exit(code)
	}
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
