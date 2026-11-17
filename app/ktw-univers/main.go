// SPDX-License-Identifier: Apache-2.0

// ktw-univers is the universe catalog sidecar: registry reads and writes
// (list, info, create, delete) and the sidecar inventory scan. In the admin
// universe it serves the management services over KTSP. It holds no
// active-universes map and spawns no cores — ktw-gui does, one ktw-hub per
// active universe.
//
// See:
//   - (modules/univers/decisions/OD-002-where-do-the-cross-universe-management-services.cwm.md)
//   - (modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md)
//   - modules/gui/concepts.md §2 (the ActiveCore model)
//   - modules/katyoti/layout.md
//   - app/ktw-univers/README.md
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-univers/cmd"
)

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
