// SPDX-License-Identifier: Apache-2.0

// Package local wires the object-pane service into a binary
// (`<provider>:object-pane/...`). It needs a CWMService and the catalog in the
// binary.
package local

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panecore "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/internal/core"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Extension publishes the ObjectPaneService over KTSP.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.ObjectPaneService](
		func(context.Context) (api.ObjectPaneService, error) { return panecore.New(), nil },
		publish.WithServiceDef(api.ObjectPaneServiceDef),
		publish.WithKTSPContainer(),
	)
}

// Tools are the object-pane tools.
func Tools() []toolsapi.Tool {
	return []toolsapi.Tool{
		toolsapi.NewFunctionTool[api.TransitionOptionsInput, api.TransitionOptionsOutput]("object-transition-options",
			"Where an object may move: per lifecycle, its state and the transitions from it, each judged for who would move (by, default a person), with what the guard's functions answered (gate('…') gives its checklist).",
			func(ctx context.Context, in api.TransitionOptionsInput) (api.TransitionOptionsOutput, error) {
				svc, err := application.UseService[api.ObjectPaneService](ctx)
				if err != nil || svc == nil {
					return api.TransitionOptionsOutput{}, fmt.Errorf("resolve ObjectPaneService: %v", err)
				}
				return svc.TransitionOptions(ctx, in)
			}, toolsapi.WithReadOnly()),
	}
}

const pluginSystemMessage = `Object-pane tools say what may be done with one object: where it may
move, and what stands in the way of each move.`

// pluginStrategy registers the `object-pane` PluginFactory. Bind via
// `plugins: [object-pane]` in a TASK.md.
var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"object-pane", "Where an object may move",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "object-pane",
			CreateFunc: func(context.Context, agentapi.LoopContext, []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolGroupPlugin("object-pane", pluginSystemMessage, Tools())}, nil
			},
		}, nil
	},
)

// AgentPluginExtension contributes the `object-pane` PluginFactory. Only a
// binary with the agent loop (ktw-agents) adds it.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("object-pane", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, pluginStrategy)
		})
	}
}
