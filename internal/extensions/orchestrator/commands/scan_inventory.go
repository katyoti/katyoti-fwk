// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewScanInventoryCommand returns `ktw-univers scan-inventory` — fresh scan
// of available sidecar binaries on the system.
func NewScanInventoryCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "scan-inventory",
		Short: "Scan the system for available sidecar binaries",
		Long:  `Walks $PATH (and project-local app/ in dev mode) for ktw-* binaries and prints what's available. Used by the universe-creation wizard to populate sidecar selection.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			out, err := svc.ScanInventory(cmd.Context(), api.ScanInventoryInput{})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(cmd.OutOrStdout(), out)
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tPATH\tVERSION\tSOURCE")
			for _, s := range out.Sidecars {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, s.BinaryPath, s.Version, s.Source)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON instead of human-readable table")
	return cmd
}
