// SPDX-License-Identifier: Apache-2.0

package create_document

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"create-document", "Document creation tool",
	func() (agentapi.PluginFactory, error) {
		tool := NewCreateDocumentTool()
		return &agentapi.FuncFactory{
			FactoryName: "create-document",
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
	init.AddContribute("create-document-tool", "", func(app *application.Application) error {
		return agentapi.ContributePluginStrategy(app, StrategyDef)
	})
}
