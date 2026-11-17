// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// NewPrereqsCommand builds `prereqs plan | install`: the programs the
// sidecars of one or more universes need on this machine, planned from their
// declarations and installed by the engine (solutions.md §10).
func NewPrereqsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prereqs",
		Short: "The programs a universe's sidecars need: plan them, install them",
	}
	cmd.AddCommand(prereqsPlanCommand(), prereqsInstallCommand())
	return cmd
}

func prereqsUniverses(cmd *cobra.Command, flags []string) []string {
	var out []string
	for _, u := range flags {
		out = append(out, resolveUniverseTarget(cmd, u))
	}
	if len(out) == 0 {
		if u := defaultUniverse(); u != "" {
			out = []string{u}
		}
	}
	return out
}

func prereqsPlanCommand() *cobra.Command {
	var (
		universes []string
		asJSON    bool
	)
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "What the sidecars of the universes need on this machine, and how each program installs here",
		Long: `Reads each wired sidecar's config check and plans its programs: one row per
program, deduplicated across sidecars at the highest version, required or
optional, the install recipe chosen for this platform (user or administrator
scope). A program no recipe installs here names its install skill.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			plan, err := s.PlanPrereqs(cmd.Context(), mktapi.PrereqsPlanInput{Universes: prereqsUniverses(cmd, universes)})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), plan)
			}
			renderPlan(cmd.OutOrStdout(), plan)
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&universes, "universe", nil, "a universe whose sidecars are read: a registered name or its folder (repeatable; default: the default universe)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the plan as JSON")
	return cmd
}

func prereqsInstallCommand() *cobra.Command {
	var (
		universes []string
		dryRun    bool
		asJSON    bool
	)
	cmd := &cobra.Command{
		Use:   "install [program...]",
		Short: "Install the programs the sidecars need (default: every required one missing)",
		Long: `Installs the named programs, or every required program missing, in your own
profile; the steps that need administrator rights run together after one
prompt. A download is refused unless its sha256 matches. Then PATH is read
again and the sidecars are checked again. An optional program is installed
only when named.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolve(cmd)
			if err != nil {
				return err
			}
			res, err := s.InstallPrereqs(cmd.Context(), mktapi.PrereqsInstallInput{
				Universes: prereqsUniverses(cmd, universes), Programs: args, DryRun: dryRun,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), res)
			}
			o := cmd.OutOrStdout()
			if len(res.Steps) == 0 && len(res.Missing) == 0 {
				fmt.Fprintln(o, "nothing to install: every required program is present")
				return nil
			}
			w := tabwriter.NewWriter(o, 0, 2, 2, ' ', 0)
			fmt.Fprintln(w, "PROGRAM\tSTEP\tSCOPE\tSTATUS\tDETAIL")
			for _, st := range res.Steps {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", st.Program, dash(st.Kind), dash(st.Scope), st.Status, st.Detail)
			}
			_ = w.Flush()
			if res.ElevationPrompts > 0 {
				fmt.Fprintln(o, "administrator rights were asked once for the steps marked admin")
			}
			if res.After != nil {
				fmt.Fprintln(o, "\nchecked again:")
				renderPlan(o, *res.After)
			}
			for _, st := range res.Steps {
				if st.Status == mktapi.StepFailed {
					return fmt.Errorf("%s did not install", st.Program)
				}
			}
			// A required program no recipe installs here, or one the checks
			// still miss, leaves the universe not ready: the install fails.
			if len(res.Missing) > 0 && !dryRun {
				return fmt.Errorf("still missing: %s", strings.Join(res.Missing, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&universes, "universe", nil, "a universe whose sidecars are read: a registered name or its folder (repeatable; default: the default universe)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "say which steps would run; run none")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the result as JSON")
	return cmd
}

// renderPlan prints a plan as a table, then the fallbacks.
func renderPlan(o io.Writer, plan mktapi.PrereqsPlan) {
	if len(plan.Programs) == 0 {
		fmt.Fprintf(o, "no program declared by %d sidecar(s)\n", len(plan.Sidecars))
	} else {
		w := tabwriter.NewWriter(o, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "PROGRAM\tNEED\tVERSION\tSTATE\tSTEP\tSCOPE\tNEEDED BY")
		for _, p := range plan.Programs {
			need := "optional"
			if p.Required {
				need = "required"
			}
			step := "-"
			if p.Recipe != nil {
				step = p.Recipe.Kind + " " + p.Recipe.Package + p.Recipe.URL + p.Recipe.Script
			}
			state := p.State
			if p.After != "" {
				state += " after " + p.After
			}
			if p.State == mktapi.PrereqInstall && !p.Selected {
				state += " (offered)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, need, dash(p.Version), state, strings.TrimSpace(step), dash(p.Scope), strings.Join(p.NeededBy, ", "))
		}
		_ = w.Flush()
	}
	for _, p := range plan.Programs {
		if p.Fallback != "" {
			fmt.Fprintf(o, "  %s: %s\n", p.Name, p.Fallback)
		}
	}
	if len(plan.Elevation) > 0 {
		fmt.Fprintf(o, "administrator rights asked once for: %s\n", strings.Join(plan.Elevation, ", "))
	}
	for _, u := range plan.Unchecked {
		fmt.Fprintf(o, "  not checked: %s\n", u)
	}
}

func missingRequired(plan mktapi.PrereqsPlan) []string {
	var out []string
	for _, p := range plan.Programs {
		if p.Required && p.State != mktapi.PrereqOK {
			out = append(out, p.Name)
		}
	}
	return out
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func writeJSON(o io.Writer, v any) error {
	enc := json.NewEncoder(o)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
