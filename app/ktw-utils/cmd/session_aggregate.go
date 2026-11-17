// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/spf13/cobra"
)

var aggregateJSON bool

func init() {
	sessionAggregateCmd.Flags().BoolVar(&aggregateJSON, "json", false, "Output raw JSON")
	sessionCmd.AddCommand(sessionAggregateCmd)
}

var sessionAggregateCmd = &cobra.Command{
	Use:   "aggregate [session-id]",
	Short: "Sum metrics across the entire delegation tree",
	Long: `Build the delegation tree for a session and aggregate token, tool, and
skill usage across all agents (parent + descendants).

Output includes total tokens, tools by name, skills loaded by each agent,
and a flat per-agent breakdown.

If no session-id is given, uses the most recent top-level session.`,
	RunE: runSessionAggregate,
}

func runSessionAggregate(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()

	rootID := ""
	if len(args) > 0 {
		rootID = args[0]
	}

	tree, err := journal.BuildDelegationTree(sessionsDir, rootID)
	if err != nil {
		return fmt.Errorf("build delegation tree: %w", err)
	}

	agg, err := journal.AggregateTree(tree)
	if err != nil {
		return fmt.Errorf("aggregate: %w", err)
	}

	if aggregateJSON {
		return printJSON(agg)
	}

	fmt.Print(formatAggregate(agg))
	return nil
}

func formatAggregate(agg *journal.AggregateMetrics) string {
	out := fmt.Sprintf("Delegation tree aggregate for: %s\n", agg.RootSessionID)
	out += fmt.Sprintf("─────────────────────────────────────────\n")
	out += fmt.Sprintf("Agents:        %d\n", agg.AgentCount)
	out += fmt.Sprintf("Total turns:   %d\n", agg.TotalTurns)
	out += fmt.Sprintf("Total tokens:  %d (in: %d, out: %d)\n", agg.TotalTokens, agg.InputTokens, agg.OutputTokens)
	out += fmt.Sprintf("Total time:    %d ms\n", agg.TotalDurationMs)
	out += fmt.Sprintf("Errors:        %d\n", agg.TotalErrors)

	if len(agg.ToolsByName) > 0 {
		out += "\nTools used:\n"
		// Sort by count desc
		type tc struct {
			name  string
			count int
		}
		var tools []tc
		for name, count := range agg.ToolsByName {
			tools = append(tools, tc{name, count})
		}
		sort.Slice(tools, func(i, j int) bool { return tools[i].count > tools[j].count })
		for _, t := range tools {
			out += fmt.Sprintf("  %-30s %d\n", t.name, t.count)
		}
	}

	if len(agg.SkillsByAgent) > 0 {
		out += "\nSkills loaded:\n"
		for agent, skills := range agg.SkillsByAgent {
			out += fmt.Sprintf("  %s: %v\n", agent, skills)
		}
	}

	out += "\nPer-agent breakdown:\n"
	for _, a := range agg.PerAgent {
		mode := ""
		if a.Mode != "" {
			mode = " [" + a.Mode + "]"
		}
		out += fmt.Sprintf("  %s%s — %d turns, %d tokens, %d ms, status=%s\n",
			a.Task, mode, a.Turns, a.TotalTokens, a.DurationMs, a.Status)
	}

	return out
}
