// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/app/ktw-compose/internal/solutionform"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	orchapi "github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
)

// newComposeUniverseCommand wires the headline `ktw-compose compose-universe
// <name>` flow — the interactive sibling of apply-universe. Interactive by
// default (the solutionform over the orchestrator's scan-inventory; the
// published build has no form); `--from-spec` runs the same core
// non-interactively from a YAML spec. Both converge on ComposeService.ApplyUniverse.
func newComposeUniverseCommand() *cobra.Command {
	var fromSpec string
	var replace bool
	var skipBuild bool
	cmd := &cobra.Command{
		Use:   "compose-universe <name>",
		Short: "Compose a katyoti universe: pick sidecars, optionally scaffold a new files-only one, create the universe",
		Long: `The headline composition flow. Interactive by default: select existing
sidecars (from scan-inventory; not-installed sidecars cannot be selected —
install/build them first), optionally scaffold + build a new files-only
sidecar, then create the universe with every member.

Non-interactive: --from-spec spec.yaml describes the same choices upfront:

    display_name: My Solution
    sidecars:
      - name: ktw-docs
        binary: C:/path/bin/ktw-docs.exe   # optional
    new_sidecar:               # optional
      name: ktw-my-recipes
      include: skills,tasks
    replace: false`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := application.UseService[api.ComposeService](cmd.Context())
			if err != nil {
				return fmt.Errorf("compose service: %w", err)
			}

			var spec api.SolutionSpec
			if fromSpec != "" {
				raw, err := os.ReadFile(fromSpec)
				if err != nil {
					return fmt.Errorf("read spec: %w", err)
				}
				if err := yaml.Unmarshal(raw, &spec); err != nil {
					return fmt.Errorf("parse spec: %w", err)
				}
			} else {
				collected, err := collectSolutionSpec(cmd)
				if err != nil {
					return err
				}
				spec = collected
			}
			// CLI arg + flags win over spec-file values.
			spec.Name = args[0]
			if replace {
				spec.Replace = true
			}
			if skipBuild {
				spec.SkipBuild = true
			}

			out, err := svc.ApplyUniverse(cmd.Context(), api.ApplyUniverseInput{Spec: spec})
			for _, step := range out.Steps {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", step)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nSolution %q ready: %d sidecar(s).\n", spec.Name, len(out.Universe.Sidecars))
			if out.Scaffolded != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "\nScaffolded sidecar follow-up:\n%s\n", out.Scaffolded.Note)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nNext: activate it (ktw-univers / ktw-gui) to bring the universe online.\n")
			return nil
		},
	}
	cmd.Flags().StringVar(&fromSpec, "from-spec", "", "YAML spec file — run non-interactively")
	cmd.Flags().BoolVar(&replace, "replace", false, "Archive an existing universe of the same name first")
	cmd.Flags().BoolVar(&skipBuild, "skip-build", false, "Skip `go build` of the scaffolded sidecar")
	return cmd
}

// collectSolutionSpec drives the interactive flow (solutionform). Inventory is
// read from the orchestrator directly — listing is the orchestrator's surface,
// not a compose verb (verbs.md §4). A build without the form (the published
// one) answers at once, before any scan: the spec comes from --from-spec.
func collectSolutionSpec(cmd *cobra.Command) (api.SolutionSpec, error) {
	if !solutionform.Interactive {
		return api.SolutionSpec{}, solutionform.ErrNotInBuild
	}
	orch, err := application.UseService[orchapi.OrchestratorService](cmd.Context())
	if err != nil {
		return api.SolutionSpec{}, fmt.Errorf("orchestrator service: %w", err)
	}
	inv, err := orch.ScanInventory(cmd.Context(), orchapi.ScanInventoryInput{})
	if err != nil {
		return api.SolutionSpec{}, fmt.Errorf("scan inventory: %w", err)
	}
	available := make([]solutionform.Sidecar, 0, len(inv.Sidecars))
	for _, sc := range inv.Sidecars {
		available = append(available, solutionform.Sidecar{Name: sc.Name, Binary: sc.BinaryPath})
	}
	return solutionform.Collect(available)
}
