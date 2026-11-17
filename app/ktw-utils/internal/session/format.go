// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"
	"strings"
)

// FormatReport renders a SessionReport as the 3-question evaluation.
func FormatReport(r *SessionReport) string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== Session Evaluation: %s ===\n\n", r.SessionID)

	// 1. Outcome Quality.
	fmt.Fprintln(&b, "1. OUTCOME QUALITY")
	statusIcon := ""
	if r.Outcome.Status == "completed" {
		statusIcon = " ✓"
	}
	fmt.Fprintf(&b, "   Status:           %s%s\n", r.Outcome.Status, statusIcon)
	fmt.Fprintf(&b, "   Turns to goal:    %d\n", r.Outcome.Turns)
	fmt.Fprintf(&b, "   User corrections: %d\n", r.Outcome.Corrections)
	if r.Outcome.Abandoned {
		fmt.Fprintln(&b, "   Abandonment:      yes")
	}
	fmt.Fprintf(&b, "   → VERDICT: %s\n\n", r.Outcome.Verdict)

	// 2. Efficiency.
	fmt.Fprintln(&b, "2. EFFICIENCY")
	fmt.Fprintf(&b, "   Token usage:      %s (input: %s / output: %s)\n",
		fmtInt(r.Efficiency.TotalTokens),
		fmtInt(r.Efficiency.InputTokens),
		fmtInt(r.Efficiency.OutputTokens))
	fmt.Fprintf(&b, "   Content-only:     %d/%d turns (%.0f%%)\n",
		r.Efficiency.ContentOnlyTurns,
		r.Outcome.Turns,
		r.Efficiency.ContentOnlyRatio*100)
	if r.Efficiency.ToolsAvailable > 0 {
		fmt.Fprintf(&b, "   Unused tools:     %d/%d loaded",
			len(r.Efficiency.UnusedTools),
			r.Efficiency.ToolsAvailable)
		if len(r.Efficiency.UnusedTools) > 0 {
			fmt.Fprintf(&b, " (%s)", join(r.Efficiency.UnusedTools, ", "))
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintf(&b, "   Ask-user count:   %d", r.Efficiency.AskUserCount)
	if r.Efficiency.AvgUserResponseMs > 0 {
		fmt.Fprintf(&b, " (response: %s)", fmtDuration(r.Efficiency.AvgUserResponseMs))
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "   → VERDICT: %s\n", r.Efficiency.Verdict)
	for _, f := range r.Findings {
		if f.Type == FindingEfficiency {
			fmt.Fprintf(&b, "   → FINDING: [%s] %s\n", f.Type, f.Message)
		}
	}
	fmt.Fprintln(&b)

	// 3. Internal Health.
	fmt.Fprintln(&b, "3. INTERNAL HEALTH")
	fmt.Fprintf(&b, "   Tool errors:      %d\n", r.Health.ErrorCount)
	fmt.Fprintf(&b, "   Avg LLM latency:  %s\n", fmtDuration(r.Health.AvgLLMLatencyMs))
	fmt.Fprintf(&b, "   TTFT trend:       %s (%s)\n", fmtDuration(r.Health.AvgTTFTMs), r.Health.TTFTTrend)
	for _, a := range r.Health.TimingAnomalies {
		fmt.Fprintf(&b, "   Timing anomaly:   %s\n", a)
	}
	fmt.Fprintf(&b, "   → VERDICT: %s\n", r.Health.Verdict)
	for _, f := range r.Findings {
		if f.Type == FindingHealth {
			fmt.Fprintf(&b, "   → FINDING: [%s] %s\n", f.Type, f.Message)
		}
	}
	fmt.Fprintln(&b)

	// Summary.
	critical, warning, info := countSeverities(r.Findings)
	fmt.Fprintf(&b, "Findings: %d (%d critical, %d warning, %d info)\n",
		len(r.Findings), critical, warning, info)

	return b.String()
}

// FormatTrends renders TrendData as a summary table.
func FormatTrends(td *TrendData) string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== Session Trends (%d sessions) ===\n\n", td.SessionCount)

	if td.SessionCount == 0 {
		fmt.Fprintln(&b, "No session data available.")
		return b.String()
	}

	for _, tt := range td.ByTask {
		fmt.Fprintf(&b, "Task: %s (%d sessions, %.1f%% completion)\n",
			tt.Task, tt.SessionCount, tt.CompletionRate*100)
		fmt.Fprintf(&b, "  Avg turns: %.1f | Avg tokens: %s | Avg TTFT: %s\n",
			tt.AvgTurns,
			fmtInt(int(tt.AvgTokens)),
			fmtDuration(int64(tt.AvgTTFTMs)))
		fmt.Fprintf(&b, "  Token trend: %s | Error trend: %s\n",
			tt.TokenTrend, tt.ErrorTrend)
		fmt.Fprintln(&b)
	}

	if len(td.Regressions) == 0 {
		fmt.Fprintln(&b, "Regressions: none")
	} else {
		fmt.Fprintf(&b, "Regressions: %d\n", len(td.Regressions))
		for _, r := range td.Regressions {
			fmt.Fprintf(&b, "  [%s] %s\n", r.Severity, r.Message)
		}
	}

	return b.String()
}

// FormatDetail renders a SessionDetail as a formatted summary.
func FormatDetail(d *SessionDetail) string {
	var b strings.Builder

	fmt.Fprintf(&b, "=== Session: %s ===\n\n", d.SessionID)
	fmt.Fprintf(&b, "Task:     %s\n", d.Task)
	fmt.Fprintf(&b, "Model:    %s\n", d.Model)
	fmt.Fprintf(&b, "Provider: %s\n", d.Provider)
	fmt.Fprintf(&b, "Status:   %s\n", d.Status)
	fmt.Fprintf(&b, "Duration: %s\n", fmtDuration(d.DurationMs))
	fmt.Fprintf(&b, "Turns:    %d\n", d.Turns)
	fmt.Fprintf(&b, "Tokens:   %s\n", fmtInt(d.Tokens))

	if len(d.ToolCalls) > 0 {
		fmt.Fprintln(&b, "\nTool calls:")
		for name, count := range d.ToolCalls {
			fmt.Fprintf(&b, "  %s: %d\n", name, count)
		}
	}

	if len(d.TurnDetails) > 0 {
		fmt.Fprintln(&b, "\nTurn details:")
		for _, t := range d.TurnDetails {
			fmt.Fprintf(&b, "  Turn %d: %s tokens (in: %d / out: %d)",
				t.Number, fmtInt(t.InputTok+t.OutputTok), t.InputTok, t.OutputTok)
			if len(t.ToolNames) > 0 {
				fmt.Fprintf(&b, " → %s", join(t.ToolNames, ", "))
			}
			if t.Errors > 0 {
				fmt.Fprintf(&b, " [%d errors]", t.Errors)
			}
			fmt.Fprintln(&b)
		}
	}

	return b.String()
}

func countSeverities(findings []Finding) (critical, warning, info int) {
	for _, f := range findings {
		switch f.Severity {
		case SeverityCritical:
			critical++
		case SeverityWarning:
			warning++
		case SeverityInfo:
			info++
		}
	}
	return
}

func fmtInt(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}

func fmtDuration(ms int64) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
