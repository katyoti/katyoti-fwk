// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// Dispatcher routes tool calls from the LLM to owning plugins.
// Each tool name maps to the plugin that declared it via ToolDefs().
//
// In addition to plugin routing, the dispatcher holds a parallel
// `tools` map populated from plugins built by NewToolPlugin /
// NewToolGroupPlugin (via toolsapi.ToolsFromPlugin). The agent loop
// uses LookupTool to fetch the underlying Tool instance for the
// per-call permission gate (Tool.CheckPermissions). Plugins that
// don't wrap typed Tools — MCP, sidecars, future shapes — simply
// don't populate the tools map; the gate then applies only the
// profile's deny rules to those tools.
type Dispatcher struct {
	mu     sync.RWMutex
	owners map[string]api.AgentPlugin // tool name → owning plugin
	defs   map[string]api.ToolDef
	tools  map[string]toolsapi.Tool // tool name + alias → wrapped Tool (nil-safe lookups)
}

// NewDispatcher creates an empty tool dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		owners: make(map[string]api.AgentPlugin),
		defs:   make(map[string]api.ToolDef),
		tools:  make(map[string]toolsapi.Tool),
	}
}

// RegisterPlugin registers all tool definitions from a plugin.
// The plugin becomes the owner and handles dispatch via HandleToolCall.
//
// If the plugin was produced by NewToolPlugin / NewToolGroupPlugin,
// its wrapped Tool instances are also indexed (by name + each alias)
// for the per-tool permission gate. Plugins that aren't wrappers
// (MCP, sidecars, custom AgentPlugin implementations) skip this
// indexing — LookupTool returns nil for their tool names and the
// gate applies only the profile's deny rules to them.
func (d *Dispatcher) RegisterPlugin(p api.AgentPlugin) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, def := range p.ToolDefs() {
		d.owners[def.Name] = p
		d.defs[def.Name] = def
	}

	// Index wrapped Tool instances for the permission gate. Only
	// wrappers from tools/api implement the unexported toolHolder
	// interface; everything else returns nil here.
	for _, tool := range toolsapi.ToolsFromPlugin(p) {
		name := tool.ToolDef().Name
		d.tools[name] = tool
		for _, alias := range tool.Aliases() {
			d.tools[alias] = tool
		}
	}
}

// LookupTool returns the wrapped Tool instance for a given tool name
// (or alias), or nil if no Tool was registered under that name.
//
// Returning nil is meaningful: it tells the agent loop's permission
// gate that this tool came from a non-wrapper plugin (MCP, sidecar,
// etc.): only the active profile's deny rules apply to it, and no
// Tool.CheckPermissions runs.
func (d *Dispatcher) LookupTool(name string) toolsapi.Tool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.tools[name]
}

// Dispatch routes a tool call to the owning plugin's HandleToolCall.
func (d *Dispatcher) Dispatch(ctx context.Context, name string, input json.RawMessage) (*api.ToolCallResult, error) {
	d.mu.RLock()
	owner, ok := d.owners[name]
	d.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
	return owner.HandleToolCall(ctx, name, input)
}

// ToolDefs returns all registered tool definitions (for sending to LLM).
func (d *Dispatcher) ToolDefs() []api.ToolDef {
	d.mu.RLock()
	defer d.mu.RUnlock()

	defs := make([]api.ToolDef, 0, len(d.defs))
	for _, def := range d.defs {
		defs = append(defs, def)
	}
	return defs
}

// Has checks if a tool is registered.
func (d *Dispatcher) Has(name string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.owners[name]
	return ok
}
