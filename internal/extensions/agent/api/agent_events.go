// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/union"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// Agent event type constants.
const (
	EventSessionStart  = "agent.session_start"
	EventSessionEnd    = "agent.session_end"
	EventTurnStart     = "agent.turn_start"
	EventLLMRequest    = "agent.llm_request"
	EventLLMResponse   = "agent.llm_response"
	EventTextDelta     = "agent.text_delta"
	EventMessageShown  = "agent.message_shown"
	EventToolCall      = "agent.tool_call"
	EventToolResult    = "agent.tool_result"
	EventToolError     = "agent.tool_error"
	EventToolFeedback  = "agent.tool_feedback"
	EventPromptShown   = "agent.prompt_shown"
	EventUserInput     = "agent.user_input"
	EventComplete      = "agent.complete"
	EventError         = "agent.error"
	EventDocument      = "agent.document"
	EventDocumentDelta = "agent.document_delta"
	// UI artifact events (display-ui-component / patch-ui-component /
	// complete-ui-component tools — display-dsl-framework, ms06).
	EventUIArtifactMount    = "agent.ui_artifact_mount"
	EventUIArtifactPatch    = "agent.ui_artifact_patch"
	EventUIArtifactComplete = "agent.ui_artifact_complete"
	// A display panel placed in the conversation: a question
	// asked from a chat shows there as its card.
	EventPanelShown = "agent.panel_shown"
)

// AllEventTypes lists all agent event type constants (for bulk subscription).
var AllEventTypes = []string{
	EventSessionStart, EventSessionEnd, EventTurnStart,
	EventLLMRequest, EventLLMResponse, EventTextDelta,
	EventMessageShown, EventToolCall, EventToolResult,
	EventToolError, EventToolFeedback, EventPromptShown,
	EventUserInput, EventComplete, EventError,
	EventDocument, EventDocumentDelta,
	EventUIArtifactMount, EventUIArtifactPatch, EventUIArtifactComplete,
	EventPanelShown,
}

// EventBase is embedded by all agent events. Provides type + timestamp + emitter.
type EventBase struct {
	Type    string    `json:"type"`
	Time    time.Time `json:"ts"`
	Emitter string    `json:"emitter,omitempty"`
}

// AgentEvent is the interface all agent events satisfy (via EventBase embedding).
type AgentEvent interface {
	AgentEventType() string
	GetEmitter() string
}

func (b EventBase) AgentEventType() string { return b.Type }
func (b EventBase) GetEmitter() string     { return b.Emitter }

// StampEvent populates the base metadata. Called by the executor before publishing.
func (b *EventBase) StampEvent(eventType string, emitter string) {
	b.Type = eventType
	b.Time = time.Now()
	b.Emitter = emitter
}

// EventRegistry is the union registry for agent events.
// Enables JSON deserialization: `event, err := EventRegistry.Deserialize(raw)`.
var EventRegistry = union.NewRegistry[AgentEvent]("type")

func init() {
	EventRegistry.Register(EventSessionStart, SessionStartEvent{})
	EventRegistry.Register(EventSessionEnd, SessionEndEvent{})
	EventRegistry.Register(EventTurnStart, TurnStartEvent{})
	EventRegistry.Register(EventLLMRequest, LLMRequestEvent{})
	EventRegistry.Register(EventLLMResponse, LLMResponseEvent{})
	EventRegistry.Register(EventTextDelta, TextDeltaEvent{})
	EventRegistry.Register(EventMessageShown, MessageShownEvent{})
	EventRegistry.Register(EventToolCall, ToolCallEvent{})
	EventRegistry.Register(EventToolResult, ToolResultEvent{})
	EventRegistry.Register(EventToolError, ToolErrorEvent{})
	EventRegistry.Register(EventToolFeedback, ToolFeedbackEvent{})
	EventRegistry.Register(EventPromptShown, PromptShownEvent{})
	EventRegistry.Register(EventUserInput, UserInputEvent{})
	EventRegistry.Register(EventComplete, CompleteEvent{})
	EventRegistry.Register(EventError, ErrorEvent{})
	EventRegistry.Register(EventDocument, DocumentEvent{})
	EventRegistry.Register(EventDocumentDelta, DocumentDeltaEvent{})
	EventRegistry.Register(EventUIArtifactMount, UIArtifactMountEvent{})
	EventRegistry.Register(EventUIArtifactPatch, UIArtifactPatchEvent{})
	EventRegistry.Register(EventUIArtifactComplete, UIArtifactCompleteEvent{})
	EventRegistry.Register(EventPanelShown, PanelShownEvent{})
}

// --- Typed event structs (all embed EventBase) ---

type SessionStartEvent struct {
	EventBase
	Name string `json:"name"`
	// ParentRunID is set when this run was spawned by another run via delegate
	// or spawn_teammate. Empty for top-level runs. Used by ktw-utils to
	// reconstruct the parent → child delegation tree from session journals.
	// The current run's own ID is in EventBase.Emitter (format: "run-{id}").
	ParentRunID string `json:"parentRunId,omitempty"`
	// Mode is the resolved delegation mode (worker, coordinator, teammate, etc.).
	// Logged here so reports can color/style nodes by mode without re-parsing TASK.md.
	Mode     string `json:"mode,omitempty"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	// Cost is NOT carried here: it's computed AT SOURCE on each call and rides
	// on Usage.Cost (priced per-model, cache-aware). A session can interleave
	// models, so a single session-level rate would be wrong — consumers sum
	// Usage.Cost instead.
	ContextWindow int `json:"contextWindow,omitempty"`
}

type SessionEndEvent struct {
	EventBase
	Turns      int   `json:"turns"`
	Usage      Usage `json:"usage"`
	DurationMs int64 `json:"durationMs"`
}

type TurnStartEvent struct {
	EventBase
	Turn     int `json:"turn"`
	MaxTurns int `json:"maxTurns"`
}

type LLMRequestEvent struct {
	EventBase
	Turn         int       `json:"turn"`
	MessageCount int       `json:"messageCount"`
	ToolCount    int       `json:"toolCount"`
	Messages     []Message `json:"messages,omitempty"`
	SystemPrompt string    `json:"systemPrompt,omitempty"`
	ToolNames    []string  `json:"toolNames,omitempty"`
}

type LLMResponseEvent struct {
	EventBase
	Turn       int                  `json:"turn"`
	StopReason string               `json:"stopReason"`
	Blocks     int                  `json:"blocks"`
	Usage      Usage                `json:"usage"`
	Content    []*llm.ContentChoice `json:"content,omitempty"`
	DurationMs int64                `json:"durationMs"`
	TTFTMs     int64                `json:"ttftMs"`
}

type TextDeltaEvent struct {
	EventBase
	Text string `json:"text"`
	Turn int    `json:"turn"`
}

type MessageShownEvent struct {
	EventBase
	Text string `json:"text"`
	Turn int    `json:"turn"`
}

type ToolCallEvent struct {
	EventBase
	Turn    int             `json:"turn"`
	Tool    string          `json:"tool"`
	ID      string          `json:"id"`
	Input   json.RawMessage `json:"input"`
	Display *ToolDisplay    `json:"display,omitempty"`
}

type ToolResultEvent struct {
	EventBase
	Turn       int    `json:"turn"`
	Tool       string `json:"tool"`
	ID         string `json:"id"`
	Result     string `json:"result"`
	DurationMs int64  `json:"durationMs"`
}

type ToolErrorEvent struct {
	EventBase
	Turn       int    `json:"turn"`
	Tool       string `json:"tool"`
	ID         string `json:"id"`
	Error      string `json:"error"`
	DurationMs int64  `json:"durationMs"`
}

type ToolFeedbackEvent struct {
	EventBase
	Tool       string `json:"tool"`
	Summary    string `json:"summary"`
	DurationMs int64  `json:"durationMs"`
}

type PromptShownEvent struct {
	EventBase
	Turn       int      `json:"turn"`
	Question   string   `json:"question,omitempty"`
	PromptType string   `json:"promptType,omitempty"`
	Options    []string `json:"options,omitempty"`
	FormSchema any      `json:"formSchema,omitempty"` // JSON Schema + UI Schema for form prompts
	FormData   any      `json:"formData,omitempty"`   // initial value to pre-fill a form prompt (channel=="form")

	// Channel is the resolved UserComms channel the ask routed to
	// ("direct" / "inbox" / "none" / …). UIs pick the affordance from
	// it: a direct ask gets the inline answer input; an inbox ask
	// shows an "answer it in the Inbox" notice instead — typing into
	// the chat input would be a dead end because nothing is waiting
	// on the HumanGate. The ask-user-data tool sets Channel="form".
	Channel string `json:"channel,omitempty"`

	// ArtifactID correlates a Channel=="form" ask (the ask-user-data tool)
	// with the ui_event the chat sends back on submit/cancel.
	ArtifactID string `json:"artifactId,omitempty"`

	// Title is the form card heading when Channel=="form" (Question is the
	// explanation line). The chat renders FormSchema inline as a form card.
	Title string `json:"title,omitempty"`
}

type UserInputEvent struct {
	EventBase
	Turn   int    `json:"turn"`
	Answer string `json:"answer,omitempty"`
}

type CompleteEvent struct {
	EventBase
	Status   string        `json:"status"`
	Summary  string        `json:"summary"`
	Turns    int           `json:"turns"`
	Usage    Usage         `json:"usage"`
	Duration time.Duration `json:"duration"`
	// Error is why a run that did not complete ended (its error's text),
	// Msg its message key when it carries one.
	Error string       `json:"error,omitempty"`
	Msg   *usermsg.Msg `json:"msg,omitempty"`
}

type ErrorEvent struct {
	EventBase
	Turn  int    `json:"turn"`
	Error string `json:"error"`
	// Msg is the error's message key - a failed model call's cause, said in
	// the person's language with its next step.
	Msg *usermsg.Msg `json:"msg,omitempty"`
}

// DocumentEvent is fired when an agent produces a document (via the
// create-document tool). Opens a new tab in the GUI.
// Fires once when the tool completes — carries the final full content.
type DocumentEvent struct {
	EventBase
	Turn      int    `json:"turn"`
	ID        string `json:"id"`        // stable document ID (matches DocumentDeltaEvent.ID)
	Title     string `json:"title"`     // tab label
	MediaType string `json:"mediaType"` // text/markdown, text/html, … (render media types)
	Content   string `json:"content"`   // full content
}

// DocumentDeltaEvent streams partial document content as the LLM
// generates the `content` field of a create-document tool call.
// Fires many times per document as the LLM streams tokens. Pairs
// with DocumentEvent (fires once at the end with the full content).
type DocumentDeltaEvent struct {
	EventBase
	ID    string `json:"id"`    // document ID (same as DocumentEvent.ID)
	Delta string `json:"delta"` // newly-arrived characters
	// Content, when set, is the whole document so far and replaces what
	// was shown: a document redrawn as it streams (a diagram), not
	// appended to.
	Content   string `json:"content,omitempty"`
	Title     string `json:"title,omitempty"`     // first time only: tab label
	MediaType string `json:"mediaType,omitempty"` // first time only: the document's media type
}

// UIArtifactMountEvent — published by the display-ui-component tool's
// stream plugin when the LLM-emitted artifact has enough info parsed to
// mount in the GUI (artifactId + kind + initial schema/value/dslTree).
// The session's translator hands it to the chat panel bridge
// (terminal/chatpanel): the artifact becomes a node of the run's chat
// panel, the one feed every chat draws it from.
//
// Wired by display-dsl-framework (ms06, 2026-05-05).
type UIArtifactMountEvent struct {
	EventBase
	ArtifactID string         `json:"artifactId"` // stable id assigned by the LLM
	Kind       string         `json:"kind"`       // "form"|"view"|"table"|"wizard"|"page"
	Schema     any            `json:"schema,omitempty"`
	Variant    string         `json:"variant,omitempty"` // the ui-<kind> extension, by name
	UI         map[string]any `json:"ui,omitempty"`      // an inline ui-<kind> block laid over the schema
	Value      any            `json:"value,omitempty"`
	DSLTree    any            `json:"dslTree,omitempty"` // set when kind == "page"
	Data       any            `json:"data,omitempty"`    // outer data for dataField bindings (display-package-restructure Phase 3)
	Mode       string         `json:"mode,omitempty"`
	Title      string         `json:"title,omitempty"`   // optional tab label
	Actions    any            `json:"actions,omitempty"` // Mode-B declarative action shortcuts (US-S3); shape: map[actionId]{tool, argsTemplate}
}

// UIArtifactPatchEvent — published when the partial-JSON parser detects
// that the streamed args have grown to a new parseable state, OR when
// the patch-ui-component tool is invoked across turns. The chat panel
// bridge applies the ops to the artifact and replaces its node.
type UIArtifactPatchEvent struct {
	EventBase
	ArtifactID string `json:"artifactId"`
	// Ops are wire-format-compatible with terminal.UIPatchOp (kept as
	// `any` here to avoid an import cycle agent/api → terminal). The
	// chat panel bridge coerces.
	Ops []any `json:"ops"`
}

// PanelShownEvent places a display panel in the run's conversation
// : the chat panel bridge (terminal/chatpanel) adds a `panel`
// node for it to the conversation, drawn bare - its page alone - where it
// came, so a question asked from a chat shows there as its card. Its
// emitter is the run whose conversation shows it.
type PanelShownEvent struct {
	EventBase
	PanelID string `json:"panelId"`
	Title   string `json:"title,omitempty"`
}

// UIArtifactCompleteEvent — published by the complete-ui-component
// tool, OR automatically by the display-ui-component stream plugin
// when the tool call's tool_use_done fires. The chat panel bridge marks
// the artifact's node complete.
type UIArtifactCompleteEvent struct {
	EventBase
	ArtifactID string `json:"artifactId"`
}
