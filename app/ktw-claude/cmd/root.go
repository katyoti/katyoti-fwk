// SPDX-License-Identifier: Apache-2.0

// Package cmd wires ktw-claude: the Claude Code driver behind a flat KTSP CLI.
package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

var version = "dev"

// Execute runs the ktw-claude CLI with the given embedded filesystem.
func Execute(efs embed.FS) error {
	return appcore.BootstrapCLI("sidecar", buildExtensions(efs)...)
}

// Extensions returns the extension list Execute wires, for the census test.
func Extensions(efs embed.FS) []application.Extension { return buildExtensions(efs) }
