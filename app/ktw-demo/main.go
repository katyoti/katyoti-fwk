// SPDX-License-Identifier: Apache-2.0

// Command ktw-demo is the reference extension: demo tasks and the sign-
// off capability.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-demo/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
