// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TraceObserver implements agentapi.Observer by writing a human-readable trace of
// key agent events to an io.Writer (typically os.Stderr). It is wired in when
// the user passes the --trace flag. Text-delta and low-signal events are
// intentionally omitted to keep the trace readable.
type TraceObserver struct {
	w     io.Writer
	start time.Time
}

// NewTraceObserver creates a TraceObserver that writes to w.
func NewTraceObserver(w io.Writer) *TraceObserver {
	return &TraceObserver{w: w, start: time.Now()}
}

func (t *TraceObserver) elapsed() string {
	return fmt.Sprintf("+%.3fs", time.Since(t.start).Seconds())
}

func str(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		// json.RawMessage is a named []byte type — must assert as json.RawMessage
		if b, ok := v.(json.RawMessage); ok {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// OnEvent prints trace lines for key events; silently ignores high-volume or
// low-signal events (text_delta, llm_request/response, message_shown, etc.).
func (t *TraceObserver) OnEvent(_ context.Context, eventType string, turn int, data any) {
	d, _ := data.(map[string]any)

	switch eventType {
	case agentapi.EventTurnStart:
		maxTurns := str(d, "maxTurns")
		fmt.Fprintf(t.w, "[trace %s] turn %d/%s start\n", t.elapsed(), turn, maxTurns)

	case agentapi.EventToolCall:
		tool := str(d, "tool")
		input := str(d, "input")
		if len(input) > 120 {
			input = input[:120] + "…"
		}
		fmt.Fprintf(t.w, "[trace %s] → %s(%s)\n", t.elapsed(), tool, input)

	case agentapi.EventToolResult:
		tool := str(d, "tool")
		result := str(d, "result")
		if len(result) > 120 {
			result = result[:120] + "…"
		}
		ms := str(d, "duration_ms")
		fmt.Fprintf(t.w, "[trace %s] ← %s: %s (%sms)\n", t.elapsed(), tool, result, ms)

	case agentapi.EventToolError:
		tool := str(d, "tool")
		errMsg := str(d, "error")
		ms := str(d, "duration_ms")
		fmt.Fprintf(t.w, "[trace %s] ✗ %s: %s (%sms)\n", t.elapsed(), tool, errMsg, ms)

	case agentapi.EventComplete:
		status := str(d, "status")
		summary := str(d, "summary")
		if len(summary) > 160 {
			summary = summary[:160] + "…"
		}
		fmt.Fprintf(t.w, "[trace %s] ✓ COMPLETE [%s] %s\n", t.elapsed(), status, summary)

	case agentapi.EventError:
		errMsg := str(d, "error")
		fmt.Fprintf(t.w, "[trace %s] ✗ ERROR: %s\n", t.elapsed(), errMsg)
	}
}

// Close is a no-op for TraceObserver; the caller owns the writer.
func (t *TraceObserver) Close() error { return nil }
