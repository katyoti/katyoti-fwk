// SPDX-License-Identifier: Apache-2.0

// Package commands is the method assistant's command line: method-overview
// ("where are we / what's next") and method-act (run the top action), over
// the whole workspace. They read and write through the same gather and the
// same actions as the method panels' verbs — one reading of the method.
package commands

import (
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/assistant"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	macore "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/internal/core"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	"github.com/spf13/cobra"
)

// NewMethodOverviewCommand prints the overview, the alignment findings
// (GAP / WASTE / ORPHAN) and the ranked next actions of the whole workspace,
// through the in-process CWMService. Read-only.
func NewMethodOverviewCommand() *cobra.Command {
	var release string
	cmd := &cobra.Command{
		Use:   "method-overview",
		Short: "Method-core assistant view — where are we / what's next (read-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cwm, err := application.UseService[cwmapi.CWMService](ctx)
			if err != nil {
				return fmt.Errorf("method-overview: resolve CWMService: %w", err)
			}
			snap, err := macore.Gather(ctx, cwm, "")
			if err != nil {
				return fmt.Errorf("method-overview: gather: %w", err)
			}
			if release != "" {
				ids, err := scopeOf(cmd, release)
				if err != nil {
					return fmt.Errorf("method-overview: the scope of %s: %w", release, err)
				}
				if snap, _, err = macore.KeepScope(snap, ids, macore.ParentReader(ctx, cwm)); err != nil {
					return err
				}
			}
			printReport(cmd, snap)
			return nil
		},
	}
	cmd.Flags().StringVar(&release, "release", "", "Only the work aimed at this release or increment (a project release: its module releases' and increments' too)")
	return cmd
}

// scopeOf is what a release - or an increment - holds, read from
// planification in this process.
func scopeOf(cmd *cobra.Command, id string) ([]string, error) {
	ctx := cmd.Context()
	var out planapi.ScopeOutput
	rels, err := application.UseService[planapi.ReleaseService](ctx)
	if err == nil {
		out, err = rels.Scope(ctx, planapi.GetInput{ID: id})
	}
	if err != nil {
		incs, incErr := application.UseService[planapi.IncrementService](ctx)
		if incErr != nil {
			return nil, err
		}
		if out, incErr = incs.Scope(ctx, planapi.GetInput{ID: id}); incErr != nil {
			return nil, err
		}
	}
	ids := make([]string, 0, len(out.Targets))
	for _, t := range out.Targets {
		ids = append(ids, t.ID)
	}
	return ids, nil
}

// NewMethodActCommand runs the top What's Next action of the whole workspace:
// an alignment fix (shape-us / review-waste) is written to CWM;
// creative work names the skill to launch.
func NewMethodActCommand() *cobra.Command {
	var kind string
	cmd := &cobra.Command{
		Use:   "method-act",
		Short: "Run the top What's-Next action (deterministic alignment fixes run; creative ones name the agent skill)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cwm, err := application.UseService[cwmapi.CWMService](ctx)
			if err != nil {
				return fmt.Errorf("method-act: resolve CWMService: %w", err)
			}
			snap, err := macore.Gather(ctx, cwm, "")
			if err != nil {
				return fmt.Errorf("method-act: gather: %w", err)
			}
			w := cmd.OutOrStdout()
			top, found := macore.TopAction(snap, kind)
			if !found {
				fmt.Fprintln(w, "no matching action.")
				return nil
			}
			fmt.Fprintf(w, "TOP ACTION: [%s] %s\n  → %s\n\n", top.Kind, top.Title, top.Rationale)
			msg, _ := macore.RunAction(ctx, cwm, top, snap)
			fmt.Fprintf(w, "%s\n\nRe-run `method-overview` to see What's Next update.\n", msg)
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "Act on the top action of this kind (default: the top action overall)")
	return cmd
}

func printReport(cmd *cobra.Command, snap assistant.Snapshot) {
	w := cmd.OutOrStdout()
	ov := assistant.ComputeOverview(snap)
	fmt.Fprintf(w, "WHERE ARE WE\n")
	fmt.Fprintf(w, "  features:  %d delivered / %d total\n", ov.USDelivered, ov.USTotal)
	fmt.Fprintf(w, "  criteria:  %d pass · %d no-test · %d failing · %d could not run (of %d)\n",
		ov.CriteriaPassing, ov.CriteriaNoTest, ov.CriteriaFailing, ov.CriteriaUnproven, ov.CriteriaTotal)
	for _, g := range ov.GoalGauges {
		fmt.Fprintf(w, "  goal %-24s served-by %d features · %d delivered\n", trunc(g.Title, 24), g.ServingUS, g.DeliveredUS)
	}

	findings := assistant.Findings(snap)
	fmt.Fprintf(w, "\nALIGNMENT (%d)\n", len(findings))
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Kind < findings[j].Kind })
	for _, f := range findings {
		fmt.Fprintf(w, "  [%-6s] %s — %s\n", f.Kind, trunc(f.Title, 30), f.Message)
	}

	actions := assistant.NextActions(snap)
	fmt.Fprintf(w, "\nWHAT'S NEXT (ranked)\n")
	for i, a := range actions {
		fmt.Fprintf(w, "  %d. [%-14s score %4d] %s\n        → %s\n", i+1, a.Kind, a.Score, trunc(a.Title, 30), a.Rationale)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
