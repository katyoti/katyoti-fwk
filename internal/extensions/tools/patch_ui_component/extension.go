// SPDX-License-Identifier: Apache-2.0

package patch_ui_component

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// StrategyDef wires the patch-ui-component tool. Single tool, no
// stream plugin (synchronous tool — fires once with all ops).
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"patch-ui-component", "Patch UI artifact tool",
	func() (agentapi.PluginFactory, error) {
		tool := NewPatchUIComponentTool()
		return &agentapi.FuncFactory{
			FactoryName: "patch-ui-component",
			CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolPlugin(tool)}, nil
			},
		}, nil
	},
)

func Extension(init *application.ApplicationInit) {
	init.AddContribute("patch-ui-component-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
