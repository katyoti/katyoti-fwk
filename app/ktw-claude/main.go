// SPDX-License-Identifier: Apache-2.0

// Command ktw-claude is the Claude Code sidecar: KaTyoti drives Claude Code
// workers on the person's own Claude subscription.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-claude/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
