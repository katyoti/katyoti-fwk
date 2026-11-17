// SPDX-License-Identifier: Apache-2.0

// Package commands provides the `connections` Cobra command group exposed
// by the bus host (ktw-hub). The connections command surface lets
// operators see the live state of every sidecar declaration without
// triggering a spawn.
//
//	ktw-hub connections list             # one-line status per declared sidecar
//	ktw-hub connections status <name>    # full ConnStatus for a binary
//	ktw-hub connections close            # universe deactivation: SIGTERM all
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/spf13/cobra"
)

// NewConnectionsCommand builds the `connections` command group. The
// ConnectionManager service must be published in the application —
// typically by the bus host's bootstrap.
func NewConnectionsCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "connections",
		Short: "Inspect the per-sidecar connection lifecycle (DECLARED / SPAWNING / READY / FAILED)",
		Long: `The connection manager owns one entry per sidecar declared in this
universe's Univers.yaml (its sidecars: block) and tracks its FSM state. Read-only at this
command surface — use 'connections close' to terminate all live entries
during universe deactivation.`,
	}
	root.AddCommand(newConnectionsListCommand())
	root.AddCommand(newConnectionsStatusCommand())
	root.AddCommand(newConnectionsCloseCommand())
	return root
}

func newConnectionsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List every declared connection's state (JSON array)",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := application.UseService[cmapi.ConnectionManager](cmd.Context())
			if err != nil {
				return fmt.Errorf("connections: service not published: %w", err)
			}
			out, err := json.MarshalIndent(mgr.All(), "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

func newConnectionsStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status <name>",
		Short: "Print the full ConnStatus for one declared sidecar",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := application.UseService[cmapi.ConnectionManager](cmd.Context())
			if err != nil {
				return fmt.Errorf("connections: service not published: %w", err)
			}
			st, ok := mgr.Status(args[0])
			if !ok {
				return fmt.Errorf("connections: unknown member %q", args[0])
			}
			out, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return nil
		},
	}
}

func newConnectionsCloseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "close",
		Short: "Close every live connection (universe deactivation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := application.UseService[cmapi.ConnectionManager](cmd.Context())
			if err != nil {
				return fmt.Errorf("connections: service not published: %w", err)
			}
			if err := mgr.Close(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return nil
		},
	}
}
