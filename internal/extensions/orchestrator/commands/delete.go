// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewDeleteCommand returns `ktw-univers delete <name>` — archive a universe.
func NewDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Archive a universe and remove it from the registry",
		Long:  `Moves the universe folder to <parent>/.archived/<name>/ (sibling of the original) and removes the registry entry. Errors if the universe is currently active (deactivate first).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			out, err := svc.DeleteUniverse(cmd.Context(), api.DeleteUniverseInput{Name: args[0]})
			if err != nil {
				return err
			}
			if !out.Archived {
				fmt.Fprintf(cmd.OutOrStdout(), "Universe %q was not in the registry; nothing to do.\n", args[0])
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Archived universe %q to %s\n", args[0], out.ArchivePath)
			return nil
		},
	}
	return cmd
}
