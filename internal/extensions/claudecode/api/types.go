// SPDX-License-Identifier: Apache-2.0

// Package api is the public contract of the Claude Code driver: KaTyoti starts
// Claude Code workers — the claude CLI spoken to over stdio, as Anthropic's
// Agent SDKs do — on the person's own subscription, and follows and answers
// them. The protocol is modules/claude/docs/claude-code-protocol.md.
package api

import (
	"encoding/json"
	"time"
)

// State is where a worker stands in its lifecycle.
type State string

const (
	// StateStarting: spawned, the initialize handshake not answered yet.
	StateStarting State = "starting"
	// StateRunning: a turn is running.
	StateRunning State = "running"
	// StateAwaitingPermission: Claude Code asks a permission; the worker waits
	// for an answer (no timeout on either side).
	StateAwaitingPermission State = "awaiting-permission"
	// StateIdle: no turn running; the worker waits for the next prompt.
	StateIdle State = "idle"
	// StateInterrupting: an interrupt was sent; the turn is ending.
	StateInterrupting State = "interrupting"
	// StateFinished: the worker ended cleanly (stdin closed, exit 0).
	StateFinished State = "finished"
	// StateCrashed: the worker ended otherwise — an exit without a result, a
	// malformed frame, a refused start (the subscription guard).
	StateCrashed State = "crashed"
)

// Ended reports whether a worker in this state is over.
func (s State) Ended() bool { return s == StateFinished || s == StateCrashed }

// PermissionMode is the --permission-mode a worker starts in.
const (
	PermissionModeDefault     = "default"
	PermissionModeAcceptEdits = "acceptEdits"
	PermissionModePlan        = "plan"
	PermissionModeDontAsk     = "dontAsk"
	PermissionModeAuto        = "auto"
)

// StartInput starts a worker.
type StartInput struct {
	Prompt string `json:"prompt" jsonschema:"description=The first user message,required"`
	Cwd    string `json:"cwd,omitempty" jsonschema:"description=The working directory; empty: the current one"`
	Name   string `json:"name,omitempty" jsonschema:"description=The session's display name"`
	Model  string `json:"model,omitempty" jsonschema:"description=The model; empty: Claude Code's default"`
	// PermissionMode: empty takes the configured default.
	PermissionMode     string   `json:"permission_mode,omitempty" jsonschema:"description=default or acceptEdits or plan or dontAsk or auto; empty: the configured default"`
	AllowedTools       []string `json:"allowed_tools,omitempty" jsonschema:"description=Tools approved without asking (permission rule syntax)"`
	DisallowedTools    []string `json:"disallowed_tools,omitempty" jsonschema:"description=Tools refused"`
	AppendSystemPrompt string   `json:"append_system_prompt,omitempty" jsonschema:"description=Text appended to Claude Code's own system prompt"`
	MaxTurns           int      `json:"max_turns,omitempty" jsonschema:"description=The most agentic turns per prompt; 0: no limit"`
	MaxBudgetUSD       float64  `json:"max_budget_usd,omitempty" jsonschema:"description=The most a worker may spend; 0: no limit"`
	// Resume continues an existing Claude Code session (its id); Fork with it
	// continues under a new id.
	Resume string `json:"resume,omitempty" jsonschema:"description=A Claude Code session id to continue"`
	Fork   bool   `json:"fork,omitempty" jsonschema:"description=With resume: continue under a new session id"`
	// RemoteControl opens the worker to claude.ai/code and the Claude app: the
	// person watches and drives it there too. RemoteControlName names it.
	RemoteControl     bool   `json:"remote_control,omitempty" jsonschema:"description=Open the worker to claude.ai/code and the Claude app (Remote Control)"`
	RemoteControlName string `json:"remote_control_name,omitempty" jsonschema:"description=The name shown in claude.ai/code; empty: the worker's name"`
	// SessionID chooses the Claude Code session id of a new session (a UUID).
	SessionID string `json:"session_id,omitempty" jsonschema:"description=The Claude Code session id of a new session (a UUID); empty: Claude Code chooses"`
}

// WorkerInput names a worker.
type WorkerInput struct {
	ID string `json:"id" jsonschema:"description=The worker,required"`
}

// SendInput sends the next user message.
type SendInput struct {
	ID     string `json:"id" jsonschema:"description=The worker,required"`
	Prompt string `json:"prompt" jsonschema:"description=The user message,required"`
}

// Behavior answers a permission.
const (
	BehaviorAllow = "allow"
	BehaviorDeny  = "deny"
)

// Scope of an allow answer.
const (
	// ScopeOnce allows this tool call only.
	ScopeOnce = "once"
	// ScopeSession adds the suggested rules for the rest of the Claude Code
	// session (destination session).
	ScopeSession = "session"
	// ScopeProject adds the suggested rules to the project's local settings
	// (destination localSettings) — only when a person picks it.
	ScopeProject = "project"
)

// AnswerInput answers the permission a worker waits on.
type AnswerInput struct {
	ID        string `json:"id" jsonschema:"description=The worker,required"`
	RequestID string `json:"request_id" jsonschema:"description=The permission request answered,required"`
	Behavior  string `json:"behavior" jsonschema:"description=allow or deny,required"`
	Scope     string `json:"scope,omitempty" jsonschema:"description=For allow: once or session or project; empty: once"`
	Message   string `json:"message,omitempty" jsonschema:"description=For deny: why - Claude reads it"`
	Interrupt bool   `json:"interrupt,omitempty" jsonschema:"description=For deny: also end the turn"`
}

// ListInput filters the workers listed.
type ListInput struct {
	All bool `json:"all,omitempty" jsonschema:"description=Ended workers too"`
}

// ListOutput is the workers this process holds.
type ListOutput struct {
	Workers []Worker `json:"workers"`
}

// PermissionRequest is a permission Claude Code asks (can_use_tool).
type PermissionRequest struct {
	RequestID   string            `json:"request_id"`
	ToolName    string            `json:"tool_name"`
	ToolUseID   string            `json:"tool_use_id,omitempty"`
	Input       json.RawMessage   `json:"input,omitempty"`
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Reason      string            `json:"reason,omitempty"`
	BlockedPath string            `json:"blocked_path,omitempty"`
	Suggestions []json.RawMessage `json:"suggestions,omitempty"`
	At          time.Time         `json:"at"`
}

// Account is who Claude Code runs as, from the initialize answer.
type Account struct {
	Email            string `json:"email,omitempty"`
	Organization     string `json:"organization,omitempty"`
	SubscriptionType string `json:"subscription_type,omitempty"`
	TokenSource      string `json:"token_source,omitempty"`
	APIKeySource     string `json:"api_key_source,omitempty"`
	APIProvider      string `json:"api_provider,omitempty"`
}

// Worker is one Claude Code process this KaTyoti holds.
type Worker struct {
	ID              string             `json:"id"`
	Name            string             `json:"name,omitempty"`
	ClaudeSessionID string             `json:"claude_session_id,omitempty"`
	State           State              `json:"state"`
	Cwd             string             `json:"cwd,omitempty"`
	Model           string             `json:"model,omitempty"`
	ClaudeVersion   string             `json:"claude_version,omitempty"`
	Account         *Account           `json:"account,omitempty"`
	Pending         *PermissionRequest `json:"pending,omitempty"`
	Turns           int                `json:"turns"`
	CostUSD         float64            `json:"cost_usd"`
	LastResult      string             `json:"last_result,omitempty"`
	LastError       string             `json:"last_error,omitempty"`
	RateLimit       string             `json:"rate_limit,omitempty"`
	// RemoteURL is the worker's page on claude.ai/code when it was started
	// with Remote Control; RemoteState its bridge (ready, connected, ...).
	RemoteURL   string     `json:"remote_url,omitempty"`
	RemoteState string     `json:"remote_state,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
}

// EventKind is what an Event says.
type EventKind string

const (
	EventState      EventKind = "state"
	EventText       EventKind = "text"
	EventToolUse    EventKind = "tool-use"
	EventToolResult EventKind = "tool-result"
	EventPermission EventKind = "permission"
	EventResult     EventKind = "result"
	EventRateLimit  EventKind = "rate-limit"
	EventStderr     EventKind = "stderr"
	// EventRemote: the Remote Control bridge changed (Text: its state, or the
	// session URL when it opened).
	EventRemote EventKind = "remote"
	// EventFrame carries a frame the driver does not interpret, as it came.
	EventFrame EventKind = "frame"
)

// Event is one thing that happened to a worker, in order.
type Event struct {
	Seq        int64              `json:"seq"`
	At         time.Time          `json:"at"`
	Kind       EventKind          `json:"kind"`
	State      State              `json:"state,omitempty"`
	Text       string             `json:"text,omitempty"`
	Tool       string             `json:"tool,omitempty"`
	Input      json.RawMessage    `json:"input,omitempty"`
	IsError    bool               `json:"is_error,omitempty"`
	CostUSD    float64            `json:"cost_usd,omitempty"`
	Permission *PermissionRequest `json:"permission,omitempty"`
	Frame      json.RawMessage    `json:"frame,omitempty"`
}
