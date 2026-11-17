// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

// UserEvent is a parsed event from user.jsonl.
type UserEvent struct {
	Type      string          `json:"type"`
	Seq       int64           `json:"seq"`
	Turn      int             `json:"turn"`
	ToolID    string          `json:"tool_id,omitempty"`
	Timestamp time.Time       `json:"ts"`
	Data      json.RawMessage `json:"data"`
}

// ParseUserEvents reads user.jsonl and returns all events.
func ParseUserEvents(userJournalPath string) ([]UserEvent, error) {
	f, err := os.Open(userJournalPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []UserEvent
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0), 10*1024*1024) // 10MB buffer
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev UserEvent
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // skip malformed lines
		}
		events = append(events, ev)
	}
	return events, scanner.Err()
}

// promptKey identifies a prompt by turn + tool_id for matching with user_input.
type promptKey struct {
	Turn   int
	ToolID string
}

// ExtractUserResponseTimes pairs prompt_shown → user_input events
// by matching on (turn, tool_id) and computing the time delta.
func ExtractUserResponseTimes(events []UserEvent) []UserResponseTime {
	type promptInfo struct {
		Timestamp time.Time
		Question  string
	}

	prompts := make(map[promptKey]promptInfo)

	for _, ev := range events {
		if ev.Type == "prompt_shown" {
			key := promptKey{Turn: ev.Turn, ToolID: ev.ToolID}
			var data struct {
				Question string `json:"question"`
			}
			_ = json.Unmarshal(ev.Data, &data)
			prompts[key] = promptInfo{
				Timestamp: ev.Timestamp,
				Question:  data.Question,
			}
		}
	}

	var results []UserResponseTime
	for _, ev := range events {
		if ev.Type == "user_input" {
			key := promptKey{Turn: ev.Turn, ToolID: ev.ToolID}
			if p, ok := prompts[key]; ok {
				delta := ev.Timestamp.Sub(p.Timestamp).Milliseconds()
				if delta < 0 {
					delta = 0
				}
				results = append(results, UserResponseTime{
					Turn:       ev.Turn,
					ToolID:     ev.ToolID,
					Question:   p.Question,
					ResponseMs: delta,
				})
			}
		}
	}
	return results
}
