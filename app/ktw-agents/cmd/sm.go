// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/spf13/cobra"
)

const defaultSMPort = 7770

func init() {
	var port int

	smCmd := &cobra.Command{
		Use:   "sm",
		Short: "Manage the Session Manager server",
	}

	var fg bool
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the Session Manager server",
		Long:  "Start the Session Manager. Runs in the background by default; use --fg for foreground.",
		RunE: func(cmd *cobra.Command, args []string) error {
			lc := ktslocal.NewSMLifecycle(version, port)
			if fg {
				return lc.StartForeground()
			}
			return lc.StartBackgroundProcess()
		},
	}
	startCmd.Flags().BoolVar(&fg, "fg", false, "Run in foreground")
	startCmd.Flags().IntVar(&port, "port", defaultSMPort, "WebSocket port for the Session Manager")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the Session Manager server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ktslocal.NewSMLifecycle(version, defaultSMPort).DoStop()
		},
	}

	var restartFg bool
	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the Session Manager server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ktslocal.NewSMLifecycle(version, port).DoRestart(restartFg)
		},
	}
	restartCmd.Flags().BoolVar(&restartFg, "fg", false, "Run in foreground after restart")
	restartCmd.Flags().IntVar(&port, "port", defaultSMPort, "WebSocket port for the Session Manager")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show Session Manager server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ktslocal.NewSMLifecycle(version, defaultSMPort).DoStatus()
		},
	}

	smCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
	rootCmd.AddCommand(smCmd)
}
