// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// resolveUniverse resolves the registry-backed UniverseService (orchestrator) —
// the SINGLE universe owner. Universe create/list are ITS concern, not the
// marketplace's; the `universe` group only borrows the command surface.
func resolveUniverse(cmd *cobra.Command) (orchapi.OrchestratorService, error) {
	return application.UseService[orchapi.OrchestratorService](cmd.Context())
}

// NewUniverseCommand builds the `universe` group (currently: wire).
func NewUniverseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "universe",
		Short: "Wire installed items into a universe (→ used)",
		Long: `Move an installed item from AVAILABLE to USED by wiring it into a
universe's Univers.yaml. ktw-univers still owns universe create/list/delete;
this only edits membership + configures the item.`,
	}
	cmd.AddCommand(universeCreateCmd())
	cmd.AddCommand(universeListCmd())
	cmd.AddCommand(universeWireCmd())
	cmd.AddCommand(universeUnwireCmd())
	cmd.AddCommand(universeProjectsCmd())
	cmd.AddCommand(universeDeleteCmd())
	cmd.AddCommand(universeStartCmd())
	cmd.AddCommand(universeStopCmd())
	cmd.AddCommand(universeRestartCmd())
	cmd.AddCommand(universeStatusCmd())
	cmd.AddCommand(universeSetSolutionCmd())
	return cmd
}

// universeSetSolutionCmd gives a universe made before solutions - its
// Univers.yaml names none - an installed solution, through
// the orchestrator's SetUniverseSolution: the refusals (a solution not
// installed, a version not installed, a name installed in several versions,
// a universe that has one) are the method's, said as they are.
func universeSetSolutionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-solution <universe> <solution>[@<version>]",
		Short: "Give a universe with no solution an installed one (a universe made before solutions)",
		Long: `Writes solution: <name>@<version> into the universe's Univers.yaml, every
other field kept, so a project whose type needs that solution opens in it. A
name alone takes the version installed on this machine; name the version when
several are. Refused for a solution not installed here and for a universe that
already has one - moving it to another version is ` + "`katyoti solution update`" + `.
A universe that runs reads it at its next start.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			info, err := s.SetUniverseSolution(cmd.Context(), orchapi.SetUniverseSolutionInput{Name: args[0], Solution: args[1]})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "universe %q now belongs to the solution %s (a running universe reads it at its next start)\n", args[0], info.Summary.Solution)
			return nil
		},
	}
}

func universeCreateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a central universe (~/.config/katyoti/universes/<name>/)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			// name-only CLI: the SERVICE defaults both DisplayName and HomePath.
			// Defaulting here too is how the two CLIs drifted apart.
			info, err := s.CreateUniverse(cmd.Context(), orchapi.CreateUniverseInput{
				Name: args[0],
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created universe %q at %s\n  (wire sidecars with `universe wire`, then `project attach . --universe %s`)\n", args[0], info.Summary.HomePath, args[0])
			return nil
		},
	}
}

func universeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List central universes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			out, err := s.ListUniverses(cmd.Context(), orchapi.ListUniversesInput{})
			if err != nil {
				return err
			}
			if len(out.Universes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no central universes — create one with `universe create <name>`")
				return nil
			}
			for _, u := range out.Universes {
				fmt.Fprintln(cmd.OutOrStdout(), u.Name)
			}
			return nil
		},
	}
}

func universeProjectsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "projects <universe>",
		Short: "List the projects attached to a central universe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			out, err := s.ListProjects(cmd.Context(), orchapi.ListProjectsInput{Universe: args[0]})
			if err != nil {
				return err
			}
			if len(out.Projects) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "no projects attached to universe %q\n", args[0])
				return nil
			}
			for _, p := range out.Projects {
				fmt.Fprintln(cmd.OutOrStdout(), p)
			}
			return nil
		},
	}
}

// resolveUniverseHome turns the --universe flag into a workspace FOLDER.
//
// At the API level this flag IS a folder (WireIntoUniverse takes a UniverseHome),
// but every other command in this group takes a NAME -- and `universe create`
// signs off by telling you to run `universe wire` next, which walked you straight
// into passing a name where a path was expected. A registered name is what the
// user means, so it wins; anything else still works as a path, so existing
// scripts keep running.
//
// A value that is neither is reported HERE. Passing it through as a relative path
// used to surface three layers down as "Univers.yaml is missing required field
// 'name'" -- an error about a file the user never wrote, at a path they never named.
func resolveUniverseHome(cmd *cobra.Command, flag string) (string, error) {
	if flag == "" {
		return "", nil // current workspace
	}
	if svc, err := resolveUniverse(cmd); err == nil {
		if info, gerr := svc.GetUniverse(cmd.Context(), orchapi.GetUniverseInput{Name: flag}); gerr == nil {
			return info.Summary.HomePath, nil
		}
	}
	if _, err := os.Stat(flag); err != nil {
		return "", fmt.Errorf(
			"universe %q is neither a registered universe nor an existing folder (see `universe list`)", flag)
	}
	return flag, nil
}

// resolveUniverseTarget is resolveUniverseHome for commands that may CREATE the
// universe (`bootstrap`), where "does not exist yet" is the normal case and must
// not be an error.
//
// A registered NAME still wins, which is the whole point: `--universe demo`
// meant `<cwd>/demo` here while meaning the registered universe everywhere else,
// so `config-set apply --universe demo` silently built a brand-new universe in
// the current directory and reported success against it. The real universe was
// never touched, and nothing said so.
func resolveUniverseTarget(cmd *cobra.Command, flag string) string {
	if flag == "" {
		return ""
	}
	if svc, err := resolveUniverse(cmd); err == nil {
		if info, gerr := svc.GetUniverse(cmd.Context(), orchapi.GetUniverseInput{Name: flag}); gerr == nil {
			return info.Summary.HomePath
		}
	}
	return flag
}

func universeDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Archive a universe and remove it from the registry",
		Long: `Moves the universe folder to <parent>/.archived/<name>/ and drops the
registry entry. Protected universes (admin) are refused, as is a universe whose
hub is still running -- deactivate it first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			out, err := s.DeleteUniverse(cmd.Context(), orchapi.DeleteUniverseInput{Name: args[0]})
			if err != nil {
				return err
			}
			if !out.Archived {
				fmt.Fprintf(cmd.OutOrStdout(), "universe %q was not in the registry; nothing to do\n", args[0])
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "archived universe %q to %s\n", args[0], out.ArchivePath)
			return nil
		},
	}
}

func universeWireCmd() *cobra.Command {
	var universe string
	cmd := &cobra.Command{
		Use:   "wire <item>",
		Short: "Wire an installed item into a universe (AddSidecar + configure)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			home, err := resolveUniverseHome(cmd, universe)
			if err != nil {
				return err
			}
			res, err := s.WireIntoUniverse(cmd.Context(), mktapi.WireIntoUniverseInput{Name: args[0], UniverseHome: home})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wired %s into universe at %s\n  (Univers.yaml updated + configured — run `configure %s` for remaining next-steps)\n", args[0], res.Home, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "universe name (or workspace folder) to wire into (default: the current workspace)")
	return cmd
}

func universeUnwireCmd() *cobra.Command {
	var universe string
	cmd := &cobra.Command{
		Use:   "unwire <item>",
		Short: "Remove an item from a universe (USED → available)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			home, err := resolveUniverseHome(cmd, universe)
			if err != nil {
				return err
			}
			res, err := s.Unwire(cmd.Context(), mktapi.UnwireInput{Name: args[0], UniverseHome: home})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unwired %s from universe at %s (Univers.yaml updated)\n", args[0], res.Home)
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "universe name (or workspace folder) to unwire from (default: the current workspace)")
	return cmd
}
