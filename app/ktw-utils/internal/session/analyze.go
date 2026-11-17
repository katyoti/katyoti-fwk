// SPDX-License-Identifier: Apache-2.0

package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// AnalyzeSession produces a full evaluation report for a single session.
func AnalyzeSession(sessionDir string) (*SessionReport, error) {
	llmPath := filepath.Join(sessionDir, "llm.jsonl")
	userPath := filepath.Join(sessionDir, "user.jsonl")
	summaryPath := filepath.Join(sessionDir, "summary.json")

	// 1. Read summary.json for status (may not exist for abandoned sessions).
	status, summaryExists := readStatusFromSummary(summaryPath)

	// 2. Compute rich metrics from journals.
	summary, err := machine.ComputeSummary(llmPath, userPath)
	if err != nil {
		// If llm.jsonl doesn't exist, we can't analyze.
		return nil, fmt.Errorf("compute summary: %w", err)
	}
	if summaryExists {
		summary.Status = status
	} else {
		summary.Status = "abandoned"
	}

	// 3. Parse conversation for tool names and per-turn structure.
	conv, _ := journal.ParseConversation(sessionDir)

	// 4. Parse user events for response times.
	var responseTimes []UserResponseTime
	if _, err := os.Stat(userPath); err == nil {
		events, _ := ParseUserEvents(userPath)
		responseTimes = ExtractUserResponseTimes(events)
	}

	// 5. Parse per-turn latency.
	perTurnLatency, _ := ParsePerTurnLatency(llmPath)

	// 6. Build report.
	sessionID := summary.SessionID
	if sessionID == "" {
		sessionID = filepath.Base(sessionDir)
	}
	task := summary.Task
	if task == "" {
		task = extractTaskFromID(sessionID)
	}
	report := &SessionReport{
		SessionID:  sessionID,
		Task:       task,
		Model:      summary.Model,
		DurationMs: summary.DurationMs,
		Timestamp:  time.Now(),
	}

	report.Outcome = computeOutcome(summary, summaryExists)
	report.Efficiency = computeEfficiency(summary, conv, responseTimes)
	report.Health = computeHealth(summary, perTurnLatency)
	report.Findings = generateFindings(report)

	return report, nil
}

func readStatusFromSummary(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var s struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", false
	}
	return s.Status, true
}

func computeOutcome(s *machine.SessionSummary, summaryExists bool) OutcomeQuality {
	oq := OutcomeQuality{
		Status:    s.Status,
		Turns:     s.Turns,
		Abandoned: !summaryExists,
	}

	if oq.Abandoned {
		oq.Verdict = "Abandoned — session never completed"
	} else if s.Status == "completed" {
		if oq.Corrections == 0 {
			oq.Verdict = fmt.Sprintf("Good — completed in %d turns with no corrections", s.Turns)
		} else {
			oq.Verdict = fmt.Sprintf("Acceptable — completed in %d turns with %d corrections", s.Turns, oq.Corrections)
		}
	} else {
		oq.Verdict = fmt.Sprintf("Incomplete — status: %s after %d turns", s.Status, s.Turns)
	}
	return oq
}

func computeEfficiency(s *machine.SessionSummary, conv *journal.Conversation, responseTimes []UserResponseTime) EfficiencyMetrics {
	em := EfficiencyMetrics{
		TotalTokens:       s.TotalTokens,
		InputTokens:       s.InputTokens,
		OutputTokens:      s.OutputTokens,
		ContentOnlyTurns:  s.ContentOnlyTurns,
		AskUserCount:      s.AskUserCount,
		UserResponseTimes: responseTimes,
	}

	if s.Turns > 0 {
		em.TokensPerTurn = float64(s.TotalTokens) / float64(s.Turns)
		em.ContentOnlyRatio = float64(s.ContentOnlyTurns) / float64(s.Turns)
	}

	// Unused tools: available minus used.
	if conv != nil {
		em.ToolsAvailable = len(conv.ToolNames)
		usedSet := make(map[string]bool)
		for name := range s.ToolCalls {
			usedSet[name] = true
		}
		em.ToolsUsed = len(usedSet)
		for _, name := range conv.ToolNames {
			if !usedSet[name] {
				em.UnusedTools = append(em.UnusedTools, name)
			}
		}
		if em.ToolsAvailable > 0 {
			em.UnusedToolRatio = float64(len(em.UnusedTools)) / float64(em.ToolsAvailable)
		}
	}

	// Average user response time.
	if len(responseTimes) > 0 {
		var total int64
		for _, rt := range responseTimes {
			total += rt.ResponseMs
		}
		em.AvgUserResponseMs = total / int64(len(responseTimes))
	}

	// Verdict.
	var issues []string
	if em.UnusedToolRatio > 0.3 {
		issues = append(issues, fmt.Sprintf("%.0f%% tool waste", em.UnusedToolRatio*100))
	}
	if em.ContentOnlyRatio > 0.5 {
		issues = append(issues, fmt.Sprintf("%.0f%% content-only turns", em.ContentOnlyRatio*100))
	}
	if len(issues) == 0 {
		em.Verdict = "Good — no significant waste detected"
	} else {
		em.Verdict = "Acceptable — " + join(issues, ", ")
	}
	return em
}

func computeHealth(s *machine.SessionSummary, perTurn []TurnLatency) InternalHealth {
	totalCalls := 0
	for _, count := range s.ToolCalls {
		totalCalls += count
	}

	ih := InternalHealth{
		ErrorCount:      s.ErrorCount,
		ToolCallCount:   totalCalls,
		AvgLLMLatencyMs: s.AvgLLMDurationMs,
		AvgTTFTMs:       s.AvgTTFTMs,
		PerTurnLatency:  perTurn,
	}

	if totalCalls > 0 {
		ih.ErrorRate = float64(s.ErrorCount) / float64(totalCalls)
	}

	// TTFT trend: compare first half vs second half.
	ih.TTFTTrend = computeTTFTTrend(perTurn)

	// Timing anomalies: tool calls > 10s (excluding notify which is TTS).
	for name, count := range s.ToolCalls {
		if name == "notify" && count > 0 {
			// TTS latency is expected; not an anomaly.
			continue
		}
	}

	// Verdict.
	if s.ErrorCount > 0 {
		ih.Verdict = fmt.Sprintf("Degraded — %d errors across %d tool calls", s.ErrorCount, totalCalls)
	} else {
		ih.Verdict = "Healthy — no errors, stable latency"
	}
	return ih
}

func computeTTFTTrend(perTurn []TurnLatency) string {
	if len(perTurn) < 2 {
		return "stable"
	}
	mid := len(perTurn) / 2
	firstHalf := avgTTFT(perTurn[:mid])
	secondHalf := avgTTFT(perTurn[mid:])

	if firstHalf == 0 {
		return "stable"
	}
	change := (secondHalf - firstHalf) / firstHalf
	if change > 0.2 {
		return "increasing"
	}
	if change < -0.2 {
		return "decreasing"
	}
	return "stable"
}

func avgTTFT(items []TurnLatency) float64 {
	if len(items) == 0 {
		return 0
	}
	var sum int64
	for _, t := range items {
		sum += t.TTFTMs
	}
	return float64(sum) / float64(len(items))
}

func generateFindings(report *SessionReport) []Finding {
	var findings []Finding

	// Abandonment.
	if report.Outcome.Abandoned {
		findings = append(findings, Finding{
			Type:     FindingAbandonment,
			Severity: SeverityWarning,
			Message:  "Session never completed (no summary.json)",
			Metric:   "abandoned",
			Value:    1,
		})
	}

	// Outcome quality.
	if report.Outcome.Corrections > 0 {
		findings = append(findings, Finding{
			Type:     FindingQuality,
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("User made %d corrections", report.Outcome.Corrections),
			Metric:   "corrections",
			Value:    float64(report.Outcome.Corrections),
		})
	}

	// Efficiency: unused tools.
	if report.Efficiency.UnusedToolRatio > 0.3 {
		findings = append(findings, Finding{
			Type:      FindingEfficiency,
			Severity:  SeverityInfo,
			Message:   fmt.Sprintf("%d tools loaded but never called (%s)", len(report.Efficiency.UnusedTools), join(report.Efficiency.UnusedTools, ", ")),
			Metric:    "unused_tool_ratio",
			Value:     report.Efficiency.UnusedToolRatio,
			Threshold: 0.3,
		})
	}

	// Efficiency: content-only turns.
	if report.Efficiency.ContentOnlyRatio > 0.5 {
		findings = append(findings, Finding{
			Type:      FindingEfficiency,
			Severity:  SeverityWarning,
			Message:   fmt.Sprintf("%.0f%% of turns had no tool calls (wasted turns)", report.Efficiency.ContentOnlyRatio*100),
			Metric:    "content_only_ratio",
			Value:     report.Efficiency.ContentOnlyRatio,
			Threshold: 0.5,
		})
	}

	// Efficiency: tokens per turn.
	if report.Efficiency.TokensPerTurn > 5000 {
		findings = append(findings, Finding{
			Type:      FindingEfficiency,
			Severity:  SeverityInfo,
			Message:   fmt.Sprintf("High token usage: %.0f tokens/turn", report.Efficiency.TokensPerTurn),
			Metric:    "tokens_per_turn",
			Value:     report.Efficiency.TokensPerTurn,
			Threshold: 5000,
		})
	}

	// Health: error rate.
	if report.Health.ErrorRate > 0.3 {
		findings = append(findings, Finding{
			Type:      FindingHealth,
			Severity:  SeverityCritical,
			Message:   fmt.Sprintf("Critical error rate: %.0f%%", report.Health.ErrorRate*100),
			Metric:    "error_rate",
			Value:     report.Health.ErrorRate,
			Threshold: 0.3,
		})
	} else if report.Health.ErrorRate > 0.1 {
		findings = append(findings, Finding{
			Type:      FindingHealth,
			Severity:  SeverityWarning,
			Message:   fmt.Sprintf("Elevated error rate: %.0f%%", report.Health.ErrorRate*100),
			Metric:    "error_rate",
			Value:     report.Health.ErrorRate,
			Threshold: 0.1,
		})
	}

	// Health: TTFT.
	if report.Health.AvgTTFTMs > 2000 {
		findings = append(findings, Finding{
			Type:      FindingHealth,
			Severity:  SeverityWarning,
			Message:   fmt.Sprintf("Slow TTFT: %dms average", report.Health.AvgTTFTMs),
			Metric:    "avg_ttft_ms",
			Value:     float64(report.Health.AvgTTFTMs),
			Threshold: 2000,
		})
	}

	return findings
}

func join(items []string, sep string) string {
	if len(items) == 0 {
		return ""
	}
	result := items[0]
	for _, s := range items[1:] {
		result += sep + s
	}
	return result
}

// extractTaskFromID extracts the task name from a session ID like "2026-03-05T16-59-23_hello-world".
func extractTaskFromID(id string) string {
	if idx := strings.Index(id, "_"); idx >= 0 && idx < len(id)-1 {
		return id[idx+1:]
	}
	return id
}
