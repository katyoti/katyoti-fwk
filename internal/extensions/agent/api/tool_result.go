// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
)

// ToolCallResult is the structured result of a tool call.
//
// Data holds the tool output — serialized to string for the LLM.
//
// NewMessages and ContextModifier are optional. They let a tool inject
// follow-up messages into the conversation or modify the execution context
// for subsequent tool calls in the same turn. ContextModifier is honored
// only for non-concurrent-safe tools — concurrent-safe tools that return
// one are logged and ignored (per FR-04).
type ToolCallResult struct {
	Data            any             // tool output (string, struct, or any JSON-serializable value)
	NewMessages     []Message       // optional — chained instructions injected after the tool returns
	ContextModifier ContextModifier // optional — applied between calls for non-concurrent-safe tools
}

// ContextModifier transforms the execution context for the next tool call
// in the same turn. Returned by tools that need to mutate state visible to
// subsequent tools (e.g., enter_worktree changes the working directory).
type ContextModifier func(ctx context.Context) context.Context

// StringResult creates a ToolCallResult from a plain string.
func StringResult(s string) *ToolCallResult {
	return &ToolCallResult{Data: s}
}

// ResultString serializes the result Data to a string for the LLM.
// Strings are returned as-is; other types are JSON-marshaled.
func (r *ToolCallResult) ResultString() string {
	if r == nil {
		return ""
	}
	switch v := r.Data.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return err.Error()
		}
		return string(data)
	}
}
