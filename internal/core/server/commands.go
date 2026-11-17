// SPDX-License-Identifier: Apache-2.0

package server

import (
	"strings"

	"github.com/spf13/cobra"
)

// LifecycleFactory creates a Lifecycle on demand. Used by CobraCommandsLazy
// to defer Lifecycle construction until command execution (after settings/flags
// are resolved).
type LifecycleFactory func() (*Lifecycle, error)

// CobraCommands generates a parent command with start/stop/restart/status
// subcommands for the lifecycle. The parent command uses l.SubCommand as its
// Use string (e.g., "sm" or "server").
// Use this when the Lifecycle can be fully constructed at init time.
func (l *Lifecycle) CobraCommands() *cobra.Command {
	return CobraCommandsLazy(l.Name, l.SubCommand, func() (*Lifecycle, error) {
		return l, nil
	})
}

// CobraCommandsLazy generates start/stop/restart/status subcommands that call
// factory() at execution time to get a fresh Lifecycle. Use this when the
// Lifecycle depends on runtime settings or flags (e.g., loaded from .env or
// cobra flag values).
func CobraCommandsLazy(name, subCmd string, factory LifecycleFactory) *cobra.Command {
	parent := &cobra.Command{
		Use:   subCmd,
		Short: "Manage the " + name + " server",
	}

	var fg bool

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start the " + name + " server",
		Long:  "Start the " + name + " server. Runs in the background by default; use --fg for foreground.",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := factory()
			if err != nil {
				return err
			}
			if fg {
				return l.StartForeground()
			}
			// Derive the foreground re-exec args from this command's
			// actual path so the background spawn works regardless of
			// where the command is mounted (root, or promoted under the
			// `sidecar` group). cmd.CommandPath() => "<bin> sidecar server start";
			// drop the binary name and append --fg.
			if parts := strings.Fields(cmd.CommandPath()); len(parts) > 1 {
				l.FgArgs = append(append([]string{}, parts[1:]...), "--fg")
			}
			return l.StartBackgroundProcess()
		},
	}
	startCmd.Flags().BoolVar(&fg, "fg", false, "Run server in foreground")

	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the " + name + " server",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := factory()
			if err != nil {
				return err
			}
			return l.DoStop()
		},
	}

	var restartFg bool
	restartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart the " + name + " server",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := factory()
			if err != nil {
				return err
			}
			// Background restart re-execs `<path> start --fg`. Derive the
			// path from this command (".. server restart") by swapping the
			// trailing verb so it works wherever the group is mounted.
			if parts := strings.Fields(cmd.CommandPath()); len(parts) > 1 {
				base := append([]string{}, parts[1:len(parts)-1]...)
				l.FgArgs = append(append(base, "start"), "--fg")
			}
			return l.DoRestart(restartFg)
		},
	}
	restartCmd.Flags().BoolVar(&restartFg, "fg", false, "Run server in foreground after restart")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show " + name + " server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := factory()
			if err != nil {
				return err
			}
			return l.DoStatus()
		},
	}

	parent.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
	return parent
}
