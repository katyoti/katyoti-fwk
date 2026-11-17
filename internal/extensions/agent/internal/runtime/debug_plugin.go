// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// debugPlugin is a built-in AgentPlugin that writes structured NLJSON entries
// for every hook point. Enables machine-readable tracing of agent behavior.
//
// The output is designed for multi-pane terminal monitoring via jq filters:
//
//	LLM pane:    jq 'select(.type | test("pre_api_call|post_api_call"))'
//	Tools pane:  jq 'select(.type | test("pre_tool_exec|post_tool_exec"))'
//	Stream pane: jq 'select(.type == "model_stream")'
//	Lifecycle:   jq 'select(.type | test("turn_|decision|result|error"))'
type debugPlugin struct {
	w io.Writer
}

// NewDebugPlugin creates a debug plugin that writes NLJSON to w.
func NewDebugPlugin(w io.Writer) api.AgentPlugin {
	return &debugPlugin{w: w}
}

func (p *debugPlugin) Name() string            { return "debug" }
func (p *debugPlugin) SystemMessage() string   { return "" }
func (p *debugPlugin) ToolDefs() []api.ToolDef { return nil }
func (p *debugPlugin) Stable() bool            { return true }
func (p *debugPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("debug plugin has no tools")
}

func (p *debugPlugin) Hooks() api.PluginHooks {
	return api.PluginHooks{
		OnTurnStart: func(_ api.LoopContext, turn int) error {
			return p.emit("turn_start", map[string]any{"turn": turn})
		},
		OnPreAPICall: func(_ api.LoopContext, messages []api.Message) error {
			fields := map[string]any{"message_count": len(messages)}
			// Extract the last user message as the prompt being sent.
			if text := lastUserText(messages); text != "" {
				fields["last_user_message"] = truncate(text, 2000)
			}
			return p.emit("pre_api_call", fields)
		},
		OnModelStream: func(_ api.LoopContext, chunk api.StreamChunk) error {
			fields := map[string]any{
				"chunk_type": chunk.Type,
				"delta_len":  len(chunk.Delta),
			}
			if chunk.Delta != "" {
				fields["delta"] = chunk.Delta
			}
			if chunk.ToolName != "" {
				fields["tool_name"] = chunk.ToolName
			}
			return p.emit("model_stream", fields)
		},
		OnPostAPICall: func(_ api.LoopContext, response api.Message) error {
			fields := map[string]any{
				"block_count": len(response.Parts),
			}
			// Extract text content and tool calls from the response.
			var textParts []string
			var toolCalls []map[string]any
			for _, part := range response.Parts {
				switch v := part.(type) {
				case llm.TextContent:
					if v.Text != "" {
						textParts = append(textParts, v.Text)
					}
				case llm.ToolCall:
					tc := map[string]any{
						"tool": v.FunctionCall.Name,
						"id":   v.ID,
					}
					if v.FunctionCall.Arguments != "" {
						tc["arguments"] = truncate(v.FunctionCall.Arguments, 500)
					}
					toolCalls = append(toolCalls, tc)
				}
			}
			if len(textParts) > 0 {
				fields["text"] = truncate(joinStrings(textParts), 3000)
			}
			if len(toolCalls) > 0 {
				fields["tool_calls"] = toolCalls
			}
			return p.emit("post_api_call", fields)
		},
		OnPreToolExec: func(_ api.LoopContext, toolName string, input map[string]any) (*api.ToolDecision, error) {
			fields := map[string]any{"tool": toolName}
			if len(input) > 0 {
				// Serialize input for display; truncate large values.
				inputJSON, err := json.Marshal(input)
				if err == nil {
					fields["input"] = truncate(string(inputJSON), 1000)
				}
			}
			p.emit("pre_tool_exec", fields)
			return nil, nil // allow
		},
		OnPostToolExec: func(_ api.LoopContext, results []api.ToolResult) error {
			errorCount := 0
			var details []map[string]any
			for _, r := range results {
				if r.IsError {
					errorCount++
				}
				d := map[string]any{
					"tool":     r.ToolName,
					"id":       r.ToolUseID,
					"is_error": r.IsError,
					"result":   truncate(r.Content, 1000),
				}
				details = append(details, d)
			}
			return p.emit("post_tool_exec", map[string]any{
				"result_count": len(results),
				"error_count":  errorCount,
				"results":      details,
			})
		},
		OnDecision: func(_ api.LoopContext, decision *api.Decision) error {
			return p.emit("decision", map[string]any{
				"continue":      decision.Continue,
				"message_count": len(decision.Messages),
			})
		},
		OnTurnEnd: func(_ api.LoopContext, turn int) error {
			return p.emit("turn_end", map[string]any{"turn": turn})
		},
		OnError: func(_ api.LoopContext, err *api.LoopError) error {
			return p.emit("error", map[string]any{
				"phase":     err.Phase,
				"error":     err.Err.Error(),
				"recovered": err.Recovered,
			})
		},
		OnAbort: func(_ api.LoopContext) error {
			return p.emit("abort", nil)
		},
		OnResult: func(ctx api.LoopContext, result *api.LoopResult) error {
			return p.emit("result", map[string]any{
				"stop_reason": result.StopReason,
				"turn_count":  result.TurnCount,
				"usage":       result.Usage,
			})
		},
	}
}

func (p *debugPlugin) emit(entryType string, fields map[string]any) error {
	entry := map[string]any{
		"type":      entryType,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range fields {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return nil // don't fail the loop on marshal error
	}
	data = append(data, '\n')
	_, _ = p.w.Write(data)
	return nil
}

// lastUserText extracts text from the last user message in the conversation.
func lastUserText(messages []api.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.ChatMessageTypeHuman {
			for _, part := range messages[i].Parts {
				if tc, ok := part.(llm.TextContent); ok {
					return tc.Text
				}
			}
		}
	}
	return ""
}

// truncate limits s to maxLen characters, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// joinStrings concatenates parts with double newlines.
func joinStrings(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += "\n\n" + p
	}
	return result
}
