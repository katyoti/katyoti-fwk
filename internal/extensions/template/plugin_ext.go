// SPDX-License-Identifier: Apache-2.0

package template

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

// RenderTemplateInput is the typed input for the render-template tool.
type RenderTemplateInput struct {
	Template string         `json:"template" jsonschema:"Jinja2-like template string" required:"true"`
	Data     map[string]any `json:"data,omitempty" jsonschema:"Variables to pass to the template"`
}

// PluginStrategyDef is the new typed strategy definition for the agent.plugin domain.
var PluginStrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"template", "Template rendering tool",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "template",
			CreateFunc:  templatePluginFactory,
		}, nil
	},
)

func templatePluginFactory(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	return []agentapi.AgentPlugin{newTemplatePlugin()}, nil
}

// templatePlugin contributes the render-template tool.
type templatePlugin struct{}

func newTemplatePlugin() *templatePlugin { return &templatePlugin{} }

func (p *templatePlugin) Name() string { return "template" }

// SystemMessage is empty: the render-template tool's own description carries
// its usage. (No unconditional prompt block — the tool is self-describing.)
func (p *templatePlugin) SystemMessage() string { return "" }

func (p *templatePlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *templatePlugin) Stable() bool                { return true }

func (p *templatePlugin) ToolDefs() []agentapi.ToolDef {
	return []agentapi.ToolDef{
		{
			Name:        "render-template",
			Description: "Render a Jinja2-like template with the provided data variables. Returns the rendered text.",
			InputSchema: corereflect.MustJSONSchema[RenderTemplateInput](),
			Display:     &agentapi.ToolDisplay{Icon: "🔧", Label: "Tmpl", Color: "#FFCB6B"},
		},
	}
}

func (p *templatePlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	switch name {
	case "render-template":
		s, err := renderTemplate(ctx, input)
		if err != nil {
			return nil, err
		}
		return agentapi.StringResult(s), nil
	default:
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
}

func renderTemplate(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Template string         `json:"template"`
		Data     map[string]any `json:"data"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return "", fmt.Errorf("parse input: %w", err)
	}
	if params.Data == nil {
		params.Data = make(map[string]any)
	}

	tpl, err := gonja.FromString(params.Template)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	gonjaCtx := exec.NewContext(params.Data)

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, gonjaCtx); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
