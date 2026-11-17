// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/inspect"
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Live KTSP infrastructure monitor",
		Long: `Connects to a running ktw-agents session via WebSocket and shows
live container topology and journal events.

Example:
  # Start ktw-agents with WebSocket enabled:
  ktw-agents run hello-world --ws-port 7770

  # Then in another terminal:
  ktw-utils inspect --connect ws://localhost:7770/ktsp`,
		RunE: func(cmd *cobra.Command, args []string) error {
			wsURL, _ := cmd.Flags().GetString("connect")
			sessionID, _ := cmd.Flags().GetString("session-id")
			return inspect.Run(cmd.Context(), wsURL, sessionID)
		},
	}
	cmd.Flags().String("connect", "ws://localhost:7770/ktsp", "KTSP WebSocket URL")
	cmd.Flags().String("session-id", "", "Session ID to inspect (default: first active session)")
	rootCmd.AddCommand(cmd)
}
