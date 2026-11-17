// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// StrategyDef is the typed strategy definition for the agent.plugin domain.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"ask-user", "User interaction tool",
	func() (agentapi.PluginFactory, error) {
		tool := NewAskUserTool()
		return &agentapi.FuncFactory{
			FactoryName: "ask-user",
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return askPlugins(tool), nil
			},
		}, nil
	},
)

// askPlugins are what an agent asks with: ask-user, and request-validation
// beside it - offered to every agent, since the screens list
// the human tasks it asks.
func askPlugins(ask toolsapi.Tool) []agentapi.AgentPlugin {
	return []agentapi.AgentPlugin{toolsapi.NewToolPlugin(ask), toolsapi.NewToolPlugin(NewRequestValidationTool())}
}

// Extension contributes the ask_user tool as a standalone agent plugin strategy.
// Replaces the legacy embedded ask_user in humangate/plugin_ext.go.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("ask-user-tool", "", func(app *application.Application) error {
		if err := agentapi.ContributePluginStrategy(app, StrategyDef); err != nil {
			return err
		}
		// request-validation: a validation, never a conversation.
		return agentapi.ContributePluginStrategy(app, RequestValidationStrategyDef)
	})
}
