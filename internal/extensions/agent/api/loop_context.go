// SPDX-License-Identifier: Apache-2.0

package api

// LoopContext provides shared-state accessors for plugins during an agent run.
// Passed to PluginFactory.CreatePlugins() and to every hook invocation.
// Plugins capture this at creation time and use it throughout the run.
//
// Thread-safety: all methods are safe for concurrent access.
// Implementations must protect shared state with appropriate synchronization.
type LoopContext interface {
	// GetMeta returns a metadata value by key, or nil if absent.
	GetMeta(key string) any

	// SetMeta stores a metadata value. Overwrites any previous value for key.
	SetMeta(key string, value any)

	// Messages returns a snapshot of the current conversation history.
	Messages() []Message

	// PushMessage appends a message to the conversation history.
	PushMessage(msg Message)

	// ReplaceMessages atomically replaces the full conversation history.
	// Used by compaction plugins to swap in a shorter context window.
	ReplaceMessages(msgs []Message)

	// AbortSignal returns a channel that is closed when the loop is aborted
	// (e.g. Ctrl+C, context cancellation). Plugins should select on this
	// during long-running operations.
	AbortSignal() <-chan struct{}

	// --- API Call Configuration (per-call overrides, cleared after each API call) ---

	// SetModelOverride overrides the model for the next API call.
	// Used by ModelFallback (switch model on error) and ModelInvocation (fast mode).
	SetModelOverride(modelID string)

	// SetMaxTokens overrides max output tokens for the next API call.
	// Used by OutputRecovery (escalate on truncation).
	SetMaxTokens(n int)

	// SetThinkingBudget overrides thinking token budget for the next API call.
	// Used by ModelInvocation (thinking mode control).
	SetThinkingBudget(tokens int)

	// --- Read-only accessors ---

	// ToolDefs returns the current merged tool definitions.
	// Used by Permission (classify tool risk) and OrphanedPermission (validate tool exists).
	ToolDefs() []ToolDef

	// Turn returns the current turn number.
	Turn() int

	// TotalUsage returns cumulative token usage across all turns so far.
	TotalUsage() Usage

	// Mode returns the run's resolved delegation mode (worker, chat,
	// coordinator, …; empty = worker). The dimension-decoded accessor:
	// plugins and the loop ask it instead of peeking at the request
	// struct.
	Mode() string

	// IsChat reports whether the run is an interactive chat session
	// (Mode() == "chat").
	IsChat() bool

	// RunID returns this run's ID (used by the delegate plugin as the
	// parent_run_id for spawned children). Empty for runs whose ID
	// hasn't been stamped yet.
	RunID() string

	// InheritedPlugins returns the parent's plugin refs to seed a
	// prompt/fork child with, or nil. The dimension-decoded accessor
	// for the inherited-plugins meta — task-side plugins read it here
	// instead of a raw meta key.
	InheritedPlugins() []any

	// PermissionProfile returns the run's active kernel permission profile
	// name (empty = the configured default). The delegate plugin reads it to
	// propagate the parent's profile onto the children it spawns.
	PermissionProfile() string
}

// LoopIntrospector is the narrow, read-only view of the agent loop that
// task-side plugins (delegate, context injection) are allowed to see.
// Depending on it instead of the full LoopContext — and instead of raw
// GetMeta("<key>") access — keeps the loop the owner of its meta-key
// strings, so an agent-side rename surfaces as a compile error in the
// task extension rather than a silent nil read.
type LoopIntrospector interface {
	// RunID is the parent_run_id for spawned children.
	RunID() string
	// Mode is this run's resolved delegation mode (drives child
	// spawn-policy escalation, e.g. coordinator-restricts-children).
	Mode() string
	// Messages snapshots the conversation (fork seeds the child from it).
	Messages() []Message
	// InheritedPlugins are the parent's plugin refs to pass to a
	// prompt/fork child, or nil.
	InheritedPlugins() []any

	// PermissionProfile is the run's active kernel permission profile name
	// (empty = configured default). Propagated onto delegated children.
	PermissionProfile() string
}

// ToolDecision is returned by OnPreToolExec to approve or deny a tool call.
type ToolDecision struct {
	Decision ToolDecisionType
	Reason   string // reason for denial (included in error tool_result)
}

// ToolDecisionType indicates whether a tool call is allowed or denied.
type ToolDecisionType int

const (
	// Allow permits the tool call to proceed.
	Allow ToolDecisionType = iota
	// Deny blocks the tool call and produces an error tool_result.
	Deny
)

// ToolResult represents the outcome of a single tool execution.
type ToolResult struct {
	ToolUseID string
	ToolName  string
	Content   string
	IsError   bool
}

// Decision is passed to OnDecision hooks.
// Plugins set Continue = true and optionally inject messages to force another turn.
type Decision struct {
	// Continue forces another turn when set to true by any plugin.
	Continue bool

	// Messages to inject before the next turn (appended to history).
	Messages []Message
}

// LoopError wraps an error with phase context for OnError hooks.
type LoopError struct {
	// Phase identifies where the error occurred: "api_call" or "tool_exec".
	Phase string

	// Err is the underlying error.
	Err error

	// Recovered is set to true by a recovery plugin to retry the failed operation.
	Recovered bool
}

func (e *LoopError) Error() string {
	return e.Err.Error()
}

func (e *LoopError) Unwrap() error {
	return e.Err
}

// StreamChunk represents one chunk from the streaming model response.
type StreamChunk struct {
	Type         string // "text_delta", "tool_use_start", "tool_use_delta", "usage", "done"
	Delta        string
	ToolName     string
	ToolID       string
	ToolInput    string
	InputTokens  int
	OutputTokens int
	StopReason   string
}
