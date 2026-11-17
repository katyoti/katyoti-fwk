// SPDX-License-Identifier: Apache-2.0

// Command ktw-sync is the multi-provider cloud storage synchronizer
// (rclone).
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-sync/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
