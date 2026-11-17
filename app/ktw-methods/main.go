// SPDX-License-Identifier: Apache-2.0

// Command ktw-methods is the methods pack sidecar: the method skills,
// the generic CWM types and the goals, features and verification
// services.
package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-methods/cmd"
)

// embeddedFS carries ALL of this binary's content: the generic articulation
// CWM type pack (embed/cwm — node/item/edge YAMLs + co-located data-type
// schemas) AND the methods pack personas / skills / references (relocated here
// 2026-06-18, migrate-pack-content-to-sidecars, so the binary fully owns +
// contributes its content; ktw-agents consumes it from the aggregate catalog).
//
//go:embed embed/cwm embed/personas embed/skills embed/references embed/tasks
var embeddedFS embed.FS

func main() {
	if err := cmd.Execute(embeddedFS); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
