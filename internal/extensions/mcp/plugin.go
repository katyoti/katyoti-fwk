// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// mcpStrategyType implements Strategy for the MCP plugin strategy.
type mcpStrategyType struct{}

func (s *mcpStrategyType) NewTypeConfig() (any, func(context.Context) (agentapi.PluginFactory, error)) {
	cfg := &MCPFactoryConfig{}
	return cfg, func(_ context.Context) (agentapi.PluginFactory, error) {
		servers := cfg.Servers
		name := cfg.Name
		if name == "" {
			name = "mcp"
		}
		return &agentapi.FuncFactory{
			FactoryName: name,
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				if len(servers) == 0 {
					return nil, nil
				}
				return []agentapi.AgentPlugin{newMCPPlugin(servers)}, nil
			},
		}, nil
	}
}

func (s *mcpStrategyType) CreateTypeDef(name string, raw agentapi.PluginFactory) strategies.TypeDef[agentapi.PluginFactory] {
	return strategies.TypeDef[agentapi.PluginFactory]{
		DefMeta:  strategies.DefMeta{Name: name, Label: "MCP: " + name},
		Strategy: "mcp",
		Impl:     raw,
	}
}

// StrategyDef is the new typed strategy definition for the agentapi.plugin domain.
// Uses typed MCPFactoryConfig instead of map[string]any.
var StrategyDef = agentapi.PluginStrategyDef{
	DefMeta:  strategies.DefMeta{Name: "mcp", Label: "MCP server bridges"},
	Strategy: &mcpStrategyType{},
}

// MCPFactoryConfig is the typed config for the MCP strategy.
type MCPFactoryConfig struct {
	Name    string            `json:"name" yaml:"name"`
	Servers []MCPServerConfig `json:"servers" yaml:"servers"`
}

// mcpPlugin integrates MCP servers as an AgentPlugin with lifecycle management.
// On Connect, it creates a Manager, connects to all configured servers,
// discovers their tools, and exposes them through the plugin's ToolDefs() method.
type mcpPlugin struct {
	servers  []MCPServerConfig
	mgr      *Manager
	defs     []agentapi.ToolDef
	handlers map[string]func(context.Context, json.RawMessage) (string, error)
}

func newMCPPlugin(servers []MCPServerConfig) *mcpPlugin {
	return &mcpPlugin{
		servers:  servers,
		handlers: make(map[string]func(context.Context, json.RawMessage) (string, error)),
	}
}

func (p *mcpPlugin) Name() string                { return "mcp" }
func (p *mcpPlugin) SystemMessage() string       { return "" }
func (p *mcpPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *mcpPlugin) Stable() bool                { return false }

func (p *mcpPlugin) ToolDefs() []agentapi.ToolDef {
	return p.defs
}

func (p *mcpPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	handler, ok := p.handlers[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %q", name)
	}
	s, err := handler(ctx, input)
	if err != nil {
		return nil, err
	}
	return agentapi.StringResult(s), nil
}

// Connect establishes connections to all configured MCP servers and discovers tools.
func (p *mcpPlugin) Connect(ctx context.Context) error {
	p.mgr = NewManager()
	if err := p.mgr.ConnectAll(ctx, p.servers); err != nil {
		return err
	}
	registerMCPTools(p, p.mgr)
	return nil
}

// Disconnect closes all MCP server connections.
func (p *mcpPlugin) Disconnect() error {
	if p.mgr != nil {
		p.mgr.Close()
		p.mgr = nil
	}
	p.defs = nil
	p.handlers = make(map[string]func(context.Context, json.RawMessage) (string, error))
	return nil
}
