// SPDX-License-Identifier: Apache-2.0

package session

import (
	"bufio"
	"encoding/json"
	"os"
)

// ParsePerTurnLatency reads llm.jsonl and returns per-turn timing data
// extracted from llm_response entries.
func ParsePerTurnLatency(llmJournalPath string) ([]TurnLatency, error) {
	f, err := os.Open(llmJournalPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var results []TurnLatency
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry struct {
			Type string          `json:"type"`
			Turn int             `json:"turn"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Type != "llm_response" {
			continue
		}
		var data struct {
			DurationMs int64 `json:"durationMs"`
			TTFTMs     int64 `json:"ttftMs"`
		}
		if err := json.Unmarshal(entry.Data, &data); err != nil {
			continue
		}
		results = append(results, TurnLatency{
			Turn:       entry.Turn,
			DurationMs: data.DurationMs,
			TTFTMs:     data.TTFTMs,
		})
	}
	return results, scanner.Err()
}
