// SPDX-License-Identifier: Apache-2.0

package api

import "encoding/json"

// TaskExecRequest is the separated request passed to MachineFactory.CreateMachine.
// It cleanly separates the four concerns that were previously mixed in a single
// map[string]any:
//
//   - Config: strategy-specific parameters from the TASK.md config: block
//   - Input: caller-provided runtime parameters, described by the task's input contract (not validated)
//   - Body: markdown content after frontmatter (mapped to a config field by strategy)
//   - Runtime: framework-injected context (workspace, parent, session)
//
// Factories that understand TaskExecRequest type-assert to it inside CreateMachine.
// Legacy factories that expect map[string]any still work via the fallback path
// in TaskService.Run.
type TaskExecRequest struct {
	// Config holds strategy-specific parameters decoded from the TASK.md
	// config: block. Each strategy defines its own config struct; this field
	// carries the decoded value (typed struct pointer or map[string]any).
	Config any `json:"config,omitempty"`

	// Input holds caller-provided runtime parameters. Always map[string]any;
	// the task's input contract describes it, nothing validates it before
	// dispatch (declared, not enforced).
	Input map[string]any `json:"input,omitempty"`

	// Body is the markdown content after the TASK.md frontmatter.
	// The strategy's BodyFieldName() declares which config field this maps to
	// (e.g., agent → "prompt", template → "template"). The framework injects
	// body into config under that key before calling CreateMachine.
	Body string `json:"body,omitempty"`

	// Runtime carries framework-injected context that is neither config nor input.
	Runtime RuntimeContext `json:"runtime"`

	// ConfigOverride carries per-spawn dimension overrides (SpawnPermissions,
	// WorkspacePaths, HistoryMode, IsolationMode, MaxBudgetUSD) computed by
	// the Spawner from parent SpawnPolicies via presets.BuildChildOverride.
	// The agent factory merges this into AgentRunRequest. Map shape mirrors
	// TaskRunRequest.ConfigOverride (which feeds the same merge for delegate-
	// spawn paths). UC-053/UC-054: this field is the structural close of
	// sec-1 / threat-T1 — workflow-spawned children now go through the same
	// policy machinery as delegate-spawned ones.
	ConfigOverride map[string]any `json:"configOverride,omitempty"`
}

// RuntimeContext holds framework-provided execution context.
type RuntimeContext struct {
	// WorkspaceRoot is the workspace directory for file operations.
	WorkspaceRoot string `json:"workspaceRoot,omitempty"`

	// ParentRunID identifies the parent run that spawned this run.
	// Empty for top-level runs.
	ParentRunID string `json:"parentRunId,omitempty"`

	// SessionID identifies the session this run belongs to.
	SessionID string `json:"sessionId,omitempty"`

	// DryRunMode is propagated from TaskRunRequest so the workflow factory
	// can wire a dry-run-aware LocalSpawner. See TaskRunRequest.DryRunMode.
	DryRunMode bool `json:"dryRunMode,omitempty"`

	// CurrentNode is the CWM context node this run is operating on, when
	// the task's Kind is `node`. Opaque map (the framework doesn't import
	// CWM types). Strategies and the task body read it however they like —
	// agent surfaces it as `node.*` template variables; custom strategies
	// can type-assert as needed. nil = no node (global task or no caller-
	// supplied node).
	CurrentNode map[string]any `json:"currentNode,omitempty"`

	// OutputSchema is the task's resolved output contract (TASK.md
	// output contract file > strategy default), as the data-DSL document.
	// The agent factory forwards it to the plugin context so the
	// `complete` tool can advertise the REAL output schema to the LLM —
	// the structured fields the task promises are what the model must
	// produce when completing.
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
}
