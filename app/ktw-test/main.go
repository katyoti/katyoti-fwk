// SPDX-License-Identifier: Apache-2.0

// Command ktw-test is the virtual-terminal test harness.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-test/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
