// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewListCommand returns `ktw-univers list` — print all registered universes.
func NewListCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all registered universes",
		Long:  `Reads ~/.config/katyoti/universes.yaml and prints the universes (name, display, home path, default flag, running flag). Use --json for machine-readable output.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			out, err := svc.ListUniverses(cmd.Context(), api.ListUniversesInput{})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(cmd.OutOrStdout(), out)
			}
			return universeTable(cmd.OutOrStdout(), out.Universes)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON instead of a human-readable table")
	return cmd
}
