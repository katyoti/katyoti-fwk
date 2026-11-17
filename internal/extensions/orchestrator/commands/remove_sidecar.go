// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/spf13/cobra"
)

// NewRemoveSidecarCommand wires `ktw-univers remove-sidecar <universe> <sidecar>`.
func NewRemoveSidecarCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-sidecar <universe> <sidecar>",
		Short: "Remove a sidecar from an existing universe",
		Long: `Removes the named sidecar's row from the universe's Univers.yaml.
Errors if the universe or the sidecar is not present. The write is atomic.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			info, err := svc.RemoveSidecar(cmd.Context(), api.RemoveSidecarInput{
				Universe: args[0],
				Sidecar:  args[1],
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed sidecar %q from universe %q (%d members)\n",
				args[1], args[0], len(info.Sidecars))
			return nil
		},
	}
	return cmd
}
