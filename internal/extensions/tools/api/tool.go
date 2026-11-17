// SPDX-License-Identifier: Apache-2.0

// Package api defines the agent tool API: the Tool interface that every
// agent-callable tool must satisfy, the FunctionTool / ToolGroup builders,
// and shared types (PermissionResult, InterruptBehavior, etc.).
//
// See decision (modules/tools/decisions/OD-001-what-shapes-does-an-agent-tool-take-and-through.cwm.md).
//
// Two tool shapes:
//
//  1. Function tool — a single typed handler.
//     api.NewFunctionTool[Input, Output](name, desc, handler, opts...)
//
//  2. Tool group — a Go interface whose methods all match the handler shape.
//     api.NewToolGroup[Tools](name, desc, impl, opts...)
//
// Each tool extension lives in its own package under
// `internal/extensions/tools/<tool_name>/` and exposes:
//
//	func Extension(init *application.ApplicationInit)
//
// which contributes a plugin strategy via agentapi.ContributePluginStrategy
// and (optionally) declares infrastructure via publish.DeclareInfra or
// publish.PublishService.
package api

import (
	"context"
	"encoding/json"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Tool is the unified interface every agent tool must satisfy.
//
// Function tools (NewFunctionTool) and group methods (NewToolGroup) both
// implement Tool. The agent loop sees a flat []Tool and does not need to
// know which shape the tool came from.
//
// All methods have safe defaults provided by FunctionTool / the group adapter,
// so tool authors only override what they care about via With* options.
//
// See spec FR-01..FR-11 and §7 Component Interface.
type Tool interface {
	// --- Identity ---

	// ToolDef returns the tool's name, description, and JSON input schema.
	// Used by the agent loop to advertise the tool to the LLM.
	ToolDef() agentapi.ToolDef

	// Aliases returns alternative names for backward-compatible lookup.
	// Default: nil.
	Aliases() []string

	// IsEnabled returns true if the tool is currently active.
	// The agent loop hides disabled tools from the LLM.
	// Default: true.
	IsEnabled() bool

	// Describe returns a human-readable description of THIS specific call,
	// used in permission dialogs and progress UI. Receives the raw JSON input.
	// Default: "<name> <truncated input>".
	Describe(input json.RawMessage) string

	// SearchHint returns a short keyword phrase used by the deferred-loading
	// search mechanism (ToolSearch). Required when ShouldDefer is true.
	// Default: "".
	SearchHint() string

	// --- Execution ---

	// ToolCall executes the tool with the given input.
	// The agent loop guarantees CheckPermissions has already returned Allow
	// before this is invoked.
	ToolCall(ctx context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error)

	// IsReadOnly returns true if the tool makes no modifications to the
	// file system or external state. Used by the loop to auto-approve
	// permission for read-only tools.
	// Default: false (fail-closed per spec FR-01).
	IsReadOnly() bool

	// IsConcurrencySafe returns true if the tool can be called in parallel
	// with other concurrency-safe tools. Implies no shared mutable state
	// touched by the call.
	// Default: false (fail-closed).
	IsConcurrencySafe() bool

	// IsDestructive returns true if the tool performs an irreversible action
	// (deletion, force-push, kill, etc.). The default permission policy
	// escalates the prompt wording for destructive tools.
	// Default: false.
	IsDestructive() bool

	// InterruptBehavior controls what happens to a running tool when the
	// agent loop's parent context is cancelled.
	// Default: InterruptBlock.
	InterruptBehavior() InterruptBehavior

	// MaxResultSize returns the maximum byte size of this tool's result data
	// before the agent loop keeps it in a file and gives the model a preview
	// (capToolResult in the agent runtime).
	// 0 means unlimited.
	// Default: MaxResultSizeDefault (256 KB).
	MaxResultSize() int

	// ShouldDefer returns true if the tool's schema should NOT be loaded at
	// startup; the LLM discovers it via ToolSearch.
	// Default: false.
	ShouldDefer() bool

	// --- Permission ---

	// CheckPermissions decides whether the tool may execute. The agent loop
	// MUST call this before ToolCall.
	//
	// Returning an empty PermissionResult{} (zero value) means PermissionAsk —
	// fail-closed per spec FR-01.
	CheckPermissions(ctx context.Context, input json.RawMessage) PermissionResult

	// --- Prompt ---

	// Prompt returns the tool's contribution to the agent system message.
	// Return "" to contribute nothing.
	Prompt() string
}
