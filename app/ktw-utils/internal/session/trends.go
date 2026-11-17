// SPDX-License-Identifier: Apache-2.0

package session

import (
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
)

// ComputeTrends aggregates metrics across sessions and detects regressions.
// It reads metrics.jsonl if available, otherwise rebuilds from session summaries.
func ComputeTrends(sessionsDir string, filter TrendFilter) (*TrendData, error) {
	metricsPath := MetricsPath(sessionsDir)
	lines, err := ReadMetrics(metricsPath)
	if err != nil || len(lines) == 0 {
		// Fallback: rebuild from session listing.
		lines, err = rebuildMetrics(sessionsDir)
		if err != nil {
			return nil, fmt.Errorf("no metrics data available: %w", err)
		}
	}

	// Apply filters.
	var filtered []MetricsLine
	for _, m := range lines {
		if filter.Task != "" && m.Task != filter.Task {
			continue
		}
		if filter.Model != "" && m.Model != filter.Model {
			continue
		}
		filtered = append(filtered, m)
	}
	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[len(filtered)-filter.Limit:]
	}

	if len(filtered) == 0 {
		return &TrendData{ByTask: make(map[string]*TaskTrend)}, nil
	}

	// Group by task.
	byTask := make(map[string][]MetricsLine)
	for _, m := range filtered {
		byTask[m.Task] = append(byTask[m.Task], m)
	}

	td := &TrendData{
		SessionCount: len(filtered),
		DateRange:    [2]string{filtered[0].SessionID, filtered[len(filtered)-1].SessionID},
		ByTask:       make(map[string]*TaskTrend),
	}

	for task, metrics := range byTask {
		tt := computeTaskTrend(task, metrics)
		td.ByTask[task] = tt

		// Detect regressions.
		if tt.TokenTrend == "increasing" {
			td.Regressions = append(td.Regressions, Finding{
				Type:     FindingEfficiency,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Token usage increasing for task %q", task),
				Metric:   "token_trend",
			})
		}
		if tt.ErrorTrend == "increasing" {
			td.Regressions = append(td.Regressions, Finding{
				Type:     FindingHealth,
				Severity: SeverityWarning,
				Message:  fmt.Sprintf("Error rate increasing for task %q", task),
				Metric:   "error_trend",
			})
		}
	}

	return td, nil
}

func computeTaskTrend(task string, metrics []MetricsLine) *TaskTrend {
	// Sort by session_id (lexicographic = chronological).
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].SessionID < metrics[j].SessionID
	})

	tt := &TaskTrend{
		Task:         task,
		SessionCount: len(metrics),
	}

	var (
		completed   int
		totalTurns  float64
		totalTokens float64
		totalDur    float64
		totalErr    float64
		totalTTFT   float64
	)
	for _, m := range metrics {
		if m.Status == "completed" {
			completed++
		}
		totalTurns += float64(m.Turns)
		totalTokens += float64(m.TotalTokens)
		totalDur += float64(m.DurationMs)
		totalErr += m.ErrorRate
		totalTTFT += float64(m.AvgTTFTMs)
	}

	n := float64(len(metrics))
	tt.CompletionRate = float64(completed) / n
	tt.AvgTurns = totalTurns / n
	tt.AvgTokens = totalTokens / n
	tt.AvgDurationMs = totalDur / n
	tt.AvgErrorRate = totalErr / n
	tt.AvgTTFTMs = totalTTFT / n

	// Recent sessions (last 5).
	start := len(metrics) - 5
	if start < 0 {
		start = 0
	}
	for _, m := range metrics[start:] {
		tt.RecentSessions = append(tt.RecentSessions, m.SessionID)
	}

	// Trend detection: compare first half vs second half.
	tt.TokenTrend = detectTrend(metrics, func(m MetricsLine) float64 { return float64(m.TotalTokens) })
	tt.ErrorTrend = detectTrend(metrics, func(m MetricsLine) float64 { return m.ErrorRate })

	return tt
}

func detectTrend(metrics []MetricsLine, extract func(MetricsLine) float64) string {
	if len(metrics) < 4 {
		return "stable"
	}
	mid := len(metrics) / 2
	firstAvg := avgMetric(metrics[:mid], extract)
	secondAvg := avgMetric(metrics[mid:], extract)

	if firstAvg == 0 {
		return "stable"
	}
	change := (secondAvg - firstAvg) / firstAvg
	if change > 0.1 {
		return "increasing"
	}
	if change < -0.1 {
		return "decreasing"
	}
	return "stable"
}

func avgMetric(metrics []MetricsLine, extract func(MetricsLine) float64) float64 {
	if len(metrics) == 0 {
		return 0
	}
	var sum float64
	for _, m := range metrics {
		sum += extract(m)
	}
	return sum / float64(len(metrics))
}

func rebuildMetrics(sessionsDir string) ([]MetricsLine, error) {
	sessions, err := journal.ListSessions(sessionsDir)
	if err != nil {
		return nil, err
	}

	var lines []MetricsLine
	for _, s := range sessions {
		report, err := AnalyzeSession(s.Dir)
		if err != nil {
			continue // skip unanalyzable sessions
		}
		lines = append(lines, MetricsFromReport(report))
	}
	return lines, nil
}
