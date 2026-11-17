// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// UserEventType identifies the kind of user journal entry.
type UserEventType string

const (
	UserSessionStart UserEventType = "session_start"
	UserSessionEnd   UserEventType = "session_end"
	UserMessageShown UserEventType = "message_shown"
	UserPromptShown  UserEventType = "prompt_shown"
	UserInput        UserEventType = "user_input"
	UserToolFeedback UserEventType = "tool_feedback"
	UserErrorShown   UserEventType = "error_shown"
)

// UserJournalEntry is a single line in the user-perspective JSONL log.
type UserJournalEntry struct {
	Type      UserEventType `json:"type"`
	Seq       int64         `json:"seq,omitempty"`
	Turn      int           `json:"turn,omitempty"`
	ToolID    string        `json:"tool_id,omitempty"`
	Timestamp time.Time     `json:"ts"`
	Data      any           `json:"data,omitempty"`
}

// UserJournal writes user-perspective events to a JSONL file.
// Events capture what the user saw and typed, correlated with
// the LLM journal via shared seq/turn/tool_id.
type UserJournal struct {
	mu       sync.Mutex
	out      rotatingFile
	path     string
	recorder *SessionRecorder
}

// Log writes a user event. Safe for concurrent use.
func (u *UserJournal) Log(eventType UserEventType, turn int, toolID string, data any) {
	if u.out.file == nil {
		return
	}

	entry := UserJournalEntry{
		Type:      eventType,
		Seq:       u.recorder.NextSeq(),
		Turn:      turn,
		ToolID:    toolID,
		Timestamp: time.Now(),
		Data:      data,
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	if u.out.file == nil {
		// Closed between the fast-path check and acquiring the lock.
		return
	}

	line, err := json.Marshal(entry)
	if err != nil {
		slog.Warn("user journal: marshal failed", "path", u.path, "type", eventType, "err", err)
		return
	}
	// Scrub secrets at the single marshaled-line chokepoint (nil-safe).
	line = u.recorder.redactor.RedactBytes(line)
	if err := u.out.write(append(line, '\n')); err != nil {
		slog.Warn("user journal: write failed", "path", u.path, "type", eventType, "err", err)
	}
}

// Path returns the user journal file path (empty if disabled).
func (u *UserJournal) Path() string {
	return u.path
}

// Close flushes and closes the user journal file. The handle is nilled
// under the write mutex so a late Log lands on the closed-journal path
// instead of writing to a closed descriptor.
func (u *UserJournal) Close() error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.out.file == nil {
		return nil
	}
	err := u.out.close()
	return err
}
