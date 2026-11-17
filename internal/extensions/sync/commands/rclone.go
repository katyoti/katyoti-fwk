// SPDX-License-Identifier: Apache-2.0

// Package commands provides sidecar-specific CLI commands for ktw-sync.
package commands

import (
	"os"

	"github.com/rclone/rclone/cmd"
	// Backends are registered in providers.go (selective imports).
	// cmd/all removed — registers ~40 rclone subcommands we don't need and slows startup.
	// The rclone passthrough still works for commands that match registered backends.

	"github.com/spf13/cobra"
)

// RcloneCommand returns a cobra command that delegates to rclone's full CLI.
// Usage: ktw-sync rclone ls remote:path
//
//	ktw-sync rclone config show
//	ktw-sync rclone help backends
func RcloneCommand() *cobra.Command {
	rcloneRoot := cmd.Root
	rcloneRoot.Use = "rclone"
	rcloneRoot.Short = "Run rclone commands directly"
	rcloneRoot.Long = `Delegates to the embedded rclone CLI. All rclone commands are available:
  ktw-sync rclone ls remote:path
  ktw-sync rclone lsd remote:
  ktw-sync rclone config show
  ktw-sync rclone about remote:
  ktw-sync rclone help backends`

	// Override rclone's os.Exit behavior
	rcloneRoot.PersistentPostRun = func(cmd *cobra.Command, args []string) {}
	rcloneRoot.SilenceErrors = true
	rcloneRoot.SilenceUsage = true

	// Rclone sets os.Args internally; override for subcommand usage
	rcloneRoot.PersistentPreRun = func(c *cobra.Command, args []string) {
		// Ensure rclone sees the right args
		_ = os.Setenv("RCLONE_CONFIG", "")
	}

	return rcloneRoot
}
