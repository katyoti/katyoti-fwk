// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewProjectCommand builds the `project` group — a project (a working folder)
// attaches to a CENTRAL universe, which supplies its config/binaries; the
// project keeps only its own data (CWM/docs). N projects : 1 universe.
func NewProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Create, attach or detach a project folder of a central universe",
		Long: `A project is a working folder that ATTACHES to a central universe by name.
The universe supplies pinned sidecars + shared config/keys; the project holds
only its own authored data (CWM graph, documents). Attaching writes
<project>/.katyoti/project.yaml (` + "`universe: <name>`" + `).`,
	}
	cmd.AddCommand(projectCreateCmd(), projectAttachCmd(), projectDetachCmd(), projectEarlierCmd(), projectImportCmd())
	return cmd
}

func projectCreateCmd() *cobra.Command {
	var universe string
	var check bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new project under its name in the projects folder",
		Long: `Makes <projects folder>/<name> and, with --universe, attaches it. Refused
before anything is created when the name is not lowercase letters, digits and
dashes, a project of that name is registered, or its folder already exists - the
rule of a universe name. --check only says where it would go and why it is
refused.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			out, err := s.CreateProject(cmd.Context(), orchapi.CreateProjectInput{Name: args[0], Universe: universe, Check: check})
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			switch {
			case check && out.Refused != "":
				fmt.Fprintf(w, "refused: %s\n", out.Refused)
			case check:
				fmt.Fprintf(w, "%s would be created in %s\n", out.Name, out.ProjectDir)
			case out.Universe != "":
				fmt.Fprintf(w, "created %s, attached to universe %q\n", out.ProjectDir, out.Universe)
			default:
				fmt.Fprintf(w, "created %s (attach it: katyoti project attach %s --universe <name>)\n", out.ProjectDir, quoteArg(out.ProjectDir))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "the universe to attach the new project to")
	cmd.Flags().BoolVar(&check, "check", false, "only say where it would go and why it is refused: nothing is created")
	return cmd
}

func projectAttachCmd() *cobra.Command {
	var universe string
	cmd := &cobra.Command{
		Use:   "attach [path]",
		Short: "Link this project folder to a central universe",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			if universe == "" {
				return fmt.Errorf("--universe <name> is required")
			}
			dir, err := projectDir(args)
			if err != nil {
				return err
			}
			if _, err := s.AttachProject(cmd.Context(), orchapi.AttachProjectInput{ProjectDir: dir, Universe: universe}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "attached %s → universe %q\n  (%s/.katyoti/project.yaml written; config now resolves from the central universe)\n", dir, universe, dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "the central universe to attach to (required)")
	return cmd
}

func projectDetachCmd() *cobra.Command {
	var universe string
	cmd := &cobra.Command{
		Use:   "detach [path]",
		Short: "Unlink this project from its universe (revert to self-contained)",
		Long: "Unlink a project folder from its universe: the one --universe names, else the one the\n" +
			"folder's .katyoti/project.yaml names, else a universe that lists the folder. A folder\n" +
			"that no longer exists is detached from the universe named.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveUniverse(cmd)
			if err != nil {
				return err
			}
			dir, err := projectDir(args)
			if err != nil {
				return err
			}
			if _, err := s.DetachProject(cmd.Context(), orchapi.DetachProjectInput{ProjectDir: dir, Universe: universe}); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "detached %s (project.yaml removed; now self-contained)\n", dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&universe, "universe", "", "the universe to detach the folder from (needed when the folder no longer exists)")
	return cmd
}

func projectEarlierCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "earlier",
		Short: "List the projects of earlier installations, to import",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			out, err := s.EarlierProjects(cmd.Context())
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if len(out.Projects) == 0 {
				fmt.Fprintln(w, "no project in an earlier installation")
			}
			for _, p := range out.Projects {
				state := "katyoti project import " + quoteArg(p.Folder)
				if p.ImportedTo != "" {
					state = "imported to " + p.ImportedTo
				}
				fmt.Fprintf(w, "%s  (%s, universe %s, data version %d)\n  %s\n  %s\n", p.Name, p.Installation, p.Universe, p.DataVersion, p.Folder, state)
			}
			for _, sk := range out.Skipped {
				fmt.Fprintf(w, "skipped: %s\n", sk)
			}
			return nil
		},
	}
}

func projectImportCmd() *cobra.Command {
	var name, universe string
	cmd := &cobra.Command{
		Use:   "import <folder>",
		Short: "Import an earlier installation's project as a copy in the projects folder",
		Long: `Copies the project into the installation's projects folder and migrates the
copy to this release's data version. The original is only read: a failed
migration removes the copy and leaves the original as it was. With --universe
the copy is attached to that universe.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			out, err := s.ImportProject(cmd.Context(), mktapi.ImportProjectInput{Folder: args[0], Name: name, Universe: universe})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported %s → %s (data version %d → %d)\n", args[0], out.Project, out.From, out.To)
			if out.Universe != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  attached to universe %q\n", out.Universe)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "its name in the projects folder (default: its folder's name)")
	cmd.Flags().StringVar(&universe, "universe", "", "a universe to attach the copy to")
	return cmd
}

// quoteArg quotes a path holding a space for a command line.
func quoteArg(p string) string {
	if strings.ContainsAny(p, " \t") {
		return `"` + p + `"`
	}
	return p
}

// projectDir returns the target project folder — the positional arg, else cwd.
func projectDir(args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	return os.Getwd()
}
