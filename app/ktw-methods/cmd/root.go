// SPDX-License-Identifier: Apache-2.0

// Package cmd wires ktw-methods: references, tasks, personas, the CWM
// type pack and data-types, the methods service and the goals, features
// and verification services.
package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

var version = "dev"

// Execute runs the ktw-methods CLI. `efs` carries the binary's OWN CWM type
// pack (embed/cwm); the shared personas/skills/references come from
// methodslocal.EmbedFS, consumed in buildExtensions.
func Execute(efs embed.FS) error {
	return appcore.BootstrapCLI("sidecar", buildExtensions(efs)...)
}

// Extensions returns the extension list Execute wires. Exposed so tests can
// build the application without running the CLI — keeping any such test bound
// to exactly what ships.
func Extensions(efs embed.FS) []application.Extension { return buildExtensions(efs) }
