// SPDX-License-Identifier: Apache-2.0

package display_ui_component

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// StrategyDef wires the display-ui-component tool + its companion stream
// plugin. Per the create-document pattern: ONE factory creates BOTH
// plugins so they share the same agent loop instance.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"display-ui-component", "Display structured UI tool",
	func() (agentapi.PluginFactory, error) {
		tool := NewDisplayUIComponentTool()
		return &agentapi.FuncFactory{
			FactoryName: "display-ui-component",
			CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				streamer := newStreamPlugin()
				streamer.ctx = ctx
				return []agentapi.AgentPlugin{
					toolsapi.NewToolPlugin(tool),
					streamer,
				}, nil
			},
		}, nil
	},
)

func Extension(init *application.ApplicationInit) {
	init.AddContribute("display-ui-component-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
