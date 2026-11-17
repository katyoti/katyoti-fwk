// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// gatesTool builds a tool whose handler resolves the GatesService from
// context and forwards the typed call.
func gatesTool[I, O any](name, desc string, call func(api.GatesService, context.Context, I) (O, error), opts ...toolsapi.ToolOption) toolsapi.Tool {
	return toolsapi.NewFunctionTool[I, O](name, desc,
		func(ctx context.Context, in I) (O, error) {
			var zero O
			svc, err := application.UseService[api.GatesService](ctx)
			if err != nil || svc == nil {
				return zero, fmt.Errorf("resolve GatesService: %v", err)
			}
			return call(svc, ctx, in)
		}, opts...)
}

// Tools are the gate tools an agent gets: it reads a checklist, or the gates
// that apply to an object. It never records nor withdraws a confirmation —
// that is a person's (the Lifecycle panel, the inbox): an agent claiming
// `by.kind: person` is not a person.
func Tools() []toolsapi.Tool {
	return []toolsapi.Tool{
		gatesTool("gate-check",
			"An object's checklist for a named gate (ready-to-build, …): each requirement met, unmet or unproven, why, and how it is resolved.",
			func(s api.GatesService, ctx context.Context, in api.CheckInput) (api.CheckOutput, error) {
				return s.Check(ctx, in)
			}, toolsapi.WithReadOnly()),
		gatesTool("gate-list",
			"The named gates that apply to an object (by its type), each with its checklist counted: passes, how many requirements are met, unmet and unproven, and each requirement's status.",
			func(s api.GatesService, ctx context.Context, in api.ListInput) (api.ListOutput, error) {
				return s.List(ctx, in)
			}, toolsapi.WithReadOnly()),
	}
}

const pluginSystemMessage = `Gate tools read an object's checklist for a named gate (a definition of
done), or every gate that applies to it. A requirement a person must confirm is
confirmed (or the confirmation withdrawn) by that person, in the object's
Lifecycle panel or through the inbox — never by an agent.`

// pluginStrategy registers the `gates` PluginFactory. Bind via
// `plugins: [gates]` in a TASK.md.
var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"gates", "Check named gates on workspace objects",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "gates",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("gates", pluginSystemMessage, Tools())}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `gates` PluginFactory. Only a binary
// with the agent loop (ktw-agents) adds it.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("gates", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, pluginStrategy)
		})
	}
}
