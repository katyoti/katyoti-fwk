// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
	"time"
)

// TestUIStreamingEventsRegistered (UC-081) — the four UI-streaming event
// types that flow from the agent runtime to the displaybridge are present
// in `AllEventTypes` AND auto-registered in `EventRegistry` so JSON
// discriminator decoding works for any subscriber.
//
// Events covered:
//   - agent.text_delta (streaming text deltas during assistant response)
//   - agent.message_shown (final assembled message after streaming completes)
//   - agent.tool_call (tool invocation request)
//   - agent.tool_result (tool completion)
//
// Without these events being registered + serializable, the displaybridge
// (and any other subscriber) cannot reconstruct typed events from the wire,
// breaking UI streaming end-to-end.
func TestUIStreamingEventsRegistered(t *testing.T) {
	cases := []struct {
		uc     string
		event  string
		sample AgentEvent
	}{
		{"UC-081 streaming text deltas", EventTextDelta, TextDeltaEvent{}},
		{"UC-081 final assembled message", EventMessageShown, MessageShownEvent{}},
		{"UC-081 tool call event", EventToolCall, ToolCallEvent{}},
		{"UC-081 tool result event", EventToolResult, ToolResultEvent{}},
	}

	// 1. Every UI streaming event appears in AllEventTypes.
	listed := make(map[string]bool, len(AllEventTypes))
	for _, t := range AllEventTypes {
		listed[t] = true
	}
	for _, c := range cases {
		if !listed[c.event] {
			t.Errorf("%s: event %q not in AllEventTypes", c.uc, c.event)
		}
	}

	// 2. JSON discriminator round-trip works for each: serialize a stamped
	// event, deserialize via EventRegistry, assert the type discriminator
	// recovers the same event kind.
	for _, c := range cases {
		// Stamp via Type field directly since we don't have a base struct here.
		// We marshal a minimal payload and ensure the registry recognises the type.
		payload := map[string]any{
			"type":    c.event,
			"ts":      time.Now().UTC().Format(time.RFC3339Nano),
			"emitter": "test",
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("%s: marshal: %v", c.uc, err)
		}
		decoded, err := EventRegistry.Deserialize(raw)
		if err != nil {
			t.Errorf("%s: registry deserialize %q: %v", c.uc, c.event, err)
			continue
		}
		if decoded.AgentEventType() != c.event {
			t.Errorf("%s: decoded type = %q, want %q", c.uc, decoded.AgentEventType(), c.event)
		}
	}
}
