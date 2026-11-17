// SPDX-License-Identifier: Apache-2.0

package terminal

import (
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// ── Server → Client events (push) ────────────────────────────────────

// MsgNewEvent — new message appended to a session's log.
type MsgNewEvent struct {
	Type    string  `json:"type"` // "msg.new"
	Session string  `json:"session"`
	SeqNo   int     `json:"seqNo"`
	Msg     Message `json:"msg"`
}

// MsgUpdateEvent — update an existing message (tool status, prompt answer).
type MsgUpdateEvent struct {
	Type    string         `json:"type"` // "msg.update"
	Session string         `json:"session"`
	ID      string         `json:"id"`      // message ID to update
	Content MessageContent `json:"content"` // new content (replaces previous)
}

// MsgDeltaEvent — ephemeral streaming text chunk (not stored).
type MsgDeltaEvent struct {
	Type    string `json:"type"` // "msg.delta"
	Session string `json:"session"`
	ID      string `json:"id"`    // message ID being streamed
	Delta   string `json:"delta"` // text chunk to append
}

// MsgDoneEvent — streaming complete for a message.
type MsgDoneEvent struct {
	Type    string `json:"type"` // "msg.done"
	Session string `json:"session"`
	ID      string `json:"id"`
}

// StateSyncEvent — connect handshake response with full state.
type StateSyncEvent struct {
	Type     string    `json:"type"` // "state.sync"
	Sessions []Session `json:"sessions"`
	History  []Message `json:"history"` // last N messages across sessions
	// Panels are the chat panel events the sessions journaled, in their
	// order, the replaces a later one makes stale left out: a reopened
	// session draws its artifacts from them.
	Panels []PanelEvent `json:"panels,omitempty"`
}

// StateUpdateEvent — state change notification (future: todos, tasks).
type StateUpdateEvent struct {
	Type       string          `json:"type"` // "state.update"
	UpdateType string          `json:"updateType"`
	Data       json.RawMessage `json:"data"`
}

// SessionCreatedEvent — new session started (e.g., background delegation).
type SessionCreatedEvent struct {
	Type    string  `json:"type"` // "session.created"
	Session Session `json:"session"`
}

// ── Client → Server events (commands) ────────────────────────────────

// ConnectCmd — handshake request from client.
type ConnectCmd struct {
	Type   string     `json:"type"` // "connect"
	Params ConnectOpt `json:"params"`
}

type ConnectOpt struct {
	LastSeqNo int `json:"lastSeqNo,omitempty"` // resume from this point
}

// AnswerCmd — respond to a prompt.
type AnswerCmd struct {
	Type   string    `json:"type"` // "answer"
	Params AnswerOpt `json:"params"`
}

type AnswerOpt struct {
	ID    string `json:"id"`    // prompt message ID
	Value string `json:"value"` // user's answer
}

// PermissionCmd — respond to a permission gate.
type PermissionCmd struct {
	Type   string        `json:"type"` // "permission"
	Params PermissionOpt `json:"params"`
}

type PermissionOpt struct {
	ID       string `json:"id"`
	Decision string `json:"decision"` // "deny_once" | "allow_once" | "allow_always" | "deny_always"
	Rule     string `json:"rule,omitempty"`
}

// StartTaskCmd — start an agent task.
type StartTaskCmd struct {
	Type   string       `json:"type"` // "start_task"
	Params StartTaskOpt `json:"params"`
}

type StartTaskOpt struct {
	TaskName string         `json:"taskName"`
	Input    map[string]any `json:"input,omitempty"`
}

// StopTaskCmd — cancel a running task.
type StopTaskCmd struct {
	Type   string      `json:"type"` // "stop_task"
	Params StopTaskOpt `json:"params"`
}

type StopTaskOpt struct {
	RunID string `json:"runId"`
}

// CallCmd — bridge to any KTSP method via the Room.
type CallCmd struct {
	Type   string  `json:"type"` // "call"
	Params CallOpt `json:"params"`
}

type CallOpt struct {
	Path string         `json:"path"` // KTSP path: "slot/method" or "container/slot/method"
	Args map[string]any `json:"args,omitempty"`
}

// SendMessageCmd — direct message from the GUI to a teammate.
// The room dispatcher publishes engine.AgentMessageEvent on
// engine.AgentMessageTopic(To) with From="gui-user". The orchestration
// plugin's mailbox bridge delivers it to the recipient's check-messages.
type SendMessageCmd struct {
	Type   string         `json:"type"` // "send_message"
	Params SendMessageOpt `json:"params"`
}

type SendMessageOpt struct {
	To      string `json:"to"`                // recipient teammate name (required)
	Message string `json:"message"`           // body (required)
	Summary string `json:"summary,omitempty"` // optional one-liner shown in cards
}

// CancelChildCmd — cancel a running spawned child (teammate or workflow branch)
// from the GUI. The room dispatcher calls Spawner.Cancel(runID, reason) which
// emits to.<name>.cancel; the spawner cleans up the child's context and the
// existing translator subscription emits child.<name>.failed → the team card
// transitions to "cancelled" state automatically.
type CancelChildCmd struct {
	Type   string         `json:"type"` // "cancel_child"
	Params CancelChildOpt `json:"params"`
}

type CancelChildOpt struct {
	RunID     string `json:"runId"`               // spawner's run ID for the target child (required)
	ChildName string `json:"childName,omitempty"` // human-readable name for logging/UX (optional but recommended)
	Reason    string `json:"reason,omitempty"`    // optional human reason (defaults to "gui-cancel" in the dispatcher)
}

// ── Team / multi-agent events (push) ────────────────────────────────
//
// These events bridge the ms08 task engine topics into the terminal
// protocol so the GUI Team tab can render live multi-agent state. The
// translator subscribes to the corresponding bus topics and republishes
// as one of these typed events.

// TeamWorkflowStartedEvent — a workflow run has begun.
// Bridges engine topic: workflow.<runID>.started
type TeamWorkflowStartedEvent struct {
	Type  string `json:"type"` // "team.workflow_started"
	RunID string `json:"runId"`
	// ParentRunID is empty for a true top-level run and set for a NESTED
	// workflow (a `type: workflow` step inside another workflow). The GUI uses
	// it to nest the sub-workflow under its parent instead of forking a second
	// root scope (which fragmented the Team graph + run selector).
	ParentRunID string         `json:"parentRunId,omitempty"`
	Name        string         `json:"name,omitempty"`
	Inputs      map[string]any `json:"inputs,omitempty"`
	Timestamp   string         `json:"ts"`
}

// TeamWorkflowDoneEvent — workflow finished successfully.
// Bridges engine topic: workflow.<runID>.done
type TeamWorkflowDoneEvent struct {
	Type      string `json:"type"` // "team.workflow_done"
	RunID     string `json:"runId"`
	Output    any    `json:"output,omitempty"`
	Timestamp string `json:"ts"`
}

// TeamWorkflowFailedEvent — workflow ended in error.
// Bridges engine topic: workflow.<runID>.failed
type TeamWorkflowFailedEvent struct {
	Type      string `json:"type"` // "team.workflow_failed"
	RunID     string `json:"runId"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"ts"`
}

// TeamChildStartedEvent — a child machine was spawned.
// Bridges engine topic: child.<name>.started
type TeamChildStartedEvent struct {
	Type        string `json:"type"` // "team.child_started"
	RunID       string `json:"runId"`
	ParentRunID string `json:"parentRunId,omitempty"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	// Mode is the child's REAL delegation mode (worker / background / fork /
	// coordinator / …) — the Team tab renders edges from it instead of
	// guessing (it used to hardcode 'worker' when the event lacked the field).
	Mode      string `json:"mode,omitempty"`
	Timestamp string `json:"ts"`
}

// TeamChildDoneEvent — a child machine completed successfully.
// Bridges engine topic: child.<name>.done
type TeamChildDoneEvent struct {
	Type      string `json:"type"` // "team.child_done"
	RunID     string `json:"runId"`
	Name      string `json:"name"`
	Output    any    `json:"output,omitempty"`
	Timestamp string `json:"ts"`
}

// TeamChildFailedEvent — a child machine ended in error.
// Bridges engine topic: child.<name>.failed
type TeamChildFailedEvent struct {
	Type      string `json:"type"` // "team.child_failed"
	RunID     string `json:"runId"`
	Name      string `json:"name"`
	Error     string `json:"error,omitempty"`
	Timestamp string `json:"ts"`
}

// TeamMessageEvent — agent-to-agent message (direct or group broadcast).
// Bridges engine topics:
//   - to.<name>.message   → To = name, Team = ""
//   - group.<team>.message → To = "",   Team = team
type TeamMessageEvent struct {
	Type      string `json:"type"` // "team.message"
	From      string `json:"from"`
	To        string `json:"to,omitempty"`
	Team      string `json:"team,omitempty"`
	Message   string `json:"message"`
	Summary   string `json:"summary,omitempty"`
	Timestamp string `json:"ts"`
}

// TeamTeamEvent — the team-lens narrative stream.
// Bridges the flat engine topic `team.event`:
// team created / member joined-left / member state / message / item shared /
// state changed / completed-failed, with the correlation envelope so the
// GUI cross-links the team lens with the task tree.
type TeamTeamEvent struct {
	Type      string         `json:"type"` // "team.team_event"
	Action    string         `json:"action"`
	TeamID    string         `json:"teamId"`
	TeamName  string         `json:"teamName,omitempty"`
	State     string         `json:"state,omitempty"`
	MemberID  string         `json:"memberId,omitempty"`
	RunID     string         `json:"runId,omitempty"`       // acting run (correlation)
	ParentRun string         `json:"parentRunId,omitempty"` // correlation
	Detail    map[string]any `json:"detail,omitempty"`
	Timestamp string         `json:"ts"`
}

// TeamCallCompletedEvent — one workflow `call` attempt finished (the
// A-hybrid observability row — calls are NOT children/team members).
// Bridges engine topic: call.<name>.completed
type TeamCallCompletedEvent struct {
	Type       string `json:"type"` // "team.call_completed"
	Node       string `json:"node"`
	URI        string `json:"uri"`
	RunID      string `json:"runId,omitempty"` // owning workflow run (correlation)
	DurationMs int64  `json:"durationMs"`
	Attempt    int    `json:"attempt"`
	Outcome    string `json:"outcome"`
	Error      string `json:"error,omitempty"`
	Timestamp  string `json:"ts"`
}

// TeamLedgerEvent — one shared-state ledger event (Magentic pattern).
// Bridges engine topics ledger.<id>.snapshot + ledger.<id>.complete so the
// GUI Team tab's Ledger lens shows the evolving shared state live (the cold
// view reconstructs the same timeline from the journal).
type TeamLedgerEvent struct {
	Type      string         `json:"type"` // "team.ledger"
	RunID     string         `json:"runId,omitempty"`
	State     map[string]any `json:"state,omitempty"`
	Version   int            `json:"version,omitempty"`
	Complete  bool           `json:"complete,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Timestamp string         `json:"ts"`
}

// ── LLM streaming protocol events (push) ────────────────────────────
//
// A run's UI artifacts reach a chat as its chat panel's events
// (PanelEvent); the renderer registrations below are
// standalone events, and the user-event flowing back to the agent is
// UIEventCmd (client → server command).
//
// See modules/display/streaming-protocol.md.

// UIPatchOp — single patch operation. The `Kind` field discriminates
// the four supported patch flavors. Other fields' meaning depends on
// kind:
//
//   - "rfc6902" : Op (add/remove/replace/move/copy/test) + Path + Value/From
//   - "rfc7396" : MergePatch object replaces the artifact's value
//   - "append"  : Path identifies an array; Value is appended
//   - "replace" : Path is set to Value (idempotent shallow set)
//   - "remove"  : Path is deleted
//
// `Target` is the LLM-friendly addressing alternative to Path: a node
// id picked at mount time. The chat panel bridge (chatpanel) resolves
// Target → Path in the artifact's tree before it applies the op.
// When both Target and Path are set, Path wins (escape hatch for
// deep indexes with no natural id).
type UIPatchOp struct {
	Kind       string          `json:"kind"`                 // "rfc6902" | "rfc7396" | "append" | "replace" | "remove"
	Op         string          `json:"op,omitempty"`         // RFC 6902 op (add/remove/replace/move/copy/test)
	Target     string          `json:"target,omitempty"`     // node-id reference; frontend resolves to Path
	Path       string          `json:"path,omitempty"`       // JSON pointer (RFC 6901)
	Value      json.RawMessage `json:"value,omitempty"`      // for add / replace / append / rfc7396
	From       string          `json:"from,omitempty"`       // for RFC 6902 move/copy
	MergePatch json.RawMessage `json:"mergePatch,omitempty"` // for rfc7396 (alternative to Value)
}

// PanelEvent — one event of the session's chat panel: the
// conversation as its display panel of type chat, each UI artifact a node
// the panel's ops replace by id, carrying the artifact whole. Every chat
// draws a run's artifacts from it. Journaled in the
// session's journal, but for the replaces of an artifact still streaming.
type PanelEvent struct {
	Type      string      `json:"type"` // "panel.event"
	Session   string      `json:"session,omitempty"`
	Event     panel.Event `json:"event"`
	Timestamp string      `json:"ts"`
}

// UIRendererRegistrationEvent — register a custom renderer at runtime.
// Source = "system" for trusted built-ins; "llm" registrations are
// sandboxed (no eval, no external network, restricted DOM).
type UIRendererRegistrationEvent struct {
	Type       string          `json:"type"` // "ui.renderer_registration"
	RendererID string          `json:"rendererId"`
	Source     string          `json:"source"`           // "system" | "llm"
	Config     json.RawMessage `json:"config,omitempty"` // renderer-specific config
	Timestamp  string          `json:"ts"`
}

// UIRendererDeregistrationEvent — remove a previously-registered renderer.
type UIRendererDeregistrationEvent struct {
	Type       string `json:"type"` // "ui.renderer_deregistration"
	RendererID string `json:"rendererId"`
	Timestamp  string `json:"ts"`
}

// StatsEvent — ephemeral session telemetry for the GUI status bar:
// the active model + cumulative token usage. Broadcast (not persisted)
// on session start (model known, zero tokens) and after each LLM
// response (cumulative totals). The frontend computes cache efficiency
// + best-effort cost from these. Cumulative across the whole session.
type StatsEvent struct {
	Type    string `json:"type"` // "stats"
	Session string `json:"session"`
	Model   string `json:"model,omitempty"`
	// Models is the distinct set of models used this session (≥2 means a
	// delegation ran a child on a different model). The status line shows "…"
	// + this list as a tooltip; Model stays the most-recent for back-compat.
	Models              []string `json:"models,omitempty"`
	Provider            string   `json:"provider,omitempty"`
	InputTokens         int      `json:"inputTokens"`
	OutputTokens        int      `json:"outputTokens"`
	CacheReadTokens     int      `json:"cacheReadTokens"`
	CacheCreationTokens int      `json:"cacheCreationTokens"`
	// CostUSD is the cumulative session spend computed on the agent
	// side from the model's configured $/MTok rates (LLMProviders.yaml).
	// Zero when the active model declares no cost. Negative-unset is not
	// used; consumers show it only when CostKnown.
	CostUSD   float64 `json:"costUSD"`
	CostKnown bool    `json:"costKnown"`
	// Context fill: the latest request's input size (incl. cached) vs
	// the model's configured window.
	CurrentInputTokens int `json:"currentInputTokens"`
	ContextWindow      int `json:"contextWindow,omitempty"`
	// Turn count + last-response latency.
	Turns          int   `json:"turns"`
	LastDurationMs int64 `json:"lastDurationMs,omitempty"`
	LastTTFTMs     int64 `json:"lastTtftMs,omitempty"`
}

// ── LLM streaming protocol commands (client → server) ────────────────

// UIEventCmd — user-interaction event from the host's artifact UI back
// to the agent. A submit or cancel of a form the agent asks goes to that
// question's panel input (the terminal room). Other event types
// (change/click/navigate/custom) flow as informational events.
type UIEventCmd struct {
	Type   string     `json:"type"` // "ui_event"
	Params UIEventOpt `json:"params"`
}

type UIEventOpt struct {
	ArtifactID string          `json:"artifactId"`
	EventType  string          `json:"eventType"`       // "submit" | "change" | "click" | "navigate" | "custom"
	Value      json.RawMessage `json:"value,omitempty"` // event payload (form data on submit, target id on click, etc.)
}
