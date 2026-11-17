// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// parseSessionIDParts splits a session-directory name into its
// (task, date, time) display components.
//
// Two formats coexist:
//
//   - Legacy `<dateT>_<taskName>` (e.g. `2026-03-04T16-09-28_song-writer`)
//     — the prefix is ISO-ish, the suffix the task. Used by sessions
//     created before the UUIDv7 cutover.
//
//   - Current `<uuidv7>_<taskName>` (e.g. `01890abc-1234-7abc-9def-…_hello-world`)
//     — the prefix is a UUIDv7 whose first 48 bits encode unix-ms.
//     Used by sessions created after the cutover; we decode that
//     timestamp here so the GUI's session list still shows real
//     dates instead of an opaque UUID.
//
// Returns the task name (without provider prefix) plus the date/time
// strings the GUI surface expects (`YYYY-MM-DD` / `HH:MM:SS`). Any
// part may be empty if the name doesn't match either format.
func parseSessionIDParts(name string) (task, date, timeStr string) {
	idx := strings.Index(name, "_")
	if idx <= 0 {
		return "", "", ""
	}
	prefix := name[:idx]
	task = name[idx+1:]

	// Legacy ISO-prefix path.
	if tIdx := strings.Index(prefix, "T"); tIdx > 0 {
		date = prefix[:tIdx]
		timeStr = strings.ReplaceAll(prefix[tIdx+1:], "-", ":")
		return task, date, timeStr
	}

	// UUIDv7 path — decode the 48-bit unix-ms timestamp at the head of
	// the UUID. `uuid.Time()` returns the UUIDv1/v6/v7 timestamp in
	// 100ns ticks since the Gregorian epoch; `UnixTime()` converts it
	// to (sec, nsec) since the Unix epoch. We localise via `time.Local`
	// so the displayed wall-clock matches the user's clock.
	if id, err := uuid.Parse(prefix); err == nil {
		sec, nsec := id.Time().UnixTime()
		t := time.Unix(sec, nsec).Local()
		date = t.Format("2006-01-02")
		timeStr = t.Format("15:04:05")
	}
	return task, date, timeStr
}

// SessionInfo is the wire shape returned by ListSessions. Mirrors the
// `app/ktw-utils/internal/journal.SessionInfo` shape — duplicated here
// because the journal package is under `internal/` and not importable by
// ktw-gui. Keep in sync with that source of truth.
type SessionInfo struct {
	ID         string `json:"id"`
	Task       string `json:"task"`
	Date       string `json:"date"`
	Time       string `json:"time"`
	Status     string `json:"status,omitempty"`
	Turns      int    `json:"turns,omitempty"`
	Tokens     int    `json:"tokens,omitempty"`
	Model      string `json:"model,omitempty"`
	DurationMs int    `json:"durationMs,omitempty"`
}

// ListSessionsResult wraps the slice + an optional error message so the
// frontend can distinguish an empty-list state from a real failure.
type ListSessionsResult struct {
	Sessions []SessionInfo `json:"sessions,omitempty"`
	Error    string        `json:"error,omitempty"`
}

// sessionSummary mirrors the on-disk summary.json shape (also duplicated
// from journal/sessions.go — see SessionInfo comment).
type sessionSummary struct {
	Status     string `json:"status"`
	Turns      int    `json:"turns"`
	TotalTok   int    `json:"total_tokens"`
	Model      string `json:"model"`
	DurationMs int    `json:"duration_ms"`
}

// ListSessions returns the session journals for the named universe, newest
// first. Sessions live at `<DataDir>/sessions/` where DataDir is the per-
// universe data root derived by `application.EnvOf(<workspaceRoot>)`.
//
// Empty `universeName` → returns an empty list (no error). Unknown universe
// names return an error. Universes that exist but have no sessions yet
// return an empty list.
//
// Wails-bound: window.go.main.App.ListSessions(universeName).
func (a *App) ListSessions(universeName string) ListSessionsResult {
	if universeName == "" {
		return ListSessionsResult{}
	}

	// Resolve the universe's workspaceRoot via the internal listUniverses
	// helper (reuses the canonical registry read).
	listResult := a.listUniverses()
	if listResult.Error != "" {
		return ListSessionsResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return ListSessionsResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	// SessionsDir = <DataDir>/sessions, where DataDir derives via the
	// canonical KTW_DATA → Settings.yaml → XDG chain.
	sessionsDir := application.EnvOf(workspaceRoot).SessionsDir()
	infos, err := scanSessions(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return ListSessionsResult{}
		}
		return ListSessionsResult{Error: fmt.Sprintf("list sessions at %s: %v", sessionsDir, err)}
	}
	return ListSessionsResult{Sessions: infos}
}

// DeleteSessionResult wraps an optional error string for the delete binding.
type DeleteSessionResult struct {
	Error string `json:"error,omitempty"`
}

// DeleteSession removes a single session's on-disk state from the named
// universe — its journal directory under `<DataDir>/sessions/<id>` plus the
// sibling terminal-recording directory `<DataDir>/terminal/<id>` (best effort).
// The sessions list grows unbounded otherwise; this is the GUI's cleanup path.
//
// Resolution mirrors ListSessions / GetSession. `sessionId` is constrained to a
// single path segment (no separators / "..") so a crafted id can't escape the
// sessions root. A running session whose files are still open will fail to
// delete on Windows (locked) — surfaced as an error rather than silently.
//
// Wails-bound: window.go.main.App.DeleteSession(universeName, sessionId).
func (a *App) DeleteSession(universeName, sessionId string) DeleteSessionResult {
	if universeName == "" || sessionId == "" {
		return DeleteSessionResult{Error: "universeName and sessionId are required"}
	}
	// Path-traversal guard: the id must be exactly one path segment.
	if sessionId != filepath.Base(sessionId) || strings.ContainsAny(sessionId, `/\`) || sessionId == "." || sessionId == ".." {
		return DeleteSessionResult{Error: fmt.Sprintf("invalid session id %q", sessionId)}
	}

	listResult := a.listUniverses()
	if listResult.Error != "" {
		return DeleteSessionResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return DeleteSessionResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionsDir := application.EnvOf(workspaceRoot).SessionsDir()
	sessionDir := filepath.Join(sessionsDir, sessionId)
	if _, err := os.Stat(sessionDir); err != nil {
		return DeleteSessionResult{Error: fmt.Sprintf("session %q not found in universe %q", sessionId, universeName)}
	}
	if err := os.RemoveAll(sessionDir); err != nil {
		return DeleteSessionResult{Error: fmt.Sprintf("delete session %q: %v", sessionId, err)}
	}
	// Best-effort: drop the sibling terminal recording dir (<DataDir>/terminal/<id>).
	_ = os.RemoveAll(filepath.Join(filepath.Dir(sessionsDir), "terminal", sessionId))
	return DeleteSessionResult{}
}

// SessionTranscriptEntry is one line from the user-perspective journal,
// projected to a wire-friendly shape. Each entry is what the user saw at
// that turn (assistant message, their own input, tool feedback, error).
type SessionTranscriptEntry struct {
	Type      string `json:"type"` // "message_shown" | "user_input" | "tool_feedback" | "prompt_shown" | "error_shown" | …
	Turn      int    `json:"turn,omitempty"`
	Timestamp string `json:"timestamp"`      // RFC3339
	Data      any    `json:"data,omitempty"` // entry payload — opaque to the GUI, rendered as JSON
}

// SessionDetail is the full read-only view of a single session. Carries
// the metadata (re-derived from session_start in llm.jsonl + summary.json)
// AND the user-perspective transcript.
type SessionDetail struct {
	ID         string                   `json:"id"`
	Task       string                   `json:"task"`
	Date       string                   `json:"date"`
	Time       string                   `json:"time"`
	Status     string                   `json:"status,omitempty"`
	Turns      int                      `json:"turns,omitempty"`
	Tokens     int                      `json:"tokens,omitempty"`
	Model      string                   `json:"model,omitempty"`
	Provider   string                   `json:"provider,omitempty"`
	DurationMs int                      `json:"durationMs,omitempty"`
	Transcript []SessionTranscriptEntry `json:"transcript,omitempty"`
}

// GetSessionResult wraps SessionDetail + an optional error message.
type GetSessionResult struct {
	Detail *SessionDetail `json:"detail,omitempty"`
	Error  string         `json:"error,omitempty"`
}

// GetSession reads the read-only transcript of a single session in the
// named universe. Returns the user-perspective journal entries
// (`<sessionDir>/user.jsonl`) — what the user actually saw, NOT the
// underlying LLM request/response stream.
//
// Wails-bound: window.go.main.App.GetSession(universeName, sessionId).
func (a *App) GetSession(universeName, sessionId string) GetSessionResult {
	if universeName == "" || sessionId == "" {
		return GetSessionResult{Error: "universeName and sessionId are required"}
	}
	// Reuse list to resolve the workspace root.
	listResult := a.listUniverses()
	if listResult.Error != "" {
		return GetSessionResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return GetSessionResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionDir := filepath.Join(application.EnvOf(workspaceRoot).SessionsDir(), sessionId)
	if _, err := os.Stat(sessionDir); err != nil {
		return GetSessionResult{Error: fmt.Sprintf("session %q not found in universe %q", sessionId, universeName)}
	}

	detail := SessionDetail{ID: sessionId}
	detail.Task, detail.Date, detail.Time = parseSessionIDParts(sessionId)

	// summary.json — opportunistic.
	if data, err := os.ReadFile(filepath.Join(sessionDir, "summary.json")); err == nil {
		var sum struct {
			Status     string `json:"status"`
			Turns      int    `json:"turns"`
			TotalTok   int    `json:"total_tokens"`
			Model      string `json:"model"`
			Provider   string `json:"provider"`
			DurationMs int    `json:"duration_ms"`
		}
		if json.Unmarshal(data, &sum) == nil {
			detail.Status = sum.Status
			detail.Turns = sum.Turns
			detail.Tokens = sum.TotalTok
			detail.Model = sum.Model
			detail.Provider = sum.Provider
			detail.DurationMs = sum.DurationMs
		}
	}

	// user.jsonl — transcript. Best-effort: skip lines that fail to parse.
	userPath := filepath.Join(sessionDir, "user.jsonl")
	if f, err := os.Open(userPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var entry struct {
				Type      string    `json:"type"`
				Turn      int       `json:"turn,omitempty"`
				Timestamp time.Time `json:"ts"`
				Data      any       `json:"data,omitempty"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
				continue
			}
			detail.Transcript = append(detail.Transcript, SessionTranscriptEntry{
				Type:      entry.Type,
				Turn:      entry.Turn,
				Timestamp: entry.Timestamp.Format(time.RFC3339),
				Data:      entry.Data,
			})
		}
	}
	// Missing user.jsonl is fine — returns an empty transcript.

	return GetSessionResult{Detail: &detail}
}

// ResumeSessionResult wraps the runID/error returned by ResumeSession.
type ResumeSessionResult struct {
	RunID string `json:"runID,omitempty"`
	Error string `json:"error,omitempty"`
}

// ResumeSession starts a fresh agent run that continues a stored session,
// optionally appending a new user prompt. The agent picks up with the
// session's prior conversation by way of `terminalapi.StartTaskInput.
// InitialMessages` → `TaskRunRequest.ConfigOverride[initialMessages]` →
// `AgentRunRequest.InitialMessages` (HistoryMode=full_fork).
//
// Resolution:
//
//  1. Reuse `UniverseList` to find the universe's workspace root.
//  2. Locate `<DataDir>/sessions/<sessionId>/llm.jsonl`.
//  3. Find the LAST `llm_request` entry — its `data.messages` array is the
//     snapshot of the conversation right before the last LLM call.
//  4. Optionally append `{role:"human", parts:[{text: newInput}]}` so the
//     LLM sees the user's follow-up as the next turn.
//  5. Parse the session id to recover the original task name
//     (`<dateT>_<task>`) and start a new run via the existing
//     `runViaUniverse` flow with InitialMessages set.
//
// The new run gets a FRESH sessionID (it writes a new session journal).
// Resuming doesn't append to the original session's journal — it forks
// the conversation forward into a new session that shares the chat history.
//
// Wails-bound: window.go.main.App.ResumeSession(universeName, sessionId, newInput).
func (a *App) ResumeSession(universeName, sessionId, newInput string) ResumeSessionResult {
	if universeName == "" || sessionId == "" {
		return ResumeSessionResult{Error: "universeName and sessionId are required"}
	}

	// Resolve workspace root.
	listResult := a.listUniverses()
	if listResult.Error != "" {
		return ResumeSessionResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return ResumeSessionResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionDir := filepath.Join(application.EnvOf(workspaceRoot).SessionsDir(), sessionId)
	if _, err := os.Stat(sessionDir); err != nil {
		return ResumeSessionResult{Error: fmt.Sprintf("session %q not found in universe %q", sessionId, universeName)}
	}

	// Recover the task name from the session id ("<dateT>_<task>").
	var taskName string
	if idx := strings.Index(sessionId, "_"); idx > 0 {
		taskName = sessionId[idx+1:]
	}
	if taskName == "" {
		return ResumeSessionResult{Error: fmt.Sprintf("can't recover task name from session id %q", sessionId)}
	}

	// Load the last llm_request from llm.jsonl. Its `data.messages` carries
	// the conversation snapshot the LLM saw at the most recent turn.
	llmPath := filepath.Join(sessionDir, "llm.jsonl")
	messages, err := lastLLMRequestMessages(llmPath)
	if err != nil {
		return ResumeSessionResult{Error: fmt.Sprintf("read llm.jsonl: %v", err)}
	}
	if len(messages) == 0 {
		return ResumeSessionResult{Error: "session has no llm_request entries — nothing to resume from"}
	}

	// Append the new user input as a fresh human turn so the agent's first
	// LLM call sees the user's follow-up. Skip when empty — caller may want
	// to resume without a new prompt (the agent loop will still emit a
	// continuation turn, depending on the task).
	if trimmed := strings.TrimSpace(newInput); trimmed != "" {
		humanMsg := map[string]any{
			"role":  "human",
			"parts": []any{map[string]any{"type": "text", "text": trimmed}},
		}
		raw, mErr := json.Marshal(humanMsg)
		if mErr == nil {
			messages = append(messages, raw)
		}
	}

	// Start the new run via the existing universe-aware path. Concurrent
	// runs (P1) — no single-task guard; the resume registers under its own
	// (fresh) session id like any other run.
	go a.runViaUniverseWithMessages(universeName, taskName, newInput, messages)

	return ResumeSessionResult{RunID: ""}
}

// lastLLMRequestMessages reads llm.jsonl and returns the messages array
// from the LATEST `llm_request` entry. Returns raw JSON entries (each a
// serialized `models.MessageContent`) suitable for forwarding via
// `terminalapi.StartTaskInput.InitialMessages`.
//
// Best-effort: lines that fail to parse are skipped. Missing/empty file
// returns nil with no error (caller decides what to do).
func lastLLMRequestMessages(path string) ([]json.RawMessage, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var latestMessages []json.RawMessage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var probe struct {
			Type string `json:"type"`
			Data struct {
				Messages []json.RawMessage `json:"messages"`
			} `json:"data"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &probe); err != nil {
			continue
		}
		if probe.Type == "llm_request" && len(probe.Data.Messages) > 0 {
			latestMessages = probe.Data.Messages
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return latestMessages, nil
}

// scanSessions walks `sessionsDir`, returns one SessionInfo per subdirectory
// that contains `llm.jsonl`. summary.json is consulted opportunistically for
// status/turns/tokens/model/duration; missing summary is non-fatal (mid-run
// session). Sorted newest-first by directory name (ISO-8601 prefix gives a
// lexicographic ordering that matches chronological).
func scanSessions(sessionsDir string) ([]SessionInfo, error) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}

	out := make([]SessionInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(sessionsDir, name)
		if _, err := os.Stat(filepath.Join(dir, "llm.jsonl")); err != nil {
			continue
		}

		info := SessionInfo{ID: name}
		info.Task, info.Date, info.Time = parseSessionIDParts(name)

		// Best-effort summary read.
		if data, err := os.ReadFile(filepath.Join(dir, "summary.json")); err == nil {
			var sum sessionSummary
			if json.Unmarshal(data, &sum) == nil {
				info.Status = sum.Status
				info.Turns = sum.Turns
				info.Tokens = sum.TotalTok
				info.Model = sum.Model
				info.DurationMs = sum.DurationMs
			}
		}

		out = append(out, info)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}
