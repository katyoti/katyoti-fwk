// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"io/fs"

	"github.com/spf13/cobra"
)

// CobraCommand wraps a *cobra.Command so it can be stored as a named service.
// Use UseNamedService[CobraCommand](app, "root") to retrieve the root,
// or any other name for sub-command groups.
type CobraCommand struct {
	Cmd *cobra.Command
}

// AppConfig defines the static identity for a ktw-* application.
// Infrastructure needs (config files, tools, API keys) are declared
// on PublishedService via WithConfigFiles/WithTools/WithAPIKeys — not here.
type AppConfig struct {
	// AppName is the binary name (e.g. "ktw-docs").
	AppName string

	// Version is set by ldflags at build time (e.g. "1.2.3").
	Version string

	// Short is the one-line description for the root command.
	Short string

	// Long is the detailed description for the root command.
	Long string

	// DisplayName is a human-friendly name for catalogs/UIs (e.g. "LeanIX
	// sidecar"). Optional — `describe` falls back to AppName.
	DisplayName string

	// Icon is an optional icon reference (emoji or name) surfaced by
	// marketplace catalogs + GUIs. Optional.
	Icon string

	// EmbeddedFS is the binary's embedded filesystem containing defaults.
	EmbeddedFS fs.FS

	// Transports lists supported transport protocols (default: ["stdio"]).
	Transports []string
}
