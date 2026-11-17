// SPDX-License-Identifier: Apache-2.0

// Package terminal defines the KTS Terminal Protocol — a display-oriented
// message protocol for system-to-user communication. Sibling to KTSP
// (system-to-system). Decision:
// (modules/display/decisions/OD-007-how-does-a-running-task-reach-the-person.cwm.md);
// the protocol is described in modules/display/terminal/concepts.md §2.
package terminal

import (
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
)

// MessageContent is the discriminated union of all content types.
// The "type" field is the discriminator. UIs that don't recognize a type
// should fall back to rendering it as text or ignoring it.
type MessageContent struct {
	raw  json.RawMessage
	typ  string
	data any
}

// Type returns the content type discriminator.
func (c *MessageContent) Type() string { return c.typ }

// Raw returns the raw JSON for custom deserialization.
func (c *MessageContent) Raw() json.RawMessage { return c.raw }

// As attempts to decode the content into the given pointer type.
func (c *MessageContent) As(v any) error {
	return json.Unmarshal(c.raw, v)
}

// MarshalJSON serializes the inner content.
func (c MessageContent) MarshalJSON() ([]byte, error) {
	if c.data != nil {
		return json.Marshal(c.data)
	}
	return c.raw, nil
}

// UnmarshalJSON reads the type discriminator and stores the raw JSON.
func (c *MessageContent) UnmarshalJSON(data []byte) error {
	// A copy: the decoder may reuse data (a journal replay's scanner does),
	// which left every replayed message but the last with a later line.
	c.raw = append(json.RawMessage(nil), data...)
	var probe struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	c.typ = probe.Type
	return nil
}

// NewContent creates a MessageContent from a typed struct.
func NewContent(v any) MessageContent {
	data, _ := json.Marshal(v)
	var probe struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(data, &probe)
	return MessageContent{raw: data, typ: probe.Type, data: v}
}

// ── Core content types (MVP) ──────────────────────────────────────────

// TextContent — Markdown-formatted text. The fundamental message type.
type TextContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// ToolContent — display-oriented tool call with lifecycle status.
// A single message updated via msg.update as the tool progresses.
type ToolContent struct {
	Type     string       `json:"type"` // "tool"
	ToolName string       `json:"toolName"`
	Status   string       `json:"status"` // "running" | "success" | "error"
	Input    any          `json:"input,omitempty"`
	Output   any          `json:"output,omitempty"`
	Error    string       `json:"error,omitempty"`
	Duration int          `json:"duration,omitempty"` // milliseconds
	Display  *DisplayData `json:"display,omitempty"`
}

// SystemContent — out-of-band info, warning, error, success.
type SystemContent struct {
	Type    string `json:"type"` // "system"
	Message string `json:"message"`
	Level   string `json:"level"` // "info" | "warning" | "error" | "success"
	Title   string `json:"title,omitempty"`
	// Msg is the message's key and params when it has one (a failed model
	// call's cause): the GUI says it in the person's language, Message is
	// its English text.
	Msg *usermsg.Msg `json:"msg,omitempty"`
}

// PromptContent — user interaction prompt (text, select, confirm, form).
type PromptContent struct {
	Type        string   `json:"type"`       // "prompt"
	PromptType  string   `json:"promptType"` // "text" | "select" | "confirm" | "form"
	Question    string   `json:"question"`
	Options     []string `json:"options,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	FormSchema  any      `json:"formSchema,omitempty"` // JSON Schema + UI Schema for form prompts
	FormData    any      `json:"formData,omitempty"`   // initial value to pre-fill a form prompt (Channel=="form")
	Answered    bool     `json:"answered"`
	Answer      string   `json:"answer,omitempty"`

	// Channel is the resolved UserComms channel the ask routed to
	// ("direct" / "inbox" / …). The GUI picks the affordance from it:
	// direct asks render the inline answer input; inbox asks render
	// an "answer it in the Inbox" notice — the chat input can't
	// answer them because nothing waits on the HumanGate.
	Channel string `json:"channel,omitempty"`

	// ArtifactID correlates a Channel=="form" ask (the ask-user-data tool)
	// with the ui_event the chat sends back on submit/cancel.
	ArtifactID string `json:"artifactId,omitempty"`

	// Title is the form card heading (Channel=="form"); Question is its
	// explanation line. The chat renders the schema (FormSchema) inline
	// in-process as a form card with these.
	Title string `json:"title,omitempty"`
}

// CodeContent — syntax-highlighted code block.
type CodeContent struct {
	Type     string `json:"type"` // "code"
	Code     string `json:"code"`
	Language string `json:"language"`
	Title    string `json:"title,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// CollapsibleContent — expandable detail section.
type CollapsibleContent struct {
	Type     string         `json:"type"` // "collapsible"
	Summary  string         `json:"summary"`
	Details  MessageContent `json:"details"`
	Expanded bool           `json:"expanded,omitempty"`
}

// GroupContent — composite message with multiple content items.
type GroupContent struct {
	Type   string           `json:"type"` // "group"
	Items  []MessageContent `json:"items"`
	Title  string           `json:"title,omitempty"`
	Layout string           `json:"layout,omitempty"` // "vertical" | "horizontal"
}

// DelegationContent — sub-agent delegation display.
type DelegationContent struct {
	Type           string `json:"type"` // "delegation"
	TaskName       string `json:"taskName"`
	Mode           string `json:"mode"`   // "worker" | "background" | "fork" | ...
	Status         string `json:"status"` // "running" | "completed" | "error"
	ChildSessionID string `json:"childSessionId,omitempty"`
	Summary        string `json:"summary,omitempty"`
}

// DocumentContent — an agent-generated document that opens as a tab in the
// GUI: the display envelope (render.Document — its media type picks the
// display engine) plus the stream status. Documents stream progressively via
// msg.delta events (same mechanism as text streaming) and finalize via
// msg.update (status -> "done").
type DocumentContent struct {
	Type string `json:"type"` // "document"
	render.Document
	Status string `json:"status"` // "streaming" | "done"
}

// ComponentContent — escape hatch for custom UI components.
// UIs that don't have the target component render the message fallback.
type ComponentContent struct {
	Type    string `json:"type"`   // "component"
	Target  string `json:"target"` // component name/ID
	Data    any    `json:"data"`
	Message string `json:"message,omitempty"` // text fallback
}

// ── Content type constructors ─────────────────────────────────────────

func Text(text string) MessageContent { return NewContent(TextContent{Type: "text", Text: text}) }
func System(level, message string) MessageContent {
	return NewContent(SystemContent{Type: "system", Level: level, Message: message})
}
func Code(code, language string) MessageContent {
	return NewContent(CodeContent{Type: "code", Code: code, Language: language})
}
