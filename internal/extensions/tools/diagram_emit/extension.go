// SPDX-License-Identifier: Apache-2.0

package diagram_emit

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// emitStrategy is the diagram-emit plugin: the tool and its stream plugin,
// which draws the diagram into a document tab while the model writes it.
var emitStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
	"diagram-emit", "Diagram rendering tool",
	func() (agentapi.PluginFactory, error) {
		tool := NewDiagramEmitTool()
		return &agentapi.FuncFactory{
			FactoryName: "diagram-emit",
			CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				streamer := newStreamPlugin()
				streamer.ctx = ctx
				return []agentapi.AgentPlugin{toolsapi.NewToolPlugin(tool), streamer}, nil
			},
		}, nil
	},
)

// Extension contributes the diagram-emit plugin (tool + live drawing) and
// the diagram-read tool.
func Extension(init *application.ApplicationInit) {
	init.AddContribute("diagram_emit-tool", "", func(app *application.Application) error {
		if err := agentapi.ContributePluginStrategy(app, emitStrategy); err != nil {
			return err
		}
		return agentapi.ContributePluginStrategy(app, toolsapi.ToolExtension(NewDiagramReadTool()))
	})
}
