// SPDX-License-Identifier: Apache-2.0

// Command ktw-hub is the messaging-bus host: embedded NATS, the
// connection manager and the contribution catalog.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-hub/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
