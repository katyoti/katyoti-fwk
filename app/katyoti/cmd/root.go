// SPDX-License-Identifier: Apache-2.0

// Package cmd wires the katyoti front-door CLI: marketplace, in-process
// universe registry, profile, startup config and launcher.
package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

var version = "dev"

// Execute runs the katyoti CLI with the given embedded filesystem.
func Execute(efs embed.FS) error {
	return appcore.BootstrapCLI("sidecar", buildExtensions(efs)...)
}

// Extensions returns the extension list Execute wires. Exposed so tests can
// build the application without running the CLI — keeping any such test bound
// to exactly what ships.
func Extensions(efs embed.FS) []application.Extension { return buildExtensions(efs) }
