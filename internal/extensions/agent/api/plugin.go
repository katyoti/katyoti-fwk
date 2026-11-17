// SPDX-License-Identifier: Apache-2.0

// Package api is the single public interface for the agent extension.
// External consumers (plugin authors, sidecars, CLI commands) import only this package.
package api

import (
	"context"
	"encoding/json"
)

// AgentPlugin is a composable unit for the agent loop — a PLUGIN.
//
// Not a "capability". That word belongs to CWM, where a capability is a field-group
// an object composes together with the panels, actions and checks that come with it.
// A CWM capability CONTRIBUTES plugins to a session scoped to an object that carries
// it (modules/cwm/concepts.md §4, "the sixth dimension"); it is never one, and this
// type is never called by its name. Decided 2026-09-24 when the two vocabularies met.
// A plugin contributes system messages, tools, and hooks.
// No Init() — initialization happens in PluginFactory.CreatePlugins()
// which receives LoopContext for shared state access.
type AgentPlugin interface {
	// Name returns a unique identifier for logging and debugging.
	Name() string

	// SystemMessage returns text to append to the system prompt.
	// Called at the start of each turn (dynamic — may change across turns).
	// Return empty string if this plugin has no system message contribution.
	SystemMessage() string

	// ToolDefs returns tool definitions contributed by this plugin.
	// Called at the start of each turn (dynamic — tools can appear/disappear).
	// Returns only schemas (pure data) — dispatch goes through HandleToolCall.
	ToolDefs() []ToolDef

	// HandleToolCall dispatches a tool call to this plugin.
	// The loop routes by tool name to the owning plugin (the one that declared the def).
	// Local plugins switch on name and call internal logic.
	// Remote plugins forward via RPC/KTSP.
	HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*ToolCallResult, error)

	// Hooks returns hook registrations for this plugin.
	// Called once during loop initialization; results are merged across all plugins.
	// Nil hook fields are skipped (zero overhead).
	Hooks() PluginHooks

	// Stable returns true if this plugin's SystemMessage() and Tools() rarely
	// change across turns. Stable plugins are ordered before dynamic plugins
	// in the assembled prompt — this maximizes LLM cache hits.
	Stable() bool
}

// PluginWithLifecycle extends AgentPlugin with connection lifecycle.
// Plugins that hold external connections (e.g. MCP servers) implement this
// so the plugin set can manage their lifecycle uniformly.
type PluginWithLifecycle interface {
	AgentPlugin
	Connect(ctx context.Context) error
	Disconnect() error
}

// PluginHooks contains one optional function field per lifecycle hook point.
// A nil field means the plugin does not participate in that hook.
type PluginHooks struct {
	// H1: OnTurnStart fires at the beginning of each loop iteration,
	// before the system prompt is assembled.
	OnTurnStart func(ctx LoopContext, turnNumber int) error

	// H2: OnPreAPICall fires after the prompt is assembled but before
	// the model is called. Plugins may inspect or mutate messages.
	OnPreAPICall func(ctx LoopContext, messages []Message) error

	// H3: OnModelStream fires for each chunk received from the streaming
	// model response. Useful for progress reporting or logging.
	OnModelStream func(ctx LoopContext, chunk StreamChunk) error

	// H4: OnPostAPICall fires after the full model response is received
	// (all content blocks, usage, stop_reason). Fires before tool execution.
	OnPostAPICall func(ctx LoopContext, response Message) error

	// H5: OnPreToolExec fires before each tool is executed.
	// Setting Decision = Deny skips the tool and returns an error tool_result.
	// Short-circuits: remaining plugins for this hook are not called on Deny.
	OnPreToolExec func(ctx LoopContext, toolName string, input map[string]any) (*ToolDecision, error)

	// H6: OnPostToolExec fires after all tools in a turn have completed.
	// Receives the list of tool results (including errors and denied tools).
	OnPostToolExec func(ctx LoopContext, results []ToolResult) error

	// H7: OnDecision fires after tool execution (or after the model response
	// if there are no tools). Plugins may set Continue = true and inject
	// messages to force another turn.
	OnDecision func(ctx LoopContext, decision *Decision) error

	// H8: OnTurnEnd fires at the end of each loop iteration, after the
	// continuation decision is made but before the next iteration begins.
	OnTurnEnd func(ctx LoopContext, turnNumber int) error

	// H9: OnError fires when the loop encounters an error (API call failure,
	// tool execution error). Setting Recovered = true causes the loop to
	// retry the failed operation instead of terminating.
	OnError func(ctx LoopContext, err *LoopError) error

	// H10: OnAbort fires when the loop is cancelled (Ctrl+C, context
	// cancellation). Plugins may perform cleanup.
	OnAbort func(ctx LoopContext) error

	// H11: OnResult fires just before the loop returns its final LoopResult.
	// Plugins may enrich the result metadata.
	OnResult func(ctx LoopContext, result *LoopResult) error
}
