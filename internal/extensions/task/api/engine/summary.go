// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// SessionSummary contains computed metrics from a session's dual journals.
type SessionSummary struct {
	SessionID string `json:"session_id"`
	Task      string `json:"task"`
	Model     string `json:"model"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`

	// Timing
	DurationMs int64 `json:"duration_ms"`

	// Turns
	Turns            int `json:"turns"`
	ContentOnlyTurns int `json:"content_only_turns"` // turns with only text, no tool calls

	// Tokens
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`

	// Tool usage
	ToolCalls    map[string]int `json:"tool_calls"` // tool name → count
	AskUserCount int            `json:"ask_user_count"`
	ErrorCount   int            `json:"error_count"`

	// LLM timing (averages in ms)
	AvgLLMDurationMs int64 `json:"avg_llm_duration_ms,omitempty"`
	AvgTTFTMs        int64 `json:"avg_ttft_ms,omitempty"`
}

// ComputeSummary reads both journal files and computes session metrics.
// Either path can be empty (metrics from that journal will be zero).
func ComputeSummary(llmJournalPath, userJournalPath string) (*SessionSummary, error) {
	s := &SessionSummary{
		ToolCalls: make(map[string]int),
	}

	// Parse LLM journal
	if llmJournalPath != "" {
		if err := s.parseLLMJournal(llmJournalPath); err != nil {
			return nil, fmt.Errorf("parse LLM journal: %w", err)
		}
	}

	// Parse user journal for additional metrics
	if userJournalPath != "" {
		if err := s.parseUserJournal(userJournalPath); err != nil {
			return nil, fmt.Errorf("parse user journal: %w", err)
		}
	}

	s.TotalTokens = s.InputTokens + s.OutputTokens

	return s, nil
}

func (s *SessionSummary) parseLLMJournal(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var llmDurations []int64
	var ttfts []int64
	turnsWithToolCalls := make(map[int]bool)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry struct {
			Type string          `json:"type"`
			Turn int             `json:"turn"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		switch JournalEntryType(entry.Type) {
		case JournalSessionStart:
			var data struct {
				Task     string `json:"task"`
				Model    string `json:"model"`
				Provider string `json:"provider"`
			}
			if json.Unmarshal(entry.Data, &data) == nil {
				s.Task = data.Task
				s.Model = data.Model
				s.Provider = data.Provider
			}

		case JournalSessionEnd:
			var data struct {
				Turns      int   `json:"turns"`
				DurationMs int64 `json:"durationMs"`
				Usage      struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(entry.Data, &data) == nil {
				s.Turns = data.Turns
				s.DurationMs = data.DurationMs
				s.InputTokens = data.Usage.InputTokens
				s.OutputTokens = data.Usage.OutputTokens
			}

		case JournalLLMResponse:
			var data struct {
				DurationMs int64 `json:"durationMs"`
				TTFTMs     int64 `json:"ttftMs"`
			}
			if json.Unmarshal(entry.Data, &data) == nil {
				if data.DurationMs > 0 {
					llmDurations = append(llmDurations, data.DurationMs)
				}
				if data.TTFTMs > 0 {
					ttfts = append(ttfts, data.TTFTMs)
				}
			}

		case JournalToolCall:
			var data struct {
				Tool string `json:"tool"`
				Turn int    `json:"turn"`
			}
			if json.Unmarshal(entry.Data, &data) == nil {
				s.ToolCalls[data.Tool]++
				if data.Tool == "ask-user" {
					s.AskUserCount++
				}
				turnsWithToolCalls[data.Turn] = true
			}

		case JournalToolResult:
			var data struct {
				IsError bool `json:"is_error"`
			}
			if json.Unmarshal(entry.Data, &data) == nil {
				if data.IsError {
					s.ErrorCount++
				}
			}
		}
	}

	// Compute content-only turns (turns without any tool calls)
	if s.Turns > 0 {
		s.ContentOnlyTurns = s.Turns - len(turnsWithToolCalls)
	}

	// Compute averages
	if len(llmDurations) > 0 {
		var total int64
		for _, d := range llmDurations {
			total += d
		}
		s.AvgLLMDurationMs = total / int64(len(llmDurations))
	}
	if len(ttfts) > 0 {
		var total int64
		for _, d := range ttfts {
			total += d
		}
		s.AvgTTFTMs = total / int64(len(ttfts))
	}

	return scanner.Err()
}

func (s *SessionSummary) parseUserJournal(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var entry struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		// Count errors from user journal (may not be in LLM journal)
		if UserEventType(entry.Type) == UserErrorShown {
			s.ErrorCount++
		}
	}

	return scanner.Err()
}

// Print writes a human-readable summary to the writer.
func (s *SessionSummary) Print(w io.Writer) {
	fmt.Fprintf(w, "\n── Session Summary ──\n")
	fmt.Fprintf(w, "Task:     %s\n", s.Task)
	fmt.Fprintf(w, "Status:   %s\n", s.Status)
	fmt.Fprintf(w, "Duration: %s\n", time.Duration(s.DurationMs)*time.Millisecond)
	fmt.Fprintf(w, "Turns:    %d (%d content-only)\n", s.Turns, s.ContentOnlyTurns)
	fmt.Fprintf(w, "Tokens:   %d in / %d out / %d total\n",
		s.InputTokens, s.OutputTokens, s.TotalTokens)

	if len(s.ToolCalls) > 0 {
		fmt.Fprintf(w, "Tools:    ")
		first := true
		for name, count := range s.ToolCalls {
			if !first {
				fmt.Fprintf(w, ", ")
			}
			fmt.Fprintf(w, "%s(%d)", name, count)
			first = false
		}
		fmt.Fprintln(w)
	}

	if s.AskUserCount > 0 {
		fmt.Fprintf(w, "Ask user: %d interactions\n", s.AskUserCount)
	}
	if s.ErrorCount > 0 {
		fmt.Fprintf(w, "Errors:   %d\n", s.ErrorCount)
	}
	if s.AvgLLMDurationMs > 0 {
		fmt.Fprintf(w, "Avg LLM:  %dms (TTFT: %dms)\n", s.AvgLLMDurationMs, s.AvgTTFTMs)
	}
	fmt.Fprintf(w, "─────────────────────\n")
}

// SaveJSON writes the summary as a JSON file. Uses 0o600 to match the journal
// permissions (R10 / audit medium-finding-3): the summary may contain
// model/tool/token-cost data that's no less sensitive than the journals it
// summarises.
func (s *SessionSummary) SaveJSON(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// SummaryPath returns the summary file path inside a session directory.
func SummaryPath(sessionDir string) string {
	return filepath.Join(sessionDir, "summary.json")
}
