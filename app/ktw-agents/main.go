// SPDX-License-Identifier: Apache-2.0

// Command ktw-agents is the main agent host: the task runner, the CWM
// host and the serving process of a universe.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-agents/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
