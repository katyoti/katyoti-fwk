// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"time"
)

// ModelCallEvent is one structured observation of a model call. The
// per-session llm-model-calls.jsonl is a stream of these, one JSON
// object per line — THIS STRUCT IS THE FILE'S SCHEMA (and the contract
// of the ktw-gui HTTP inspector). Events group by CallID:
//
//	call_start  — method/url/model + scrubbed request headers + decoded body
//	sse_event   — one STRUCTURAL streamed frame (message_start,
//	              content_block_start/stop, message_delta, message_stop,
//	              error; per-token deltas are aggregated, not emitted)
//	call_end    — status, durations, response (decoded body for non-SSE,
//	              SSESummary for streams), usage, error
type ModelCallEvent struct {
	Kind   string    `json:"kind"` // call_start | sse_event | call_end
	CallID string    `json:"call_id"`
	TS     time.Time `json:"ts"`

	// call_start
	Provider       string            `json:"provider,omitempty"`
	Model          string            `json:"model,omitempty"`
	Method         string            `json:"method,omitempty"`
	URL            string            `json:"url,omitempty"`
	RequestHeaders map[string]string `json:"request_headers,omitempty"` // credential values scrubbed
	RequestBody    json.RawMessage   `json:"request_body,omitempty"`

	// sse_event
	SSEType string          `json:"sse_type,omitempty"`
	SSEData json.RawMessage `json:"sse_data,omitempty"`

	// call_end
	Status          int               `json:"status,omitempty"`
	DurationMs      int64             `json:"duration_ms,omitempty"`
	FirstByteMs     int64             `json:"first_byte_ms,omitempty"`
	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	ResponseBody    json.RawMessage   `json:"response_body,omitempty"` // non-SSE responses
	SSESummary      *SSESummary       `json:"sse_summary,omitempty"`   // streamed responses
	Error           string            `json:"error,omitempty"`
}

// SSESummary aggregates a streamed response's delta frames (which are
// not emitted individually) on the call_end event.
type SSESummary struct {
	// EventCount is the total number of SSE frames received.
	EventCount int `json:"event_count"`
	// DeltaCount is the number of aggregated incremental frames
	// (text / input_json deltas).
	DeltaCount int `json:"delta_count"`
	// Text is the assembled text content of the response.
	Text string `json:"text,omitempty"`
	// Contents is the response reassembled as one entry per received
	// content block, in arrival order — text blocks with their full
	// text, tool_use blocks with their name + assembled input JSON.
	Contents []SSEContent `json:"contents,omitempty"`
}

// SSEContent is one reassembled response content block.
type SSEContent struct {
	// Type is the block type (text, tool_use, thinking, …).
	Type string `json:"type"`
	// Name is the tool name for tool_use blocks.
	Name string `json:"name,omitempty"`
	// ID is the tool-use id for tool_use blocks.
	ID string `json:"id,omitempty"`
	// Text is the assembled text (text / thinking blocks).
	Text string `json:"text,omitempty"`
	// Input is the assembled input JSON (tool_use blocks).
	Input json.RawMessage `json:"input,omitempty"`
}

// ModelCallObserver receives structured model-call events. Sinks own
// serialization (the session JSONL log marshals one event per line).
// Implementations must tolerate concurrent calls: SSE events are
// emitted from the stream-consumer goroutine.
type ModelCallObserver interface {
	ObserveModelCall(evt ModelCallEvent)
}

// WithCallObserver attaches a structured observer to this model call:
// it receives call_start / sse_event / call_end events (auth headers
// scrubbed, bodies decoded). Used by the agent loop to produce the
// per-session llm-model-calls.jsonl. Providers without observer
// support ignore the option.
func WithCallObserver(o ModelCallObserver) CallOption {
	return func(opts *CallOptions) {
		opts.CallObserver = o
	}
}
