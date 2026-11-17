// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// toolHolder is the unexported interface implemented by every plugin
// produced by NewToolPlugin / NewToolGroupPlugin. It exposes the
// wrapped Tool instance(s) so the agent runtime can fetch them for
// permission gating before dispatching a tool call.
//
// The interface is unexported by design — it's plumbing between the
// kernel and the wrapper code in this package, not part of any public
// surface that downstream extensions need to implement. External
// callers should use the exported ToolsFromPlugin helper instead.
type toolHolder interface {
	tools() []Tool
}

// ToolsFromPlugin returns the wrapped Tool instances from a plugin
// produced by NewToolPlugin or NewToolGroupPlugin, or nil if the
// plugin is not one of those wrappers (e.g. an MCP plugin or a
// sidecar plugin that doesn't expose typed Tool values).
//
// The agent runtime's dispatcher calls this at plugin-registration
// time and indexes the returned Tools by name + alias for the
// per-tool permission gate. Plugins that return nil don't get
// per-tool gating — only the active profile's deny rules apply to
// their tools — so a plugin holding typed Tools must be built with
// NewToolPlugin / NewToolGroupPlugin.
//
// This is the only public entry point for accessing a wrapper's
// underlying Tool instances; the toolHolder interface itself stays
// unexported.
func ToolsFromPlugin(p agentapi.AgentPlugin) []Tool {
	holder, ok := p.(toolHolder)
	if !ok {
		return nil
	}
	return holder.tools()
}

// --- Function tool plugin (single Tool wrapped as one plugin) ---

// toolPlugin adapts a single Tool into an AgentPlugin.
type toolPlugin struct {
	tool Tool
}

func (p *toolPlugin) Name() string                 { return p.tool.ToolDef().Name }
func (p *toolPlugin) SystemMessage() string        { return p.tool.Prompt() }
func (p *toolPlugin) ToolDefs() []agentapi.ToolDef { return []agentapi.ToolDef{p.tool.ToolDef()} }
func (p *toolPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p *toolPlugin) Stable() bool                 { return true }

func (p *toolPlugin) HandleToolCall(ctx context.Context, _ string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	return p.tool.ToolCall(ctx, input)
}

// tools satisfies the unexported toolHolder interface used by
// ToolsFromPlugin so the agent runtime can fetch the underlying Tool
// instance(s) for permission gating. See ToolsFromPlugin for the
// public entry point.
func (p *toolPlugin) tools() []Tool { return []Tool{p.tool} }

// NewToolPlugin creates an AgentPlugin from a single Tool. The resulting
// plugin contributes one ToolDef and routes every HandleToolCall to the
// underlying tool regardless of the requested name (the loop already
// resolved this plugin by tool name).
func NewToolPlugin(tool Tool) agentapi.AgentPlugin {
	return &toolPlugin{tool: tool}
}

// NewToolPluginFactory wraps a single Tool into a PluginFactory suitable
// for use inside a strategies.FuncStrategy[agentapi.PluginFactory].
//
// Typical usage in an extension package:
//
//	var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
//	    "grep", "Content search",
//	    func() (agentapi.PluginFactory, error) {
//	        return api.NewToolPluginFactory(grepTool), nil
//	    },
//	)
//
// Function tools have no per-session state, so the factory ignores ctx,
// loop, and refs and returns the same single-tool plugin every time.
func NewToolPluginFactory(tool Tool) agentapi.PluginFactory {
	def := tool.ToolDef()
	return &agentapi.FuncFactory{
		FactoryName: def.Name,
		CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
			return []agentapi.AgentPlugin{NewToolPlugin(tool)}, nil
		},
	}
}

// ToolExtension creates a PluginStrategyDef that wraps a single Tool into
// a plugin factory. The strategy is named after the tool.
// Register the result via agentapi.ContributePluginStrategy(app, def).
//
// This is the convenience path for function tools that don't need any
// extension-side wiring beyond the strategy registration.
func ToolExtension(tool Tool) agentapi.PluginStrategyDef {
	def := tool.ToolDef()
	return strategies.FuncStrategy[agentapi.PluginFactory](
		def.Name, def.Description,
		func() (agentapi.PluginFactory, error) {
			return NewToolPluginFactory(tool), nil
		},
	)
}

// --- Tool group plugin (multiple Tools wrapped as one composite plugin) ---

// toolGroupPlugin adapts a slice of Tools (typically produced by NewToolGroup)
// into a single AgentPlugin that routes HandleToolCall by tool name.
//
// The slice field is named `toolList` (not `tools`) to leave the
// `tools` identifier free for the unexported toolHolder method below.
type toolGroupPlugin struct {
	name          string
	systemMessage string
	toolList      []Tool
	defs          []agentapi.ToolDef
	byName        map[string]Tool
}

// NewToolGroupPlugin builds a composite AgentPlugin from a slice of Tools.
// The plugin advertises every tool's ToolDef and routes HandleToolCall by
// name to the matching tool.
//
// Typical usage inside a plugin strategy that resolves a group's impl
// from DI before constructing the per-method tools:
//
//	var pluginStrategy = strategies.FuncStrategy[agentapi.PluginFactory](
//	    "lsp", "Language Server tools",
//	    func() (agentapi.PluginFactory, error) {
//	        return &agentapi.FuncFactory{
//	            FactoryName: "lsp",
//	            CreateFunc: func(ctx context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
//	                impl := application.UseService[Tools](ctx)
//	                if c, ok := impl.(api.ConnectableToolGroup); ok {
//	                    if err := c.Open(ctx); err != nil { return nil, err }
//	                }
//	                methods := api.NewToolGroup[Tools]("lsp", "Language Server.", impl, api.WithReadOnly())
//	                return []agentapi.AgentPlugin{api.NewToolGroupPlugin("lsp", "", methods)}, nil
//	            },
//	        }, nil
//	    },
//	)
//
// systemMessage may be empty — if non-empty, it is contributed as the
// composite plugin's system-prompt block (in addition to each individual
// tool's Prompt()). For most tool groups the per-tool prompts are enough
// and systemMessage stays empty.
func NewToolGroupPlugin(name, systemMessage string, tools []Tool) agentapi.AgentPlugin {
	defs := make([]agentapi.ToolDef, len(tools))
	byName := make(map[string]Tool, len(tools))
	for i, t := range tools {
		def := t.ToolDef()
		defs[i] = def
		byName[def.Name] = t
		// Also register aliases so HandleToolCall finds them.
		for _, alias := range t.Aliases() {
			byName[alias] = t
		}
	}
	return &toolGroupPlugin{
		name:          name,
		systemMessage: systemMessage,
		toolList:      tools,
		defs:          defs,
		byName:        byName,
	}
}

func (p *toolGroupPlugin) Name() string                 { return p.name }
func (p *toolGroupPlugin) SystemMessage() string        { return p.systemMessage }
func (p *toolGroupPlugin) ToolDefs() []agentapi.ToolDef { return p.defs }
func (p *toolGroupPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p *toolGroupPlugin) Stable() bool                 { return true }

func (p *toolGroupPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	tool, ok := p.byName[name]
	if !ok {
		return nil, fmt.Errorf("tool group %q: unknown tool %q", p.name, name)
	}
	return tool.ToolCall(ctx, input)
}

// tools satisfies the unexported toolHolder interface used by
// ToolsFromPlugin so the agent runtime can fetch the underlying Tool
// instances for permission gating. A defensive copy keeps callers
// from mutating the plugin's internal slice.
func (p *toolGroupPlugin) tools() []Tool {
	out := make([]Tool, len(p.toolList))
	copy(out, p.toolList)
	return out
}
