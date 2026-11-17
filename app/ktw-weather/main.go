// SPDX-License-Identifier: Apache-2.0

// Command ktw-weather is the canonical minimum sidecar (Open-Meteo
// weather).
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-weather/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
