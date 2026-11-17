// SPDX-License-Identifier: Apache-2.0

package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/app/ktw-utils/internal/journal"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ShowSession loads summary + conversation for display.
func ShowSession(sessionDir string) (*SessionDetail, error) {
	llmPath := filepath.Join(sessionDir, "llm.jsonl")
	userPath := filepath.Join(sessionDir, "user.jsonl")
	summaryPath := filepath.Join(sessionDir, "summary.json")

	detail := &SessionDetail{}

	// Load summary for metadata.
	if data, err := os.ReadFile(summaryPath); err == nil {
		var s machine.SessionSummary
		if err := json.Unmarshal(data, &s); err == nil {
			detail.SessionID = s.SessionID
			detail.Task = s.Task
			detail.Model = s.Model
			detail.Provider = s.Provider
			detail.Status = s.Status
			detail.DurationMs = s.DurationMs
			detail.Turns = s.Turns
			detail.Tokens = s.TotalTokens
			detail.ToolCalls = s.ToolCalls
		}
	} else {
		// Try to re-compute from journals.
		if s, err := machine.ComputeSummary(llmPath, userPath); err == nil {
			detail.SessionID = s.SessionID
			detail.Task = s.Task
			detail.Model = s.Model
			detail.Provider = s.Provider
			detail.Status = "abandoned"
			detail.DurationMs = s.DurationMs
			detail.Turns = s.Turns
			detail.Tokens = s.TotalTokens
			detail.ToolCalls = s.ToolCalls
		} else {
			return nil, fmt.Errorf("no session data found in %s", sessionDir)
		}
	}

	// Load conversation for per-turn details.
	if conv, err := journal.ParseConversation(sessionDir); err == nil {
		for _, turn := range conv.Turns {
			td := TurnDetail{
				Number:     turn.Number,
				UserText:   turn.UserText,
				AssistText: turn.AssistText,
				DurationMs: turn.DurationMs,
				InputTok:   turn.InputTok,
				OutputTok:  turn.OutputTok,
			}
			for _, tc := range turn.ToolCalls {
				td.ToolNames = append(td.ToolNames, tc.Name)
				if tc.Error {
					td.Errors++
				}
			}
			detail.TurnDetails = append(detail.TurnDetails, td)
		}
	}

	return detail, nil
}
