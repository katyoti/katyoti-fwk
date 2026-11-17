// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewSeedCommand builds `seed list | show <name> | apply <name>`: the content
// of a universe — a project (its CWM objects and the whole project folder) and
// the universe's tasks — applied to a universe like a config-set.
func NewSeedCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Browse and apply seeds: a universe's content (a project and its tasks) offered by your marketplaces",
	}
	cmd.AddCommand(newSeedListCommand(), newSeedShowCommand(), newSeedApplyCommand())
	return cmd
}

func newSeedListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List seeds across enabled marketplaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			seeds, err := s.ListSeeds(cmd.Context())
			if err != nil {
				return err
			}
			if len(seeds) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no seeds offered by the enabled marketplaces")
				return nil
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tLATEST\tMARKETPLACE\tDESCRIPTION")
			for _, c := range seeds {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Name, c.Latest, c.Marketplace, c.Description)
			}
			return w.Flush()
		},
	}
}

func newSeedShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show a seed's versions and the members it was made with",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			out, err := s.ShowSeed(cmd.Context(), mktapi.ShowSeedInput{Name: args[0]})
			if err != nil {
				return err
			}
			o := cmd.OutOrStdout()
			fmt.Fprintf(o, "%s", out.Name)
			if out.DisplayName != "" && out.DisplayName != out.Name {
				fmt.Fprintf(o, " — %s", out.DisplayName)
			}
			fmt.Fprintln(o)
			if out.Description != "" {
				fmt.Fprintf(o, "  %s\n", out.Description)
			}
			fmt.Fprintf(o, "  marketplace: %s\n  versions: %s\n", out.Marketplace, strings.Join(out.Versions, ", "))
			if len(out.Requires) > 0 {
				fmt.Fprintf(o, "  made with: %s\n", strings.Join(out.Requires, ", "))
			}
			return nil
		},
	}
}

func newSeedApplyCommand() *cobra.Command {
	var universe, project string
	cmd := &cobra.Command{
		Use:   "apply <seed>[@version] --universe <name|folder> [--project <folder>]",
		Short: "Lay a seed out into a universe: a new project (attached, its own identity) and the universe's tasks",
		Long: `Applies a seed to an existing universe. The universe must have the members the
seed was made with (a missing one refuses; a different version is reported). The
project goes into --project (default: <universe name>-project beside the universe);
a folder that already holds files is refused — a seed never overwrites a project.
The project gets its own identity (its project object re-minted).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(universe) == "" {
				return fmt.Errorf("name the universe the seed goes into (--universe <name|folder>)")
			}
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			ref := parseRef(args[0])
			out, err := s.ApplySeed(cmd.Context(), mktapi.ApplySeedInput{
				Name: ref.Name, Version: ref.Version, UniverseHome: resolveUniverseTarget(cmd, universe), ProjectDir: project,
			})
			for _, d := range out.Differs {
				fmt.Fprintf(cmd.ErrOrStderr(), "  note: made with another version — %s\n", d)
			}
			if err != nil {
				return err
			}
			o := cmd.OutOrStdout()
			fmt.Fprintf(o, "applied %s@%s to %s\n  project %s laid out and attached", out.Name, out.Version, out.UniverseHome, out.Project)
			if out.ProjectID != "" {
				fmt.Fprintf(o, " (project %s, its own identity)", out.ProjectID)
			}
			fmt.Fprintln(o)
			if out.Tasks > 0 {
				fmt.Fprintf(o, "  %d task(s) given to the universe\n", out.Tasks)
			}
			if out.NotRecorded != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "  note: not recorded as created on this PC - %s\n", out.NotRecorded)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "the universe: a registered name, a new name or a folder (required)")
	cmd.Flags().StringVar(&project, "project", "", "project folder (default: <universe name>-project beside the universe; one holding files is refused)")
	return cmd
}
