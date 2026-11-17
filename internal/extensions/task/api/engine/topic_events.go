// SPDX-License-Identifier: Apache-2.0

package engine

import "time"

// Typed event payloads for topic conventions.
// All events implement EventStamper for auto-stamping by the executor.
// Child lifecycle events (ChildStartedEvent, ChildDoneEvent, ChildFailedEvent)
// are defined in child_events.go.

// ── Agent Messaging ───────────────────────────────────────────────────

// AgentMessageEvent is the payload for to.<name>.message topics.
// Also used by group.<team>.message (with Team field set).
type AgentMessageEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	From      string    `json:"from"`
	To        string    `json:"to"`             // agent name or "*"
	Team      string    `json:"team,omitempty"` // for group broadcasts
	Text      string    `json:"text"`
	Summary   string    `json:"summary,omitempty"`
}

func (e *AgentMessageEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ChildSpawnEvent is emitted by the `delegate` tool just before it asks
// the BackgroundTaskManager (or sync runner) to fork off a child. I2:
// captures everything an operator needs to reconstruct the spawn graph —
// parent identity, child name, mode preset, the serialised override map,
// and the policy names sticking to the parent at spawn time. Distinct
// from the existing `child.<name>.started` lifecycle event because that
// one fires AFTER the child machine instantiates; ChildSpawnEvent fires
// BEFORE, with the parent's full decision context attached.
//
// Captures intent, not content: no message bodies, no LLM transcripts.
type ChildSpawnEvent struct {
	HasCorrelation
	Type          string         `json:"type"`
	Timestamp     time.Time      `json:"timestamp"`
	Emitter       string         `json:"emitter"`
	ParentRunID   string         `json:"parentRunId"`
	ParentName    string         `json:"parentName,omitempty"`
	ChildName     string         `json:"childName"`
	ChildKind     string         `json:"childKind,omitempty"` // task type (agent / workflow / simulator / ...)
	Mode          string         `json:"mode,omitempty"`      // preset name (worker / coordinator / ...)
	Override      map[string]any `json:"override,omitempty"`  // configOverride map (no secrets)
	SpawnPolicies []string       `json:"spawnPolicies,omitempty"`
	// Input is the child's runtime input map — it feeds the session
	// task-tree view ("what ran, with WHICH INPUT"). Journal redaction
	// scrubs secrets at the write chokepoint, so carrying the input is safe.
	Input map[string]any `json:"input,omitempty"`
}

func (e *ChildSpawnEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// CallCompletedEvent is the per-attempt observability payload for a
// workflow `call` node (topic: call.<name>.completed). A call is ONE
// synchronous action invocation, NOT a child machine (A-hybrid model) —
// this event is its entire journal/session-view trail.
// One event per attempt: a node-level on_error retry emits another with
// Attempt incremented.
type CallCompletedEvent struct {
	HasCorrelation
	Type       string    `json:"type"`
	Timestamp  time.Time `json:"timestamp"`
	Emitter    string    `json:"emitter"` // the workflow's runID (executor-stamped)
	Node       string    `json:"node"`    // the call step's name (results key)
	URI        string    `json:"uri"`
	DurationMs int64     `json:"durationMs"`
	Attempt    int       `json:"attempt"` // 1-based
	Outcome    string    `json:"outcome"` // "ok" | "error" | "timeout"
	Error      string    `json:"error,omitempty"`
}

func (e *CallCompletedEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ChildPermissionDeniedEvent is emitted by SpawnPermissionHook or
// WorkspacePathHook whenever a child's tool call is rejected. I3: makes
// permission denials observable in the session journal rather than only
// appearing as tool-call errors in the LLM transcript. Carries the rule
// (which hook fired), the tool, and the offending input fragment (path
// or URL) — never the full tool input, which can contain secrets.
type ChildPermissionDeniedEvent struct {
	HasCorrelation
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	Emitter     string    `json:"emitter"`
	RunID       string    `json:"runId"`
	Hook        string    `json:"hook"` // "spawn_permission" / "workspace_path" / "url_allowlist"
	Tool        string    `json:"tool"`
	Reason      string    `json:"reason"`
	DeniedValue string    `json:"deniedValue,omitempty"` // path / URL / tool kind — bounded payload only
}

func (e *ChildPermissionDeniedEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// MailboxCrossTeamDropEvent is the payload for the audit-trail topic
// emitted whenever MailboxManager.receiveEvent drops an incoming message
// because the sender's team doesn't match the receiver's. Closes R5+I1
// (security T3 follow-up): cross-team drops are no longer silent.
//
// Captures sender + receiver identity + both teams + a free-form reason
// so operators replaying a session can reconstruct who tried to message
// whom across team boundaries. The message text is INTENTIONALLY omitted
// to avoid leaking content into the security audit channel.
type MailboxCrossTeamDropEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	FromTeam  string    `json:"fromTeam,omitempty"`
	ToTeam    string    `json:"toTeam,omitempty"`
	Reason    string    `json:"reason"`
}

func (e *MailboxCrossTeamDropEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// AgentTaskEvent is the payload for to.<name>.task — a task assignment.
type AgentTaskEvent struct {
	HasCorrelation
	Type        string         `json:"type"`
	Timestamp   time.Time      `json:"timestamp"`
	Emitter     string         `json:"emitter"`
	From        string         `json:"from"`
	To          string         `json:"to"`
	TaskID      string         `json:"taskId"`
	Description string         `json:"description"`
	Input       map[string]any `json:"input,omitempty"`
}

func (e *AgentTaskEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// AgentResultEvent is the payload for to.<name>.result — a reply to a task.
type AgentResultEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	TaskID    string    `json:"taskId"`
	Output    any       `json:"output,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func (e *AgentResultEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ── Child Progress ────────────────────────────────────────────────────

// ChildProgressEvent is periodic progress from a running child (opt-in per child).
type ChildProgressEvent struct {
	HasCorrelation
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Emitter   string         `json:"emitter"`
	ChildName string         `json:"childName"`
	RunID     string         `json:"runId"`
	Metrics   map[string]any `json:"metrics,omitempty"` // tokens, tool calls, step count, etc.
	Message   string         `json:"message,omitempty"`
}

func (e *ChildProgressEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ── Workflow Lifecycle ────────────────────────────────────────────────

// WorkflowStartedEvent signals a workflow instance began executing.
type WorkflowStartedEvent struct {
	HasCorrelation
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Emitter   string         `json:"emitter"`
	RunID     string         `json:"runId"`
	Name      string         `json:"name,omitempty"`
	Inputs    map[string]any `json:"inputs,omitempty"`
}

func (e *WorkflowStartedEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// WorkflowDoneEvent signals a workflow completed successfully.
type WorkflowDoneEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	RunID     string    `json:"runId"`
	Output    any       `json:"output,omitempty"`
	Duration  string    `json:"duration,omitempty"`
}

func (e *WorkflowDoneEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// WorkflowFailedEvent signals a workflow failed.
type WorkflowFailedEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	RunID     string    `json:"runId"`
	Error     string    `json:"error"`
}

func (e *WorkflowFailedEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// RunQueueDepthEvent is the periodic ops-quality metric emitted by
// TaskService so dashboards / Grafana / oncall can subscribe to a steady
// pulse of "how full are the run-state maps right now". Counts are
// best-effort snapshots — they're sampled under each map's own lock but
// not coordinated, so a momentary skew of ±1 across the three values is
// expected and acceptable for a metrics gauge.
//
// Per `decisions/tasks-performance-budget.md`: peak goroutines @ 1000
// runs should stay under 5000. The Runs count here is the most relevant
// number — Background and Spawner counts contextualize what's driving
// the depth (foreground workflow run vs. async background fan-out vs.
// workflow-level child spawning).
type RunQueueDepthEvent struct {
	Type              string    `json:"type"`
	Timestamp         time.Time `json:"timestamp"`
	Emitter           string    `json:"emitter"`
	Runs              int       `json:"runs"`              // TaskService.runs
	BackgroundEntries int       `json:"backgroundEntries"` // bgManager.entries
	SpawnerChildren   int       `json:"spawnerChildren"`   // LocalSpawner.children (sampled per workflow)
}

func (e *RunQueueDepthEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ── Structured Protocols (request/response with correlation) ──────────

// PlanApprovalRequest is sent by an agent asking for plan approval.
type PlanApprovalRequest struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ID        string    `json:"id"` // correlation ID
	AgentName string    `json:"agentName"`
	Plan      string    `json:"plan"`
}

func (e *PlanApprovalRequest) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// PlanApprovalResponse is the reply to a plan approval request.
type PlanApprovalResponse struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ID        string    `json:"id"` // correlation ID — matches request
	Approved  bool      `json:"approved"`
	Feedback  string    `json:"feedback,omitempty"`
}

func (e *PlanApprovalResponse) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// PermissionRequest is sent by an agent asking to use a restricted tool.
type PermissionRequest struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Emitter   string         `json:"emitter"`
	ID        string         `json:"id"` // correlation ID
	AgentName string         `json:"agentName"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

func (e *PermissionRequest) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// PermissionResponse is the reply to a permission request.
type PermissionResponse struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ID        string    `json:"id"`
	Decision  string    `json:"decision"` // "allow" | "deny" | "allow-with-rules"
	Rules     string    `json:"rules,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

func (e *PermissionResponse) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ShutdownRequest asks a running agent to shut down cooperatively.
type ShutdownRequest struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ID        string    `json:"id"`
	AgentName string    `json:"agentName"`
	Reason    string    `json:"reason,omitempty"`
	GraceMs   int       `json:"graceMs,omitempty"` // grace period before forced termination
}

func (e *ShutdownRequest) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ShutdownResponse is the agent's reply (accept/reject).
type ShutdownResponse struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ID        string    `json:"id"`
	Accepted  bool      `json:"accepted"`
	Reason    string    `json:"reason,omitempty"`
}

func (e *ShutdownResponse) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ── Ledger Events (Magentic shared-state pattern) ──────────────────────

// LedgerSnapshotEvent is emitted by the ledger machine on every state change.
// Subscribers see the latest full snapshot. Version increments monotonically
// so consumers can detect stale data.
type LedgerSnapshotEvent struct {
	HasCorrelation
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Emitter   string         `json:"emitter"`
	State     map[string]any `json:"state"`
	Version   int            `json:"version"`
}

func (e *LedgerSnapshotEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// LedgerUpdateEvent applies a JSON Merge Patch (RFC 7396) to the ledger state.
// Top-level keys merge; nested objects merge recursively; null deletes a key.
// Last-writer-wins for conflicting keys (events serialize through executor inbox).
type LedgerUpdateEvent struct {
	HasCorrelation
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Emitter   string         `json:"emitter"`
	Patch     map[string]any `json:"patch"`
	// Source is an optional identifier of the agent sending the update (for audit/debug).
	Source string `json:"source,omitempty"`
}

func (e *LedgerUpdateEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// LedgerCompleteEvent terminates the ledger. The optional FinalState overrides
// the current snapshot as the ledger machine's Output. If nil, the current
// snapshot is used.
type LedgerCompleteEvent struct {
	HasCorrelation
	Type       string         `json:"type"`
	Timestamp  time.Time      `json:"timestamp"`
	Emitter    string         `json:"emitter"`
	FinalState map[string]any `json:"finalState,omitempty"`
	Reason     string         `json:"reason,omitempty"`
}

func (e *LedgerCompleteEvent) StampEvent(eventType, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}
