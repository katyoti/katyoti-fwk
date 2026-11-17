// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// NewCreateCommand returns `ktw-univers create <name> [flags]`.
//
// v1: flag-driven only (interactive prompts come with the gui-universe-picker
// wizard — this CLI is for scripting + power users).
func NewCreateCommand() *cobra.Command {
	var (
		displayName string
		description string
		icon        string
		color       string
		homePath    string
		withProject bool
		projectDir  string
		firstGuide  string
		noWelcome   bool
		sidecars    []string
		tags        []string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new universe",
		Long: `Creates <home>/.katyoti/ skeleton (config/Univers.yaml + .env + config/Settings.yaml + per-sidecar config dirs) for the path given by --home, or defaults to ~/.config/katyoti/universes/<name>/ if --home is omitted. Adds the universe to ~/.config/katyoti/universes.yaml.

Does NOT activate. Activation spawns a per-universe hub and is the GUI/launcher's job — this binary has no "activate" verb. Open it with: katyoti gui --universe <name>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := resolveSvc(cmd.Context())
			if err != nil {
				return err
			}
			name := args[0]
			// An empty display name is defaulted by the SERVICE, so both CLIs agree.

			if icon == "" {
				icon = strings.ToUpper(name[:1])
			}
			if color == "" {
				color = "violet"
			}

			selections := make([]api.CreateSidecarSelection, 0, len(sidecars))
			for _, s := range sidecars {
				// name=binary pins the binary; a bare name is both.
				name, binary, ok := strings.Cut(s, "=")
				if !ok {
					binary = name
				}
				selections = append(selections, api.CreateSidecarSelection{Name: name, BinaryPath: binary})
			}

			info, err := svc.CreateUniverse(cmd.Context(), api.CreateUniverseInput{
				Name:        name,
				DisplayName: displayName,
				Description: description,
				Icon:        icon,
				Color:       color,
				HomePath:    homePath,
				Sidecars:    selections,
				WithProject: withProject,
				ProjectDir:  projectDir,
				FirstGuide:  firstGuide,
				NoWelcome:   noWelcome,
				Tags:        tags,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created universe %q at %s\n", info.Summary.Name, info.Summary.HomePath)
			fmt.Fprintf(cmd.OutOrStdout(), "Sidecars: %d\n", len(info.Sidecars))
			// Activation spawns a per-universe hub, which is launcher/GUI
			// orchestration -- this binary has no `activate` verb and never did.
			// Pointing at one that does not exist is worse than saying nothing.
			fmt.Fprintf(cmd.OutOrStdout(), "Open it with `katyoti gui --universe %s`.\n", info.Summary.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&displayName, "display", "", "Human-readable name (default: capitalized <name>)")
	cmd.Flags().StringVar(&description, "description", "", "Optional description")
	cmd.Flags().StringVar(&icon, "icon", "", "1-2 char IconRibbon icon (default: first char of name uppercased)")
	cmd.Flags().StringVar(&color, "color", "", "Mantine color preset: violet, blue, teal, lime, yellow, orange, red, pink, grape (default: violet)")
	cmd.Flags().StringVar(&homePath, "home", "", "Absolute path to the universe home folder (default: ~/.config/katyoti/universes/<name>/)")
	cmd.Flags().StringSliceVar(&sidecars, "sidecar", nil, "Sidecar to include (repeatable): its name, or name=<absolute binary path> to pin its binary. Use scan-inventory to see available sidecars.")
	cmd.Flags().StringVar(&firstGuide, "first-guide", "", "The guide the universe starts with: a guide-recipe URI of one of its sidecars (<sidecar>:/guide-recipe/<id>)")
	cmd.Flags().BoolVar(&noWelcome, "no-welcome", false, "Create it with its welcome screens off: its GUI opens neither its first guide nor Quoi de neuf by itself (a test or a screen capture's universe)")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "What the universe is for: a universe-tag one of its sidecars declares (repeatable); its GUI shows what the tags allow")
	cmd.Flags().BoolVar(&withProject, "with-project", false, "Give the universe a project of its own (<projects folder>/<name>, or --project): created and attached")
	cmd.Flags().StringVar(&projectDir, "project", "", "Folder of the universe's project (implies --with-project): created when missing and attached; refused inside the installation")
	return cmd
}
