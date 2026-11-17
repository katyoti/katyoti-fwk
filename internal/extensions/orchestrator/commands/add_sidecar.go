// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	api "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/spf13/cobra"
)

// NewAddSidecarCommand wires `ktw-univers add-sidecar <universe> <sidecar>`.
func NewAddSidecarCommand() *cobra.Command {
	var binary string
	cmd := &cobra.Command{
		Use:   "add-sidecar <universe> <sidecar>",
		Short: "Add a sidecar to an existing universe",
		Long: `Appends a sidecar row to the universe's Univers.yaml (transport nats,
args "sc ktsp nats"). Use --binary to pin the absolute binary path; when
given it must exist. The write is atomic.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			info, err := svc.AddSidecar(cmd.Context(), api.AddSidecarInput{
				Universe: args[0],
				Sidecar:  api.CreateSidecarSelection{Name: args[1], BinaryPath: binary},
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added sidecar %q to universe %q (%d members)\n",
				args[1], args[0], len(info.Sidecars))
			return nil
		},
	}
	cmd.Flags().StringVar(&binary, "binary", "", "Absolute path to the sidecar binary (validated when given)")
	return cmd
}
