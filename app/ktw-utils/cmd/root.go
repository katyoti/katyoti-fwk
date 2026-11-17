// SPDX-License-Identifier: Apache-2.0

// Package cmd is the developer CLI: session analysis, the journal
// browser, the KTSP inspect TUI and gen-ktsp-client.
package cmd

import (
	// Blank-import cenv to load .env files and register the env resolver.
	_ "github.com/katyoti/katyoti-fwk/internal/core/cenv"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "ktw-utils",
	Short: "Utilities for ktw-agents sessions and journals",
}

func Execute() error {
	return rootCmd.Execute()
}
