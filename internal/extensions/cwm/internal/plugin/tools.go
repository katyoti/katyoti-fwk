// SPDX-License-Identifier: Apache-2.0

// Package plugin builds CWM's agent tools: the object tools (tools_object.go —
// reads, writes, object files and named files) and the edge tools (tools.go),
// grouped in the one composite AgentPlugin the `cwm` PluginFactory strategy
// produces. Tools resolve api.CWMService from context at call time, so the
// factory has no state.
//
// Wiring: cwm/local's AgentPluginExtension() contributes Strategy; only
// binaries that run the agent loop add it.
package plugin

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// cwmTool builds a CWM agent tool whose handler resolves the
// CWMService from context and forwards the typed call.
func cwmTool[I, O any](
	name, desc string,
	call func(api.CWMService, context.Context, I) (O, error),
	opts ...toolsapi.ToolOption,
) toolsapi.Tool {
	return toolsapi.NewFunctionTool[I, O](
		name, desc,
		func(ctx context.Context, in I) (O, error) {
			var zero O
			svc, err := application.UseService[api.CWMService](ctx)
			if err != nil {
				return zero, fmt.Errorf("resolve CWMService: %w", err)
			}
			if svc == nil {
				return zero, fmt.Errorf("resolve CWMService: nil service")
			}
			return call(svc, ctx, in)
		},
		opts...,
	)
}

// CWMTools returns the CWM agent tools: the object tools, then the edge
// tools. Verifying and linting are the verification plugin's (object-verify,
// object-lint).
func CWMTools() []toolsapi.Tool {
	return append(objectTools(), graphTools()...)
}

// graphTools are the tools over an object's edges.
// The slice ordering is stable: reads first, writes after.
func graphTools() []toolsapi.Tool {
	return []toolsapi.Tool{
		// --- reads ---
		cwmTool("cwm-edge-list",
			"List typed edges. Pass object_id to scope; kind / direction filter further.",
			func(s api.CWMService, ctx context.Context, in api.EdgeListInput) (api.EdgeListOutput, error) {
				return s.EdgeList(ctx, in)
			}, toolsapi.WithReadOnly()),

		// --- writes ---
		cwmTool("cwm-edge-create",
			"Add a typed edge from one object to another (the source object's edges list). Idempotent.",
			func(s api.CWMService, ctx context.Context, in api.EdgeCreateInput) (api.EdgeCreateOutput, error) {
				return s.EdgeCreate(ctx, in)
			}),

		cwmTool("cwm-edge-delete",
			"Remove a typed edge. Removing an edge that is not there is refused (not found).",
			func(s api.CWMService, ctx context.Context, in api.EdgeDeleteInput) (api.EdgeDeleteOutput, error) {
				return s.EdgeDelete(ctx, in)
			}, toolsapi.WithDestructive()),
	}
}

const cwmPluginSystemMessage = `CWM tools manage the workspace knowledge graph: objects, their
named files (an object's memory is its working and notes files) and their
edges. Find with cwm-object-search before reading.`

// Strategy registers a PluginFactory under the strategy
// name "cwm". Bind via `plugins: [cwm]` in TASK.md (or as a code
// default in a persona's plugin list).
//
// What a chat on an object receives is the object-context plugin's, which the
// agent factory attaches to every run with a target object.
var Strategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"cwm", "Contexia Workspace Manager tools",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "cwm",
			CreateFunc:  cwmFactoryCreate,
		}, nil
	},
)

// cwmFactoryCreate is the factory function called by the resolver
// once per agent session.
func cwmFactoryCreate(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("cwm", cwmPluginSystemMessage, CWMTools())}, nil
}
