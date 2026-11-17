// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewResolveObjectCommand returns `ktw-univers resolve-object <ref>` — the
// registry half of "which file is this" (identity-and-index.md §1): a full
// name from a log, or a bare project id, to the universe and folder that
// mount it. The object half is the universe core's `cwm object-resolve`.
func NewResolveObjectCommand() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "resolve-object <full-name | project-uuid>",
		Short: "Which universe mounts the project a full name belongs to, and where",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			out, err := svc.ResolveProject(cmd.Context(), api.ResolveProjectInput{Ref: args[0]})
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(cmd.OutOrStdout(), out)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "project   %s (%s)\nuniverse  %s\nfolder    %s\n", out.Name, out.ProjectID, out.Universe, out.ProjectDir)
			if out.ObjectID != "" {
				fmt.Fprintf(w, "object    %s\nnext      ktw-agents --universe %s cwm object-resolve %s\n", out.ObjectID, out.Universe, out.ObjectID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}
