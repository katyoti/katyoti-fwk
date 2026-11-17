// SPDX-License-Identifier: Apache-2.0

package task

import (
	"encoding/json"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// --- Typed tool input/output structs ---

// CompleteInput is the input for the complete tool.
type CompleteInput struct {
	Summary string   `json:"summary" jsonschema:"Brief summary of what was accomplished" required:"true"`
	Files   []string `json:"files,omitempty" jsonschema:"List of files generated (relative paths)"`
}

// DelegateInput is the input for the delegate tool.
type DelegateInput struct {
	TaskName string         `json:"task_name,omitempty" jsonschema:"Name of the task to delegate to (optional if prompt is provided)"`
	Mode     string         `json:"mode,omitempty"      jsonschema:"Delegation mode: worker (sync, default), background (async), fork (context inheritance), isolated (git worktree)"`
	Input    map[string]any `json:"input,omitempty"     jsonschema:"Input parameters for the delegate"`
	Prompt   string         `json:"prompt,omitempty"    jsonschema:"Instructions for the delegate (required for fork modes, or when task_name is omitted)"`
	Finish   string         `json:"finish,omitempty"    jsonschema:"How the child ends — 'self' (it produces a result then completes itself; the default when you delegate a task to GET a product, even via agent_as_human) or 'driven' (an open conversation you steer until you stop it with stop-child). Use 'self' to drive a wizard to completion, 'driven' for an unbounded chat."`
}

// TaskStatusInput is the input for the task-status tool.
type TaskStatusInput struct {
	TaskID string `json:"task_id" jsonschema:"Task ID returned by delegate with an async mode" required:"true"`
}

// --- ToolDef builders using auto-generated schemas ---

// CompleteToolDef returns the ToolDef for the complete tool.
//
// outputSchema, when non-nil, is the task's compiled output contract
// (from the task's output contract file) and REPLACES the generic
// {summary, files} input schema — what the model passes to complete IS
// the task's structured output (journaled with the tool call; the
// Output view and workflows read it from there).
func CompleteToolDef(outputSchema json.RawMessage) agentapi.ToolDef {
	def := agentapi.ToolDef{
		Name:        "complete",
		Description: "Signal that you are done. Call this when all deliverables have been produced.",
		InputSchema: corereflect.MustJSONSchema[CompleteInput](),
		Display:     &agentapi.ToolDisplay{Icon: "✅", Label: "Done", Color: "#C3E88D", Template: "{{.summary}}"},
	}
	if len(outputSchema) > 0 {
		def.Description = "Signal that you are done by producing the task's declared output. Call this with the output fields once all deliverables exist."
		def.InputSchema = outputSchema
	}
	return def
}

// DelegateToolDef returns the ToolDef for the delegate tool.
func DelegateToolDef() agentapi.ToolDef {
	return agentapi.ToolDef{
		Name:        "delegate",
		Description: "Delegate work to a specialist. Provide task_name for a registered task, or prompt for free-form delegation.",
		InputSchema: corereflect.MustJSONSchema[DelegateInput](),
		Display:     &agentapi.ToolDisplay{Icon: "🤖", Label: "Agent", Color: "#F78C6C", Template: "{{.task_name}} [{{.mode}}]"},
	}
}

// TaskStatusToolDef returns the ToolDef for the task-status tool.
func TaskStatusToolDef() agentapi.ToolDef {
	return agentapi.ToolDef{
		Name:        "task-status",
		Description: "Check the status and progress of a background task.",
		InputSchema: corereflect.MustJSONSchema[TaskStatusInput](),
		Display:     &agentapi.ToolDisplay{Icon: "📊", Label: "Status", Color: "#89DDFF", Template: "{{.task_id}}"},
		Polling:     true,
	}
}
