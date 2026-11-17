// SPDX-License-Identifier: Apache-2.0

// Command ktw-code is the code-methodology sidecar: code analysis,
// module discovery, decisions and the charter and manifest projections.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-code/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
