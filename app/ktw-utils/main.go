// SPDX-License-Identifier: Apache-2.0

// Command ktw-utils is the developer CLI: session analysis, journal
// browsing, KTSP inspection and client codegen.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
