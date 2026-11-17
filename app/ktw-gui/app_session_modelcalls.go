// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// GetSessionModelCalls is the cold/live accessor for the HTTP inspector
// view: it parses one session's llm-model-calls.jsonl (the
// models/api.ModelCallEvent stream) into per-call records — call_start
// + structural sse_event lines + call_end grouped by call_id. A
// call_start without its call_end is an IN-FLIGHT call (live file,
// request still running).

// SessionModelCallSSE is one structural streamed frame of a call.
type SessionModelCallSSE struct {
	Type string          `json:"type"`
	TS   string          `json:"ts,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// SessionModelCall is one model call, DevTools-Network-row shaped.
type SessionModelCall struct {
	CallID   string `json:"callId"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Method   string `json:"method,omitempty"`
	URL      string `json:"url,omitempty"`
	TS       string `json:"ts,omitempty"`

	Status      int    `json:"status,omitempty"`
	DurationMs  int64  `json:"durationMs,omitempty"`
	FirstByteMs int64  `json:"firstByteMs,omitempty"`
	Error       string `json:"error,omitempty"`
	InFlight    bool   `json:"inFlight,omitempty"`

	RequestHeaders  map[string]string `json:"requestHeaders,omitempty"`
	RequestBody     json.RawMessage   `json:"requestBody,omitempty"`
	ResponseHeaders map[string]string `json:"responseHeaders,omitempty"`
	ResponseBody    json.RawMessage   `json:"responseBody,omitempty"`

	SSEEvents  []SessionModelCallSSE `json:"sseEvents,omitempty"`
	SSESummary json.RawMessage       `json:"sseSummary,omitempty"`
}

// GetSessionModelCallsResult wraps the call list. LoggingOff means the
// session has no llm-model-calls.jsonl at all — model-call logging was
// disabled for its profile (distinct from a parse error).
type GetSessionModelCallsResult struct {
	Calls      []SessionModelCall `json:"calls,omitempty"`
	LoggingOff bool               `json:"loggingOff,omitempty"`
	Error      string             `json:"error,omitempty"`
}

// GetSessionModelCalls parses `<sessionDir>/llm-model-calls.jsonl`.
// Resolution mirrors GetSessionTaskTree / GetSessionJournal exactly.
//
// Wails-bound: window.go.main.App.GetSessionModelCalls(universeName, sessionId).
func (a *App) GetSessionModelCalls(universeName, sessionId string) GetSessionModelCallsResult {
	if universeName == "" || sessionId == "" {
		return GetSessionModelCallsResult{Error: "universeName and sessionId are required"}
	}

	listResult := a.listUniverses()
	if listResult.Error != "" {
		return GetSessionModelCallsResult{Error: listResult.Error}
	}
	var workspaceRoot string
	for _, u := range listResult.Universes {
		if u.Name == universeName {
			workspaceRoot = u.HomePath
			break
		}
	}
	if workspaceRoot == "" {
		return GetSessionModelCallsResult{Error: fmt.Sprintf("universe %q not found in registry", universeName)}
	}

	sessionDir := filepath.Join(application.EnvOf(workspaceRoot).SessionsDir(), sessionId)
	if _, err := os.Stat(sessionDir); err != nil {
		return GetSessionModelCallsResult{Error: fmt.Sprintf("session %q not found in universe %q", sessionId, universeName)}
	}

	logPath := filepath.Join(sessionDir, "llm-model-calls.jsonl")
	if _, err := os.Stat(logPath); err != nil {
		// No log file — model-call logging was off for this session.
		return GetSessionModelCallsResult{LoggingOff: true}
	}

	calls, err := parseSessionModelCalls(logPath)
	if err != nil {
		return GetSessionModelCallsResult{Error: fmt.Sprintf("parse llm-model-calls.jsonl: %v", err)}
	}
	return GetSessionModelCallsResult{Calls: calls}
}

// parseSessionModelCalls groups the event stream by call_id, in
// call_start order. Unknown kinds and unparseable lines are skipped
// (a live file can end mid-line).
func parseSessionModelCalls(path string) ([]SessionModelCall, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	byID := map[string]*SessionModelCall{}
	var order []string

	call := func(id string) *SessionModelCall {
		if c, ok := byID[id]; ok {
			return c
		}
		c := &SessionModelCall{CallID: id, InFlight: true}
		byID[id] = c
		order = append(order, id)
		return c
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		var evt struct {
			Kind   string `json:"kind"`
			CallID string `json:"call_id"`
			TS     string `json:"ts"`

			Provider       string            `json:"provider"`
			Model          string            `json:"model"`
			Method         string            `json:"method"`
			URL            string            `json:"url"`
			RequestHeaders map[string]string `json:"request_headers"`
			RequestBody    json.RawMessage   `json:"request_body"`

			SSEType string          `json:"sse_type"`
			SSEData json.RawMessage `json:"sse_data"`

			Status          int               `json:"status"`
			DurationMs      int64             `json:"duration_ms"`
			FirstByteMs     int64             `json:"first_byte_ms"`
			ResponseHeaders map[string]string `json:"response_headers"`
			ResponseBody    json.RawMessage   `json:"response_body"`
			SSESummary      json.RawMessage   `json:"sse_summary"`
			Error           string            `json:"error"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil || evt.CallID == "" {
			continue
		}

		c := call(evt.CallID)
		switch evt.Kind {
		case "call_start":
			c.TS = evt.TS
			c.Provider = evt.Provider
			c.Model = evt.Model
			c.Method = evt.Method
			c.URL = evt.URL
			c.RequestHeaders = evt.RequestHeaders
			c.RequestBody = evt.RequestBody

		case "sse_event":
			c.SSEEvents = append(c.SSEEvents, SessionModelCallSSE{
				Type: evt.SSEType,
				TS:   evt.TS,
				Data: evt.SSEData,
			})

		case "call_end":
			c.InFlight = false
			c.Status = evt.Status
			c.DurationMs = evt.DurationMs
			c.FirstByteMs = evt.FirstByteMs
			c.ResponseHeaders = evt.ResponseHeaders
			c.ResponseBody = evt.ResponseBody
			c.SSESummary = evt.SSESummary
			c.Error = evt.Error
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	calls := make([]SessionModelCall, 0, len(order))
	for _, id := range order {
		calls = append(calls, *byID[id])
	}
	return calls, nil
}
