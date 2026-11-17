// SPDX-License-Identifier: Apache-2.0

// Command ktw-docs is the document and diagram sidecar: rendering,
// diagrams, the drawio and slide-deck engines.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-docs/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
