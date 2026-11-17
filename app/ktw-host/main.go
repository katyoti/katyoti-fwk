// SPDX-License-Identifier: Apache-2.0

// Command ktw-host is the generic files-only host: it serves a folder as
// a sidecar.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-host/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
