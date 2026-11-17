// SPDX-License-Identifier: Apache-2.0

package ask_user_data

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// StrategyDef is the typed strategy definition for the agent.plugin domain.
// Named "ask-user-data" — OPT-IN: a task adds it to its `plugins:` list; it
// is deliberately NOT part of the standard plugin set.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"ask-user-data", "Schema-driven form/wizard user input",
	func() (agentapi.PluginFactory, error) {
		tool := NewAskUserDataTool()
		return &agentapi.FuncFactory{
			FactoryName: "ask-user-data",
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{toolsapi.NewToolPlugin(tool)}, nil
			},
		}, nil
	},
)

// Extension contributes the ask-user-data tool as an opt-in agent plugin
// strategy. Any binary that wires it gains the `ask-user-data` plugin name;
// tasks opt in via their `plugins:` list.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("ask-user-data-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
