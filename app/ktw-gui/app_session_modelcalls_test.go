// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSessionModelCalls pins the HTTP-inspector projection:
// events group by call_id (start + structural sse + end), a start
// without an end is in-flight, and unparseable trailing lines (live
// file mid-write) are skipped.
func TestParseSessionModelCalls(t *testing.T) {
	jsonl := `{"kind":"call_start","call_id":"c-0001","ts":"t1","provider":"anthropic","model":"claude-haiku-4-5","method":"POST","url":"https://x.test/anthropic/v1/messages","request_headers":{"X-Api-Key":"***"},"request_body":{"model":"claude-haiku-4-5","max_tokens":16}}
{"kind":"sse_event","call_id":"c-0001","ts":"t2","sse_type":"message_start","sse_data":{"type":"message_start"}}
{"kind":"sse_event","call_id":"c-0001","ts":"t3","sse_type":"message_stop","sse_data":{"type":"message_stop"}}
{"kind":"call_end","call_id":"c-0001","ts":"t4","status":200,"duration_ms":1200,"first_byte_ms":300,"response_headers":{"Content-Type":"text/event-stream"},"sse_summary":{"event_count":10,"delta_count":8,"text":"Hello"}}
{"kind":"call_start","call_id":"c-0002","ts":"t5","provider":"anthropic","model":"claude-haiku-4-5","method":"POST","url":"https://x.test/anthropic/v1/messages"}
{"kind":"call_sta` // truncated final line: live file mid-write

	dir := t.TempDir()
	path := filepath.Join(dir, "llm-model-calls.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	calls, err := parseSessionModelCalls(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2: %+v", len(calls), calls)
	}

	c1 := calls[0]
	if c1.CallID != "c-0001" || c1.InFlight {
		t.Errorf("c1 identity/in-flight: %+v", c1)
	}
	if c1.Model != "claude-haiku-4-5" || c1.Method != "POST" || c1.Provider != "anthropic" {
		t.Errorf("c1 start fields: %+v", c1)
	}
	if c1.Status != 200 || c1.DurationMs != 1200 || c1.FirstByteMs != 300 {
		t.Errorf("c1 end fields: %+v", c1)
	}
	if len(c1.SSEEvents) != 2 || c1.SSEEvents[0].Type != "message_start" {
		t.Errorf("c1 sse events: %+v", c1.SSEEvents)
	}
	if string(c1.SSESummary) == "" {
		t.Error("c1 sse summary missing")
	}
	if got := c1.RequestHeaders["X-Api-Key"]; got != "***" {
		t.Errorf("scrubbed header lost: %q", got)
	}

	c2 := calls[1]
	if !c2.InFlight {
		t.Errorf("c2 must be in-flight (no call_end): %+v", c2)
	}
}

// TestGetSessionModelCalls_LoggingOff: an existing session without the
// jsonl file reports loggingOff, not an error.
func TestGetSessionModelCalls_LoggingOff(t *testing.T) {
	// Exercised at the parse level: the binding's os.Stat branch is
	// trivial; pin the parse-side contract that an empty file yields
	// zero calls without error (absent-file handling lives in the
	// binding and needs a universe registry — covered by live use).
	dir := t.TempDir()
	path := filepath.Join(dir, "llm-model-calls.jsonl")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	calls, err := parseSessionModelCalls(path)
	if err != nil {
		t.Fatalf("parse empty: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("calls = %d, want 0", len(calls))
	}
}
