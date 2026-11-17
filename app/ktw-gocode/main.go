// SPDX-License-Identifier: Apache-2.0

// Command ktw-gocode is the Go language pack sidecar: the Go module,
// use-case, verify and tasktest verbs and the Go toolchain wrappers.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-gocode/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
