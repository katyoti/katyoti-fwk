// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/session"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/spf13/cobra"
)

var (
	analyzeAll       bool
	analyzeJSON      bool
	analyzeNoMetrics bool
	trendsTask       string
	trendsJSON       bool
	showJSON         bool
)

func init() {
	sessionAnalyzeCmd.Flags().BoolVar(&analyzeAll, "all", false, "Analyze all sessions")
	sessionAnalyzeCmd.Flags().BoolVar(&analyzeJSON, "json", false, "Output raw JSON")
	sessionAnalyzeCmd.Flags().BoolVar(&analyzeNoMetrics, "no-metrics", false, "Skip writing to metrics.jsonl")

	sessionTrendsCmd.Flags().StringVar(&trendsTask, "task", "", "Filter by task name")
	sessionTrendsCmd.Flags().BoolVar(&trendsJSON, "json", false, "Output raw JSON")

	sessionShowCmd.Flags().BoolVar(&showJSON, "json", false, "Output raw JSON")

	sessionCmd.AddCommand(sessionAnalyzeCmd)
	sessionCmd.AddCommand(sessionTrendsCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	rootCmd.AddCommand(sessionCmd)
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Analyze and inspect session data",
}

var sessionAnalyzeCmd = &cobra.Command{
	Use:   "analyze [session-id]",
	Short: "Evaluate a session's quality, efficiency, and health",
	Long: `Analyze a session and produce an evaluation report answering:
1. Outcome quality — completion, turns, corrections
2. Efficiency — token waste, unused tools, user response time
3. Internal health — errors, latency, TTFT trends

With --all: batch analyze all sessions and summarize findings.`,
	RunE: runSessionAnalyze,
}

var sessionTrendsCmd = &cobra.Command{
	Use:   "trends",
	Short: "Show cross-session trends and regressions",
	RunE:  runSessionTrends,
}

var sessionShowCmd = &cobra.Command{
	Use:   "show [session-id]",
	Short: "Display session summary and per-turn breakdown",
	Long:  "Without arguments: shows a session picker. With a session ID: opens that session directly.",
	RunE:  runSessionShow,
}

func runSessionAnalyze(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()

	if analyzeAll {
		return runAnalyzeAll(sessionsDir)
	}

	dir, err := resolveSession(sessionsDir, args)
	if err != nil {
		return err
	}

	report, err := session.AnalyzeSession(dir)
	if err != nil {
		return fmt.Errorf("analyze session: %w", err)
	}

	if analyzeJSON {
		return printJSON(report)
	}

	fmt.Print(session.FormatReport(report))

	if !analyzeNoMetrics {
		metricsPath := session.MetricsPath(sessionsDir)
		line := session.MetricsFromReport(report)
		if err := session.AppendMetrics(metricsPath, line); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to write metrics: %v\n", err)
		}
	}

	return nil
}

func runAnalyzeAll(sessionsDir string) error {
	sessions, err := journal.ListSessions(sessionsDir)
	if err != nil {
		return fmt.Errorf("list sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	var (
		totalFindings int
		critical      int
		warning       int
		analyzed      int
		failed        int
	)

	for _, s := range sessions {
		report, err := session.AnalyzeSession(s.Dir)
		if err != nil {
			failed++
			continue
		}
		analyzed++

		for _, f := range report.Findings {
			totalFindings++
			switch f.Severity {
			case session.SeverityCritical:
				critical++
			case session.SeverityWarning:
				warning++
			}
		}

		if analyzeJSON {
			_ = printJSON(report)
		} else {
			fmt.Print(session.FormatReport(report))
			fmt.Println()
		}

		if !analyzeNoMetrics {
			metricsPath := session.MetricsPath(sessionsDir)
			line := session.MetricsFromReport(report)
			_ = session.AppendMetrics(metricsPath, line)
		}
	}

	if !analyzeJSON {
		fmt.Printf("=== Summary: %d analyzed, %d failed, %d findings (%d critical, %d warning) ===\n",
			analyzed, failed, totalFindings, critical, warning)
	}
	return nil
}

func runSessionTrends(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()

	filter := session.TrendFilter{
		Task: trendsTask,
	}

	trends, err := session.ComputeTrends(sessionsDir, filter)
	if err != nil {
		return fmt.Errorf("compute trends: %w", err)
	}

	if trendsJSON {
		return printJSON(trends)
	}

	fmt.Print(session.FormatTrends(trends))
	return nil
}

func runSessionShow(cmd *cobra.Command, args []string) error {
	sessionsDir := application.Env().SessionsDir()

	dir, err := resolveSession(sessionsDir, args)
	if err != nil {
		return err
	}

	detail, err := session.ShowSession(dir)
	if err != nil {
		return fmt.Errorf("show session: %w", err)
	}

	if showJSON {
		return printJSON(detail)
	}

	fmt.Print(session.FormatDetail(detail))
	return nil
}

// resolveSession finds a session directory from args or interactive picker.
func resolveSession(sessionsDir string, args []string) (string, error) {
	sessions, err := journal.ListSessions(sessionsDir)
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no sessions found")
	}

	if len(args) > 0 {
		for _, s := range sessions {
			if s.ID == args[0] || strings.HasSuffix(s.ID, "_"+args[0]) {
				return s.Dir, nil
			}
		}
		return "", fmt.Errorf("session not found: %s", args[0])
	}

	// Interactive picker.
	return journal.PickSession(sessions)
}

func printJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
