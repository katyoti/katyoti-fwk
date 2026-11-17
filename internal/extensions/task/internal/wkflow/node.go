// SPDX-License-Identifier: Apache-2.0

package wkflow

// NodeKind identifies the control-flow primitive a Node represents.
type NodeKind string

const (
	// Core primitives (workflow-core)
	NodeDelegate NodeKind = "delegate" // leaf: run a child state machine (absorbs the former "spawn"; implicit `task:` steps map here)
	NodeCall     NodeKind = "call"     // leaf: one synchronous action invocation via ActionService (no child machine)
	NodeSequence NodeKind = "sequence" // run children in order
	NodeReturn   NodeKind = "return"   // terminate workflow with a value

	// Control primitives (workflow-control — stubs for future)
	NodeParallel NodeKind = "parallel" // run children concurrently, wait for all
	NodeRace     NodeKind = "race"     // run children concurrently, first wins
	NodeIf       NodeKind = "if"       // branch on condition
	NodeSwitch   NodeKind = "switch"   // multi-way branch
	NodeLoop     NodeKind = "loop"     // N times
	NodeWhile    NodeKind = "while"    // while condition
	NodeForeach  NodeKind = "foreach"  // iterate over collection
	NodeWait     NodeKind = "wait"     // wait for event or duration
	NodeAssign   NodeKind = "assign"   // write to state.Data
	NodePanel    NodeKind = "panel"    // open, update, ask through or complete a display panel
)

// Node is the generic tree node in the workflow AST.
// Each node has a unique ID and kind. Kind-specific data is stored in
// exactly one of the optional fields (Spawn, Sequence, Return, etc.).
//
// Every node can optionally define OnError for composable error handling.
// When a node fails and has OnError set, the strategy is applied before
// propagating the error up the activation stack.
type Node struct {
	ID   string
	Kind NodeKind

	// OnError defines what happens when this node fails.
	// If nil, errors propagate up the activation stack (default: fail).
	OnError *OnErrorDef

	// Kind-specific — only one group is populated per node.
	Delegate *DelegateNode
	Call     *CallNode
	Sequence *SequenceNode
	Return   *ReturnNode
	Parallel *ParallelNode
	Race     *RaceNode
	If       *IfNode
	Switch   *SwitchNode
	Loop     *LoopNode
	While    *WhileNode
	Foreach  *ForeachNode
	Wait     *WaitNode
	Assign   *AssignNode
	Panel    *PanelNode
}

// OnErrorStrategy defines the built-in error handling strategies.
type OnErrorStrategy string

const (
	// OnErrorFail propagates the error up (default behavior).
	OnErrorFail OnErrorStrategy = "fail"

	// OnErrorContinue skips the failed node and continues execution.
	// The failed node's result is recorded as nil in state.results.
	OnErrorContinue OnErrorStrategy = "continue"

	// OnErrorRetry re-executes the failed node up to MaxRetries times.
	OnErrorRetry OnErrorStrategy = "retry"

	// OnErrorFallback runs an alternative node instead of the failed one.
	OnErrorFallback OnErrorStrategy = "fallback"
)

// OnErrorDef defines error handling for a node.
// Either Strategy is set (for simple cases) or Fallback is set (for fallback node).
type OnErrorDef struct {
	// Strategy is the error handling approach: fail, continue, retry, fallback.
	Strategy OnErrorStrategy `json:"strategy" yaml:"strategy"`

	// MaxRetries is the maximum retry count (only for strategy=retry). Default 1.
	MaxRetries int `json:"max_retries,omitempty" yaml:"max_retries"`

	// BackoffMs, when set, is the wait before the first retry; each retry
	// waits twice the one before, at most MaxBackoffMs, ±20 % — a timer the
	// run waits on as a machine (no slot, saved, kept across a restart). Zero
	// retries at once.
	BackoffMs    int `json:"backoff_ms,omitempty" yaml:"backoff_ms"`
	MaxBackoffMs int `json:"max_backoff_ms,omitempty" yaml:"max_backoff_ms"`

	// Fallback is the alternative node to run (only for strategy=fallback).
	Fallback *Node `json:"fallback,omitempty" yaml:"fallback"`
}

// DelegateNode runs exactly one child state machine. It is THE child-run
// primitive (absorbing the former SpawnNode):
// three addressing forms — inline task definition (ChildKind+Config), a
// registry task name (TaskName), or an action URI (URI) — exactly one is
// populated. Await semantics derive from Mode: foreground modes suspend
// until child.done/failed; background modes return {task_id, run_id}
// immediately and the workflow continues.
type DelegateNode struct {
	// ChildKind is the machine type to spawn (e.g., "agent", "simulator").
	// Populated for the inline form (and for TaskName, which resolves to
	// kind "task" with a _taskName config key).
	ChildKind string

	// ChildName is the logical role name, used as key in state.results
	// and in topic conventions (child.<name>.done).
	ChildName string

	// URI addresses the child as an action URI (<provider>:tasks/<name>).
	// Resolved at runtime through ActionService; must describe an async
	// (task) action — sync actions belong to the `call` primitive.
	URI string

	// TaskName references a named task in the registry (loader maps it to
	// ChildKind "task" + Config["_taskName"]).
	TaskName string

	// Config is the strategy-specific config for the child machine.
	Config map[string]any

	// InputBinds maps input field names to expressions (evaluated at spawn time).
	InputBinds map[string]any

	// Body is prompt text or template text for the child.
	Body string

	// Mode is the child's delegation mode (worker, teammate, fork, etc.). When
	// non-empty, the spawner uses presets.BuildChildOverride(Mode, ParentMode)
	// to inject the mode-derived config (permissions, isolation, history).
	// Mode also decides await semantics (foreground suspend vs background
	// immediate return). Read from the step's `mode:` field at load time.
	Mode string

	// MaxDurationS / MaxBudgetUSD are the v1 per-delegate dimension params
	// (scheduling limits). The full override surface arrives with the
	// shared ConfigOverride contract.
	MaxDurationS int
	MaxBudgetUSD float64
}

// CallNode invokes an action URI as ONE synchronous request/response via
// ActionService — a workflow primitive, NOT a child state machine: no
// lifecycle topics, no Team-tab presence, no dimensions. Emits a typed
// call.completed event per attempt for journal/session-view observability
// (the A-hybrid model).
type CallNode struct {
	// Name keys the result in state.results and labels the call.* events.
	Name string

	// URI is the action to invoke (<provider>:<slot>/<method>). Must
	// describe a SYNC action (Describe().IsAsync == false) — task URIs
	// belong to the `delegate` primitive.
	URI string

	// InputBinds maps input field names to expressions (evaluated at call time).
	InputBinds map[string]any

	// TimeoutMs bounds the blocking invocation (mandatory; loader defaults
	// it when absent and caps it).
	TimeoutMs int
}

// SequenceNode runs children in declared order.
type SequenceNode struct {
	Steps []Node
}

// ReturnNode terminates the workflow with an output value.
type ReturnNode struct {
	// Value is the output — can be a literal or expression string.
	Value any
}

// ParallelNode runs all branches concurrently and waits for all to complete.
type ParallelNode struct {
	Branches []Node
	// OnError: "fail_fast" (default) cancels siblings on first failure.
	// "collect" waits for all, aggregates errors.
	OnBranchError string // "fail_fast" | "collect"
}

// RaceNode runs all branches concurrently. First to SUCCEED wins, others are
// cancelled; a branch failure eliminates that branch and survivors keep racing.
type RaceNode struct {
	Branches []Node

	// TimeoutMs, when > 0, bounds the race: if no branch settles it within
	// the window, all children are cancelled and the race fails (subject to
	// on_error — fallback gives the timeout-default pattern). Note: the
	// inbox timer restarts per waiting period, so an intervening branch
	// FAILURE resets the window; with no intervening events (the primary
	// timeout case) it behaves as an absolute bound.
	TimeoutMs int
}

// IfNode branches on a boolean expression.
type IfNode struct {
	// Condition is an expression string evaluated against the workflow scope.
	Condition string
	Then      Node
	Else      *Node // optional
}

// SwitchNode is a multi-way branch on an expression value.
type SwitchNode struct {
	// On is an expression string whose result is matched against case keys.
	On      string
	Cases   map[string]Node
	Default *Node // optional
}

// LoopNode repeats its body a fixed number of times.
type LoopNode struct {
	Times int
	Body  Node
}

// WhileNode repeats its body while a condition is true.
type WhileNode struct {
	// Condition is evaluated before each iteration.
	Condition     string
	Body          Node
	MaxIterations int // mandatory — no unbounded loops
}

// ForeachNode iterates over a collection.
type ForeachNode struct {
	// Items is an expression that evaluates to a list.
	Items string
	Body  Node
	// Mode: "sequential" (default) or "parallel".
	Mode           string
	MaxConcurrency int // parallel only; 0 = unlimited
}

// PanelNode drives one display panel of the run, named Name in the workflow
// : Action is open, progress, ask or complete, Spec its body
// (resolved against the workflow's state when the step runs). An ask waits
// for the person's input - a durable wait - and keeps it as results.<name>.
type PanelNode struct {
	Name   string
	Action string
	Spec   map[string]any
}

// WaitNode suspends execution for a duration or until an event arrives.
type WaitNode struct {
	// DurationMs is the wait time in milliseconds (0 = no duration wait).
	DurationMs int
	// ForEvent is the topic to wait for (empty = no event wait).
	ForEvent string
	// TimeoutMs applies when waiting for an event (0 = no timeout).
	TimeoutMs int
}

// AssignNode writes a value to the workflow's shared state (Data map).
type AssignNode struct {
	// Target is the key in state.Data to write to (e.g., "ready").
	Target string
	// Value is a literal or expression string.
	Value any
}
