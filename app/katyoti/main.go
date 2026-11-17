// SPDX-License-Identifier: Apache-2.0

// Command katyoti is the katyoti front door: the launcher, updater and
// marketplace CLI that installs katyoti, creates and lists universes and
// launches the GUI.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/katyoti/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
