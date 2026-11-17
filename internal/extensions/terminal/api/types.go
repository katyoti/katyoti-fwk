// SPDX-License-Identifier: Apache-2.0

// Package api defines the public contract for the Terminal slot.
//
// The Terminal slot exposes the terminal protocol (task execution +
// streamed events + user input commands) on the bus, so a client like
// ktw-gui can drive many tasks against the ktw-agents core of a universe —
// no per-task subprocess spawn.
//
// Wire model:
//
//	Client → Server (request/reply):
//	  ktw-agents:Terminal/start-task    → {runID, sessionID}
//	  ktw-agents:Terminal/cmd           command = raw terminal-protocol command JSON
//	  ktw-agents:Terminal/cancel
//	  ktw-agents:Terminal/list-sessions → every session the core's Room holds
//
//	Server → Client (bus publishes, see the Notifications section below):
//	  …session.<sid>.task.<runID>.event.event  raw terminal-protocol event JSON
//	  …session.<sid>.task.<runID>.event.done   DoneNotification, exactly once per run
//
// Lifecycle: each StartTask creates an in-process Channel transport pair —
// one end is given to ServeTask (Room + Translator + HumanGate +
// TaskService.Run), the other end is forwarded to the bus by the service
// implementation. ServeTask consumes a terminal.TerminalTransport, as in
// every other mode.
//
// Decision:
// (modules/display/decisions/OD-008-how-does-the-gui-run-a-task-in-a-universe-and.cwm.md);
// described in modules/display/terminal/concepts.md §3.
package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
)

// TerminalService is the published KTSP contract for terminal protocol
// over KTSP. Implemented by `local.NewService` in this same package tree.
type TerminalService interface {
	StartTask(ctx context.Context, in StartTaskInput) (StartTaskOutput, error)
	Cmd(ctx context.Context, in CmdInput) (CmdOutput, error)
	Cancel(ctx context.Context, in CancelInput) (CancelOutput, error)
	// ListSessions returns every session the core's Room currently holds
	// (running + recently-finished), across ALL runs on this core — the
	// live "running sessions" registry a client renders to follow what is
	// executing. Sourced from the Room singleton (the session manager
	// service), NOT the on-disk session journals.
	ListSessions(ctx context.Context, in ListSessionsInput) (ListSessionsOutput, error)
}

// TerminalServiceDef declares the TerminalService for KTSP publishing.
var TerminalServiceDef = meta.ServiceDef{
	Name:    "Terminal",
	Short:   "Run tasks + stream terminal protocol events over KTSP",
	Long:    `Wraps the existing terminal protocol (Room/Translator/Session/HumanGate) as a KTSP slot. Clients call StartTask to launch a task in this core, send Cmd to forward user input (cancel_child / send_message / answer_prompt) into the running task, and receive terminal events as Event notifications scoped by runID. Done notification fires exactly once per run on completion. Multiple runs can be multiplexed over a single KTSP session.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"StartTask": meta.MethodLong(
			"Start a task and return its runID",
			`Spawns the task inside this ktw-agents core. The task runs asynchronously; events stream back as Terminal.Event notifications. Completion is signaled by Terminal.Done. Multiple tasks can run concurrently in the same core (multiplexed over the KTSP session).`,
		),
		"Cmd": meta.MethodLong(
			"Forward a terminal-protocol command to a running task",
			`Forwards a raw terminal-protocol command (as JSON) to the running task identified by runID. Used for user input that arrives mid-task: cancel_child, send_message, answer_prompt. The command JSON shape is the existing terminal protocol — no translation here.`,
		),
		"Cancel": meta.MethodLong(
			"Cancel a running task",
			`Graceful cancellation of the named runID. Sends a cancel signal into the task's transport. Equivalent to user clicking 'Stop' in the GUI. Idempotent — no-ops on already-completed runs.`,
		),
		"ListSessions": meta.MethodLong(
			"List the sessions currently held by this core",
			`Returns every session in the core's Room (running + recently-finished), across all runs — the live registry a client renders to follow what is executing. Sourced from the Room (session manager service), not the on-disk journals.`,
		),
	},
}

// ListSessionsInput is the (empty) request for ListSessions — it lists
// every session on the core; scope (tenant/universe) comes from the wire
// headers, like every other Terminal method.
type ListSessionsInput struct{}

// ListSessionsOutput carries the live session snapshot.
type ListSessionsOutput struct {
	Sessions []SessionInfo `json:"sessions" desc:"Every session the core's Room holds (running + finished)" jsonschema:"Live registry snapshot from the session manager service"`
}

// SessionInfo is the wire shape of one session — mirrors
// terminal.Session, kept local so the KTSP contract carries no dependency
// on the terminal protocol package.
type SessionInfo struct {
	ID              string `json:"id"`
	ParentSessionID string `json:"parentSessionId,omitempty"`
	Name            string `json:"name"`
	Status          string `json:"status"` // running | done | error | stopped
	TaskID          string `json:"taskId"`
	ParentTaskID    string `json:"parentTaskId,omitempty"`
	MessageCount    int    `json:"messageCount"`
	StartedAt       string `json:"startedAt"`
}

// ── Notifications (server → client) ──────────────────────────────────
//
// Post bus-split-ktw-gui-hub-cutover, streaming events publish over the
// NATS bus on session-scoped subjects, not as KTSP notifications:
//
//	Event: ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.event
//	  body = raw terminal-protocol event JSON (no envelope — runID is in
//	  the subject path, event type is the trailing `event` token).
//
//	Done:  ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.done
//	  body = DoneNotification (error string only — runID is in the subject).
//
// Subscribers (typically ktw-gui's terminalRouter) attach to
// `…session.<sid>.>` via plain `bus.Subscribe` (NOT QueueSubscribe) so
// multiple GUI instances on the same session each receive every event
// (cowork via NATS pub/sub fan-out).

// DoneNotification is the body shape published on the session-scoped
// `…task.<runID>.event.done` subject. Fired exactly once per StartTask;
// after Done, no more events arrive on this run's subject family.
//
// The RunID field is redundant with the subject path (the runID token
// in `…task.<tid>.event.done` IS the runID). It is retained as a parity
// check + ergonomic debugging aid: subscribers can confirm payload
// alignment without re-parsing the subject. RunID is OPTIONAL in
// serialization — `omitempty` keeps the wire minimal in the common case.
type DoneNotification struct {
	RunID string       `json:"runID,omitempty" desc:"The run that has terminated (parity with subject path)" jsonschema:"Mirrors the runID token in the subject path; safe to ignore when subject parsing is in place"`
	Error string       `json:"error,omitempty"  desc:"Non-empty if the task failed"                       jsonschema:"Empty on success; carries the task error string on failure"`
	Msg   *usermsg.Msg `json:"msg,omitempty" desc:"The failure's message key and params when the error knows one" jsonschema:"A client says the failure in the person's language from it"`
}

// ── I/O types — request/response ────────────────────────────────────

// StartTaskInput collects the parameters needed to start a task.
//
// Mirrors the existing TaskOpts shape used by ServeTask, plus a
// client-supplied label that may identify which chat panel / window
// the run belongs to (purely for logging — runID is the canonical id).
//
// RunID: optional. When the client supplies one, the server uses it as
// the run identifier (eliminates the race window where notifications
// could arrive before the client has registered its receive channel).
// When empty, the server generates one. Clients SHOULD supply their own
// runID for any flow that subscribes to Event notifications immediately
// after StartTask returns.
//
// SessionID: optional. Identifies the bus-side session this task belongs
// to. Event + Done notifications publish on
// `ktw.<tenant>.universe.<u>.session.<sid>.task.<runID>.event.<type>` —
// `sid` is this field. Empty defaults to "default" (single-session-per-
// universe convention used by v1 ktw-gui). Multi-session callers (e.g.
// future ktw-gui with multiple chat panels per universe) supply their
// own session ID so subscribers can scope to one session.
type StartTaskInput struct {
	RunID         string                 `json:"runID,omitempty" desc:"Optional client-supplied runID" jsonschema:"If non-empty, the server uses this verbatim. If empty, the server generates one. Clients pre-register their notification consumer using the same runID."`
	SessionID     string                 `json:"sessionID,omitempty" desc:"Optional client-supplied session ID" jsonschema:"Identifies the bus-side session this task belongs to. Empty defaults to \"default\". Event/Done notifications publish on subjects scoped by this value, so all subscribers wanting this run's events must use the same value."`
	TaskName      string                 `json:"taskName"      desc:"Task to run"             jsonschema:"Name of the task as registered in the TaskService" required:"true"`
	Input         map[string]interface{} `json:"input,omitempty" desc:"Task input"           jsonschema:"Free-form input passed to the task"`
	WorkspaceRoot string                 `json:"workspaceRoot,omitempty" desc:"Workspace root override" jsonschema:"Working directory for the task"`
	DryRun        bool                   `json:"dryRun,omitempty" desc:"Dry-run mode"        jsonschema:"If true, the task runs in dry-run mode (no real side effects)"`
	ClientLabel   string                 `json:"clientLabel,omitempty" desc:"Client-side label for logging" jsonschema:"Identifies which client window/panel started the run; opaque to the server"`

	// InitialMessages provides pre-populated conversation history. When
	// set, the agent loop's first turn includes these messages — used
	// for session resume (ktw-gui's SessionViewer Resume button loads the
	// session's llm.jsonl, extracts the last llm_request's messages, and
	// passes them here). Empty means the agent starts fresh (standard
	// fire-and-forget StartTask behavior).
	//
	// Each entry is the `agent.api.Message` wire shape (`{role, content,
	// toolCalls?, toolCallId?}`). Encoded as raw JSON here to keep the
	// terminal API free of the agent's import. Forwarded to the agent
	// via TaskRunRequest.ConfigOverride["initialMessages"] +
	// ConfigOverride["historyMode"]="full_fork".
	InitialMessages []json.RawMessage `json:"initialMessages,omitempty" desc:"Conversation history (resume)" jsonschema:"Pre-populated agent.api.Message entries. When non-empty, the agent runs with HistoryMode=full_fork."`

	// Context carries the plugin-context envelope passed to the agent
	// loop's factory. Forwarded as `ConfigOverride["context"]` →
	// `AgentRunRequest.Context` → `WithPluginContext(ctx, ...)`, where
	// plugins read it via `agentapi.PluginContextFrom(ctx)`.
	//
	// Canonical envelope keys (extend as new dimensions land):
	//   _target   — CWM object id (the cwm plugin wrapper reads
	//               this to inject the <cwm_object_context>
	//               system-prompt block).
	//
	// Empty map (or nil) = no envelope; plugins fall back to their
	// default behavior. Underscore-prefix is the convention for
	// substrate-reserved keys; non-prefixed keys are caller-defined.
	Context map[string]any `json:"context,omitempty" desc:"Plugin-context envelope (e.g. _target for CWM plugin)" jsonschema:"Forwarded to AgentRunRequest.Context via ConfigOverride[\"context\"]; plugins read via PluginContextFrom(ctx)."`
}

// StartTaskOutput returns the run identity of the freshly-started run.
//
// RunID is the key the client uses to (a) filter incoming events by
// run, (b) send Cmd / Cancel for that run, (c) match the eventual Done
// notification.
//
// SessionID is the journal session id (UUIDv7) — matches the on-disk
// session directory under `<DataDir>/sessions/` and the `sessionId`
// field of every event emitted on the session-scoped bus subjects.
// Use it to subscribe / open the journal browser / fetch the
// structured output without a second round-trip.
type StartTaskOutput struct {
	RunID     string `json:"runID" desc:"Stable runID for this run" jsonschema:"Use this to filter Terminal.Event notifications and to invoke Terminal.Cmd / Terminal.Cancel"`
	SessionID string `json:"sessionID,omitempty" desc:"Journal session id (UUIDv7) — matches the on-disk session directory name"`
}

// CmdInput forwards a raw terminal-protocol command to a running task.
//
// The command JSON is opaque to the KTSP layer — it's the same JSON
// shape ServeTask reads from a stdio transport today (cancel_child,
// send_message, answer_prompt, etc.). The KTSP layer just delivers it
// into the task's input channel.
type CmdInput struct {
	RunID   string          `json:"runID"   desc:"The run to forward the command to"  jsonschema:"From StartTask.Output" required:"true"`
	Command json.RawMessage `json:"command" desc:"Raw terminal-protocol command JSON" jsonschema:"Wire-compatible with the existing NDJSON terminal protocol — no transformation" required:"true"`
}

// CmdOutput is intentionally empty — Cmd is fire-and-forget for the
// caller's perspective; effects are observed via subsequent Event
// notifications.
type CmdOutput struct{}

// CancelInput cancels a running task.
type CancelInput struct {
	RunID  string `json:"runID"          desc:"The run to cancel"          jsonschema:"From StartTask.Output" required:"true"`
	Reason string `json:"reason,omitempty" desc:"Optional human-readable reason" jsonschema:"Surfaced in logs; not visible to the task itself"`
}

// CancelOutput is intentionally empty.
type CancelOutput struct{}
