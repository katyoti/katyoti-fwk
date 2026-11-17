// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// MetricsPath returns the default metrics file path.
func MetricsPath(sessionsDir string) string {
	return filepath.Join(sessionsDir, "metrics.jsonl")
}

// MetricsFromReport extracts a MetricsLine from a SessionReport.
func MetricsFromReport(report *SessionReport) MetricsLine {
	return MetricsLine{
		SessionID:         report.SessionID,
		Task:              report.Task,
		Model:             report.Model,
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		Status:            report.Outcome.Status,
		Turns:             report.Outcome.Turns,
		TotalTokens:       report.Efficiency.TotalTokens,
		ContentOnlyRatio:  report.Efficiency.ContentOnlyRatio,
		UnusedToolRatio:   report.Efficiency.UnusedToolRatio,
		ErrorRate:         report.Health.ErrorRate,
		AvgLLMLatencyMs:   report.Health.AvgLLMLatencyMs,
		AvgTTFTMs:         report.Health.AvgTTFTMs,
		AskUserCount:      report.Efficiency.AskUserCount,
		AvgUserResponseMs: report.Efficiency.AvgUserResponseMs,
		DurationMs:        report.DurationMs,
		FindingCount:      len(report.Findings),
	}
}

// AppendMetrics appends a MetricsLine to the JSONL file.
// Deduplicates by session_id (skips if already present).
func AppendMetrics(path string, line MetricsLine) error {
	// Check for duplicate.
	existing, _ := ReadMetrics(path)
	for _, m := range existing {
		if m.SessionID == line.SessionID {
			return nil // already recorded
		}
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(line)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// ReadMetrics reads all lines from a metrics.jsonl file.
func ReadMetrics(path string) ([]MetricsLine, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []MetricsLine
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0), 1*1024*1024)
	for scanner.Scan() {
		b := scanner.Bytes()
		if len(b) == 0 {
			continue
		}
		var ml MetricsLine
		if err := json.Unmarshal(b, &ml); err != nil {
			continue
		}
		lines = append(lines, ml)
	}
	return lines, scanner.Err()
}
