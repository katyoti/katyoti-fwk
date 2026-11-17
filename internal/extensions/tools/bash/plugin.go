// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// StrategyDef is the typed strategy definition for the agent.plugin domain.
// The bash tool runs as a standalone plugin (not part of workspace).
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"bash", "Shell command execution",
	func() (agentapi.PluginFactory, error) {
		tool := NewBashTool()
		return &agentapi.FuncFactory{
			FactoryName: "bash",
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				// Start in the project the universe serves, if not already set.
				if GetCwd() == "" {
					InitCwd(application.ProjectRoot())
				}
				return []agentapi.AgentPlugin{toolsapi.NewToolPlugin(tool)}, nil
			},
		}, nil
	},
)
