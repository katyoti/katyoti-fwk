// SPDX-License-Identifier: Apache-2.0

// Package api defines the ActionService contract — the unified
// dispatcher for every runnable thing in the system (tasks, scripts,
// service methods).
//
// ActionService is a THIN dispatcher: parse the incoming tool URI,
// switch by slot prefix, forward to the appropriate runner. It does
// NOT host its own runners; it consumes TaskService / ScriptService /
// the KTSP sender behind the slot dispatch.
//
// Slot routing v1:
//
//	<provider>:tasks/<name>          → TaskService.Run
//	<provider>:tasks/<a>/<b>/<c>     → TaskService.Run (multi-segment task names)
//	<provider>:scripts/<name>        → ScriptService.Run
//	<provider>:<svc>/<method>        → KTSP method call via Sender.Send
//
// Describe inspects the same routing — returns the action's metadata
// (label / description / icon / input schema URI / async-ness)
// without invoking it. Used by the GUI action bar to render
// schema-driven launch forms before any work is done.
package api

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// ActionServiceDef describes the ActionService for KTSP publishing.
var ActionServiceDef = meta.ServiceDef{
	Name:    "action",
	Short:   "Unified dispatcher for tasks, scripts, and service-method calls",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Describe": meta.Method("Inspect an action URI: label, description, input schema, async flag"),
		"Run":      meta.Method("Run an action URI with the given JSON input"),
	},
}

// ActionService is the dispatcher interface. Both methods take a
// tool URI string + ctx; the implementation parses the URI internally
// and forwards. Returning typed structs (vs json.RawMessage) keeps
// consumers loose from the underlying runners' shapes.
type ActionService interface {
	Describe(ctx context.Context, in DescribeInput) (DescribeOutput, error)
	Run(ctx context.Context, in RunInput) (RunOutput, error)
}

// DescribeInput requests inspection of one action URI.
type DescribeInput struct {
	URI string `json:"uri" jsonschema:"Tool URI to inspect (e.g. ktw-agents:tasks/dossier/refine-scope)"`
}

// DescribeOutput carries the action's launch-time metadata.
type DescribeOutput struct {
	URI          string          `json:"uri"`
	Label        string          `json:"label,omitempty"`
	Description  string          `json:"description,omitempty"`
	IconName     string          `json:"icon_name,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema,omitempty"  jsonschema:"JSON Schema for the input payload, compiled from the action's DSL declaration; inline for now (an InputSchemaURI follow-up will reference a DataType Contribution)"`
	OutputSchema json.RawMessage `json:"output_schema,omitempty" jsonschema:"JSON Schema for the output payload, compiled from the action's DSL declaration; inline"`
	// InputSchemaAST is the input as the canonical DSL AST (a schema with
	// its root), the fields' options computed: what a form renders — nested
	// objects and lists included — where InputSchema is what an LLM or a
	// validator reads. A task's comes from its DSL; a method's is read back
	// from its JSON Schema.
	InputSchemaAST json.RawMessage `json:"input_schema_ast,omitempty" jsonschema:"The input as the canonical DSL AST (name root types) with the fields' options computed; what a form renders"`
	IsAsync        bool            `json:"is_async"                jsonschema:"true for tasks (returns a task_id and runs async), false for sync method calls and scripts (returns a value)"`
	// RequiresHumanGate signals callers that this action MAY prompt
	// the user (via ask_user) during execution. The GUI action bar
	// can warn before launching such actions headlessly. Wired by
	// the user-comms-adapter plugin in US-16; v1 surfaces this as a
	// pure metadata field.
	RequiresHumanGate bool `json:"requires_human_gate,omitempty"`

	// ReadOnly says the action changes nothing: a method that declares
	// itself read-only. Only such an action may serve as a guide recipe's
	// check, and running it writes no invocation record.
	ReadOnly bool `json:"read_only,omitempty" jsonschema:"true when the action only reads (a method declaring itself read-only); running it leaves no invocation record"`

	// Details is the action's LONG form — a task's markdown body, which says
	// what it will actually do. Description is the one-liner a card shows;
	// this is what a reader opens when the one-liner is not enough.
	//
	// It rides on Describe rather than on the action LIST because a list is
	// read for every action at once and this is read for one at a time. The
	// task gallery learned that the expensive way: fetching every body to draw
	// a list cost four seconds (2026-09-25).
	Details string `json:"details,omitempty" jsonschema:"The action's long form — a task's markdown body; read on demand, never in a listing"`
}

// RunInput launches one action URI with a JSON-encoded input payload.
type RunInput struct {
	URI   string          `json:"uri"             jsonschema:"Tool URI to invoke"`
	Input json.RawMessage `json:"input,omitempty" jsonschema:"JSON-encoded input payload matching the action's input schema"`

	// Context is the plugin-context envelope threaded to a TASK action's run,
	// mirroring the GUI's terminal StartTaskInput.Context. It carries reserved
	// `_`-prefixed substrate keys — notably `_target` (the object id) so the
	// cwm plugin renders the `<cwm_object_context>` block for an object action.
	// Ignored by sync (script / method) slots.
	Context map[string]any `json:"context,omitempty" jsonschema:"Plugin-context envelope for task actions, e.g. {\"_target\":\"<object id>\"}"`
}

// RunOutput carries the action's result. Async actions (tasks)
// return only the TaskID; sync actions (method calls, scripts) return
// the value inline. Exactly one of TaskID OR Value is set per call.
type RunOutput struct {
	URI    string          `json:"uri"`
	TaskID string          `json:"task_id,omitempty"   jsonschema:"For async actions (tasks): the task run ID. Empty for sync."`
	Value  json.RawMessage `json:"value,omitempty"     jsonschema:"For sync actions (method calls, scripts): the JSON-encoded result. Empty for async."`
	Panel  string          `json:"panel,omitempty"     jsonschema:"For a method declared long: the id of the display panel its run reports into (with task_id). Empty otherwise."`
}
