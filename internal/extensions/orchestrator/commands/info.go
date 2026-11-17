// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewInfoCommand returns `ktw-univers info <name>` — print full details for
// a named universe (its .katyoti/workspace.yaml contents).
func NewInfoCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "info <name>",
		Short: "Show full details for a named universe",
		Long:  `Reads <home>/.katyoti/workspace.yaml for the universe and prints the full topology (sidecars, layered homes if any). Use --json for machine-readable output.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			info, err := svc.GetUniverse(cmd.Context(), api.GetUniverseInput{Name: args[0]})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(cmd.OutOrStdout(), info)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Name:        %s\n", info.Summary.Name)
			fmt.Fprintf(out, "DisplayName: %s\n", info.Summary.DisplayName)
			fmt.Fprintf(out, "Description: %s\n", info.Summary.Description)
			fmt.Fprintf(out, "Icon:        %s\n", info.Summary.Icon)
			fmt.Fprintf(out, "Color:       %s\n", info.Summary.Color)
			fmt.Fprintf(out, "HomePath:    %s\n", info.Summary.HomePath)
			fmt.Fprintf(out, "Protected:   %v\n", info.Summary.Protected)
			fmt.Fprintln(out)
			fmt.Fprintf(out, "Sidecars (%d):\n", len(info.Sidecars))
			for _, s := range info.Sidecars {
				fmt.Fprintf(out, "  - %s [%s]  binary=%s  config=%s\n", s.Name, s.Transport, s.Binary, s.ConfigDir)
			}
			if len(info.LayeredHomes) > 0 {
				fmt.Fprintln(out)
				fmt.Fprintf(out, "Layered homes (%d):\n", len(info.LayeredHomes))
				for _, h := range info.LayeredHomes {
					fmt.Fprintf(out, "  - %s\n", h)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON instead of human-readable text")
	return cmd
}
