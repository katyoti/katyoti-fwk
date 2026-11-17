// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// --- Typed tool inputs/outputs ---

// ScriptRunInput is the input for the scripts-run agent tool.
type ScriptRunInput struct {
	Name   string            `json:"name"           jsonschema:"Script URI in provider:name format (e.g., ktw-docs:elk-layout)" required:"true"`
	Params map[string]string `json:"params,omitempty" jsonschema:"Named parameters passed as SCRIPT_PARAM_* env vars"`
	Args   []string          `json:"args,omitempty"   jsonschema:"Positional arguments"`
}

// ScriptRunOutput is the output of the scripts-run agent tool.
type ScriptRunOutput struct {
	Stdout   string `json:"stdout"           jsonschema:"Script stdout"`
	Stderr   string `json:"stderr,omitempty" jsonschema:"Script stderr"`
	ExitCode int    `json:"exitCode"         jsonschema:"Exit code (0 = success)"`
}

// ScriptListInput is the input for the scripts-list agent tool.
type ScriptListInput struct{}

// ScriptListOutput is the output of the scripts-list agent tool.
type ScriptListOutput struct {
	Scripts []ScriptEntry `json:"scripts" jsonschema:"Available scripts"`
}

// ScriptEntry is a single script in the list.
type ScriptEntry struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Description string `json:"description"`
}

// --- Agent plugin ---

// AgentPluginExtension returns an Extension that registers the "scripts"
// agent plugin strategy. Add this to binaries that include the agent loop.
func AgentPluginExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("scripts", ".plugin", func(app *application.Application) error {
			return agentapi.ContributePluginStrategy(app, scriptsStrategyDef)
		})
	}
}

var scriptsStrategyDef = strategies.FuncStrategy[agentapi.PluginFactory](
	"scripts", "Script execution tools",
	func() (agentapi.PluginFactory, error) {
		return &agentapi.FuncFactory{
			FactoryName: "scripts",
			CreateFunc:  scriptsPluginFactory,
		}, nil
	},
)

func scriptsPluginFactory(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	svc, err := application.UseService[api.ScriptService](ctx)
	if err != nil {
		return nil, nil // no ScriptService — skip
	}
	return []agentapi.AgentPlugin{newScriptsPlugin(svc)}, nil
}

type scriptsPlugin struct {
	tools []toolsapi.Tool
}

func newScriptsPlugin(svc api.ScriptService) *scriptsPlugin {
	return &scriptsPlugin{
		tools: []toolsapi.Tool{
			toolsapi.NewFunctionTool(
				"scripts-run",
				"Execute a script by provider:name URI.",
				func(ctx context.Context, in ScriptRunInput) (ScriptRunOutput, error) {
					out, err := svc.Run(ctx, api.ScriptRunInput{
						Name:   in.Name,
						Params: in.Params,
						Args:   in.Args,
					})
					if err != nil {
						return ScriptRunOutput{}, err
					}
					return ScriptRunOutput{
						Stdout:   out.Stdout,
						Stderr:   out.Stderr,
						ExitCode: out.ExitCode,
					}, nil
				},
				toolsapi.WithMutating(),
				toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "⚡", Label: "Script", Color: "#F78C6C", Template: "{{.name}}"}),
			),
			toolsapi.NewFunctionTool(
				"scripts-list",
				"List all available scripts.",
				func(ctx context.Context, _ ScriptListInput) (ScriptListOutput, error) {
					out, err := svc.List(ctx)
					if err != nil {
						return ScriptListOutput{}, err
					}
					var entries []ScriptEntry
					for _, s := range out.Scripts {
						entries = append(entries, ScriptEntry{
							Name:        s.Name,
							Provider:    s.Provider,
							Description: s.Description,
						})
					}
					return ScriptListOutput{Scripts: entries}, nil
				},
				toolsapi.WithReadOnly(),
			),
		},
	}
}

func (p *scriptsPlugin) Name() string                { return "scripts" }
func (p *scriptsPlugin) SystemMessage() string       { return "" }
func (p *scriptsPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *scriptsPlugin) Stable() bool                { return true }

func (p *scriptsPlugin) ToolDefs() []agentapi.ToolDef {
	defs := make([]agentapi.ToolDef, len(p.tools))
	for i, t := range p.tools {
		defs[i] = t.ToolDef()
	}
	return defs
}

func (p *scriptsPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	for _, t := range p.tools {
		if t.ToolDef().Name == name {
			return t.ToolCall(ctx, input)
		}
	}
	return nil, nil
}
