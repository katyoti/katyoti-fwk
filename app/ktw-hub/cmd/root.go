// SPDX-License-Identifier: Apache-2.0

// Package cmd is the Cobra root for the `ktw-hub` binary.
//
// ktw-hub is the messaging-bus host: embedded NATS server (desktop) or
// NATS-cluster client (cloud), plus ConnectionManager + Catalog + room /
// session / journal state services. Per the bus decisions in
// modules/bus/decisions/: (NATS carries the messages)
// (how subjects are scoped), (why ktw-hub is its own binary).
//
// ktw-hub is NOT a user-facing day-to-day binary. ktw-gui and ktw-agents
// (CLI) spawn it via the hubclient library. Direct invocation is for
// headless / systemd / k8s deploys.
package cmd

import (
	"embed"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// version is set at build time via -ldflags "-X cmd.version=...".
var version = "dev"

// Execute runs the ktw-hub CLI with the given embedded filesystem.
func Execute(efs embed.FS) error {
	return appcore.BootstrapCLI("hub", buildExtensions(efs)...)
}

// Extensions returns the extension list Execute wires. Exposed so tests can
// build the application without running the CLI — keeping any such test bound
// to exactly what ships.
func Extensions(efs embed.FS) []application.Extension { return buildExtensions(efs) }
