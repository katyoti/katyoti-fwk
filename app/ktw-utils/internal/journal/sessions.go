// SPDX-License-Identifier: Apache-2.0

package journal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// parseSessionIDParts splits a session-directory name into its
// (task, date, time) components. Handles both the legacy ISO-prefixed
// format (`2026-03-04T16-09-28_song-writer`) and the current
// UUIDv7-prefixed format (`01890abc-…_song-writer`). For UUIDv7 the
// timestamp is decoded from the UUID's 48-bit unix-ms field so the
// session-list TUI still surfaces a real date.
func parseSessionIDParts(name string) (task, date, timeStr string) {
	idx := strings.Index(name, "_")
	if idx <= 0 {
		return "", "", ""
	}
	prefix := name[:idx]
	task = name[idx+1:]

	if tIdx := strings.Index(prefix, "T"); tIdx > 0 {
		date = prefix[:tIdx]
		timeStr = strings.ReplaceAll(prefix[tIdx+1:], "-", ":")
		return task, date, timeStr
	}
	if id, err := uuid.Parse(prefix); err == nil {
		sec, nsec := id.Time().UnixTime()
		t := time.Unix(sec, nsec).Local()
		date = t.Format("2006-01-02")
		timeStr = t.Format("15:04:05")
	}
	return task, date, timeStr
}

// SessionInfo holds metadata about a discovered session.
type SessionInfo struct {
	ID       string // e.g. "2026-03-04T16-09-28_song-writer"
	Dir      string // full path to session directory
	Task     string // task name extracted from ID
	Date     string // date portion "2026-03-04"
	Time     string // time portion "16:09:28"
	Status   string // from summary.json
	Turns    int
	Tokens   int
	Model    string
	Duration int // ms
}

// Summary matches the summary.json structure.
type Summary struct {
	SessionID  string         `json:"session_id"`
	Task       string         `json:"task"`
	Model      string         `json:"model"`
	Provider   string         `json:"provider"`
	Status     string         `json:"status"`
	DurationMs int            `json:"duration_ms"`
	Turns      int            `json:"turns"`
	InputTok   int            `json:"input_tokens"`
	OutputTok  int            `json:"output_tokens"`
	TotalTok   int            `json:"total_tokens"`
	ToolCalls  map[string]int `json:"tool_calls"`
	ErrorCount int            `json:"error_count"`
}

// ListSessions scans the sessions directory and returns all sessions sorted by ID (newest first).
func ListSessions(sessionsDir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}

	var sessions []SessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(sessionsDir, name)

		// Must have llm.jsonl
		if _, err := os.Stat(filepath.Join(dir, "llm.jsonl")); err != nil {
			continue
		}

		info := SessionInfo{
			ID:  name,
			Dir: dir,
		}

		info.Task, info.Date, info.Time = parseSessionIDParts(name)

		// Read summary if available
		sumPath := filepath.Join(dir, "summary.json")
		if data, err := os.ReadFile(sumPath); err == nil {
			var sum Summary
			if json.Unmarshal(data, &sum) == nil {
				info.Status = sum.Status
				info.Turns = sum.Turns
				info.Tokens = sum.TotalTok
				info.Model = sum.Model
				info.Duration = sum.DurationMs
			}
		}

		sessions = append(sessions, info)
	}

	// Sort newest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID > sessions[j].ID
	})

	return sessions, nil
}
