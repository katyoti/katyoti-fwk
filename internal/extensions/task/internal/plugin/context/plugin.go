// SPDX-License-Identifier: Apache-2.0

package context

import (
	gocontext "context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// StrategyDef is the new typed strategy definition for the agent.plugin domain.
var StrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"context", "Task context injection",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "context",
			CreateFunc:  contextFactory,
		}, nil
	},
)

func contextFactory(ctx gocontext.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	pluginCtx := agentapi.PluginContextFrom(ctx)
	if len(pluginCtx) == 0 {
		return nil, nil
	}
	return []agentapi.AgentPlugin{newContextPlugin(pluginCtx)}, nil
}

// contextPlugin contributes task context information to the system message.
type contextPlugin struct {
	context map[string]any
}

func newContextPlugin(ctx map[string]any) *contextPlugin {
	return &contextPlugin{context: ctx}
}

func (p *contextPlugin) Name() string { return "context" }

func (p *contextPlugin) SystemMessage() string {
	if len(p.context) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Task Context\n\n")
	b.WriteString("The following context was provided for this task execution:\n\n")
	for k, v := range p.context {
		switch val := v.(type) {
		case string:
			if k == "input_content" && len(val) > 500 {
				b.WriteString(fmt.Sprintf("- **%s**: (%d chars, see below)\n", k, len(val)))
			} else {
				b.WriteString(fmt.Sprintf("- **%s**: %s\n", k, val))
			}
		default:
			b.WriteString(fmt.Sprintf("- **%s**: %v\n", k, v))
		}
	}
	if content, ok := p.context["input_content"].(string); ok && len(content) > 500 {
		b.WriteString("\n### Input Content\n\n")
		b.WriteString("```\n")
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}
	b.WriteString("\n")
	return b.String()
}

func (p *contextPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *contextPlugin) HandleToolCall(_ gocontext.Context, name string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("unknown tool: %q", name)
}
func (p *contextPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *contextPlugin) Stable() bool                { return true }
