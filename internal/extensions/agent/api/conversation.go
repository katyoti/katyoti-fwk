// SPDX-License-Identifier: Apache-2.0

// Package api defines the agent API types.
//
// LLM types (Model, llm.MessageContent, llm.ContentPart, ToolCall, CallOption, etc.)
// are defined in the llm_*.go files in this package — copied from langchaingo
// and owned by us. No external dependency.
//
// ToolDef is our own type (richer than the generic Tool, integrated with the
// permission system).
package api

import (
	"encoding/json"

	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// ── Role aliases (convenience) ──────────────────────────────────────────

// Message is a conversation turn (alias for llm.MessageContent).
type Message = llm.MessageContent

const (
	RoleUser      = llm.ChatMessageTypeHuman
	RoleAssistant = llm.ChatMessageTypeAI
	RoleSystem    = llm.ChatMessageTypeSystem
	RoleTool      = llm.ChatMessageTypeTool
)

// ── Usage ───────────────────────────────────────────────────────────────

// Usage captures token-level accounting for a single model call, plus the
// cost computed AT SOURCE from the model's own pricing (so consumers never
// replicate provider-specific cache formulas). Model/Provider identify which
// model produced this usage — essential because one session interleaves runs
// on DIFFERENT models, each with its own rates.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`

	Model    string    `json:"model,omitempty"`
	Provider string    `json:"provider,omitempty"`
	Cost     *llm.Cost `json:"cost,omitempty"`
}

// ExtractUsage pulls usage data from GenerationInfo map.
func ExtractUsage(info map[string]any) Usage {
	u := Usage{}
	if info == nil {
		return u
	}
	if v, ok := info["InputTokens"].(int); ok {
		u.InputTokens = v
	} else if v, ok := info["PromptTokens"].(int); ok {
		u.InputTokens = v
	}
	if v, ok := info["OutputTokens"].(int); ok {
		u.OutputTokens = v
	} else if v, ok := info["CompletionTokens"].(int); ok {
		u.OutputTokens = v
	}
	if v, ok := info["CacheReadInputTokens"].(int); ok {
		u.CacheReadInputTokens = v
	}
	if v, ok := info["CacheCreationInputTokens"].(int); ok {
		u.CacheCreationInputTokens = v
	}
	return u
}

// ── ToolDef (our own — richer than the generic Tool) ────────────────────

// ToolDisplay carries rendering metadata for tool calls in the TUI/CLI.
// Defined here (agent/api) so both the tool layer and the agent loop can
// reference it without a circular import. The tools/api package re-exports
// this type and provides the WithDisplay option for tool authors.
type ToolDisplay struct {
	Icon     string `json:"icon,omitempty"`
	Label    string `json:"label,omitempty"`
	Color    string `json:"color,omitempty"`
	Template string `json:"template,omitempty"`
}

// ToolDef defines a tool available to the LLM. Carries the raw JSON schema
// for the permission system and is used throughout the tool/plugin layer.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Display     *ToolDisplay    `json:"display,omitempty"`

	// Interactive marks a tool that blocks on user input and emits its
	// own conversation events (prompt_shown before blocking, user_input
	// after). The loop suppresses its generic tool_feedback echo for
	// interactive tools — the user's answer is not "feedback" and is
	// already surfaced by the tool itself.
	Interactive bool `json:"interactive,omitempty"`

	// Polling marks a tool designed to be called repeatedly with
	// identical arguments (status checks, mailbox polls). The
	// loop-safety guard applies a higher repetition threshold to
	// polling tools instead of treating identical calls as a runaway.
	Polling bool `json:"polling,omitempty"`

	// Idle marks a tool whose repeated identical call is the canonical
	// idle shape of an interactive session (ask-user's "wait for the
	// user's next message"). The loop-safety guard exempts idle tools
	// entirely when the run is in an interactive keepalive mode;
	// outside it they keep the default threshold.
	Idle bool `json:"idle,omitempty"`

	// Blocking marks a tool that blocks until a real external event (a
	// child's bubbled question / completion, etc.) rather than returning
	// immediately. Such a tool is self-rate-limiting — it physically
	// cannot busy-spin (each call parks until an event or a timeout), and
	// MaxTurns remains the hard ceiling — so the loop-safety repetition
	// guard exempts it entirely. Distinct from Polling, which caps a
	// NON-blocking poll; a blocking await needs no cap.
	Blocking bool `json:"blocking,omitempty"`
}

// ToTool converts our ToolDef to the generic Tool type.
//
// FunctionDefinition.Parameters carries a DECODED JSON-schema map —
// every provider consumes map[string]any, never raw bytes. The decode
// happens here, once, at the boundary; a missing or invalid schema
// becomes the minimal valid object schema so providers that require
// input_schema (Anthropic spec; enforced by gateways like Azure AI
// Foundry) always receive one.
func (td ToolDef) ToTool() llm.Tool {
	var params map[string]any
	if len(td.InputSchema) > 0 {
		_ = json.Unmarshal(td.InputSchema, &params)
	}
	if params == nil {
		params = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return llm.Tool{
		Type: "function",
		Function: &llm.FunctionDefinition{
			Name:        td.Name,
			Description: td.Description,
			Parameters:  params,
		},
	}
}

// ToolDefsToTools converts a slice of ToolDefs to generic Tools.
func ToolDefsToTools(defs []ToolDef) []llm.Tool {
	tools := make([]llm.Tool, len(defs))
	for i, td := range defs {
		tools[i] = td.ToTool()
	}
	return tools
}

// ── Helper constructors ─────────────────────────────────────────────────

// NewTextMessage creates a single-text message.
func NewTextMessage(role llm.ChatMessageType, text string) Message {
	return llm.MessageContent{Role: role, Parts: []llm.ContentPart{llm.TextPart(text)}}
}

// NewToolResultMessages creates one message per tool result.
func NewToolResultMessages(results ...llm.ToolCallResponse) []Message {
	msgs := make([]Message, len(results))
	for i, r := range results {
		msgs[i] = llm.MessageContent{
			Role:  llm.ChatMessageTypeTool,
			Parts: []llm.ContentPart{r},
		}
	}
	return msgs
}
