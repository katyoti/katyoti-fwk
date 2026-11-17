// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"fmt"
	"sort"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// PluginSet collects plugin instances and assembles the agent's dispatcher, system message, and hooks.
type PluginSet struct {
	plugins []api.AgentPlugin
}

// NewPluginSet creates an empty plugin set.
func NewPluginSet() *PluginSet {
	return &PluginSet{}
}

// Plugins returns the plugin list.
func (r *PluginSet) Plugins() []api.AgentPlugin { return r.plugins }

// SortStableFirst reorders plugins: Stable()=true first, then Stable()=false.
// Within each group, original registration order is preserved.
func (r *PluginSet) SortStableFirst() {
	sort.SliceStable(r.plugins, func(i, j int) bool {
		si := r.plugins[i].Stable()
		sj := r.plugins[j].Stable()
		if si == sj {
			return false // preserve order
		}
		return si // stable before dynamic
	})
}

// Add adds a plugin to the set.
func (r *PluginSet) Add(p api.AgentPlugin) {
	r.plugins = append(r.plugins, p)
}

// BuildSystemMessage concatenates system message contributions from all plugins.
func (r *PluginSet) BuildSystemMessage() string {
	var out string
	for _, p := range r.plugins {
		if s := p.SystemMessage(); s != "" {
			if out != "" {
				out += "\n"
			}
			out += s
		}
	}
	return out
}

// BuildDispatcher creates a Dispatcher populated with all plugin tools.
// Each plugin's ToolDefs() are registered with the plugin as the owner.
func (r *PluginSet) BuildDispatcher() *Dispatcher {
	d := NewDispatcher()
	for _, p := range r.plugins {
		d.RegisterPlugin(p)
	}
	return d
}

// ConnectAll calls Connect on all plugins that implement PluginWithLifecycle.
// Stops on the first error.
func (r *PluginSet) ConnectAll(ctx context.Context) error {
	for _, p := range r.plugins {
		if lc, ok := p.(api.PluginWithLifecycle); ok {
			if err := lc.Connect(ctx); err != nil {
				return fmt.Errorf("plugin %q connect: %w", p.Name(), err)
			}
		}
	}
	return nil
}

// DisconnectAll calls Disconnect on all plugins that implement PluginWithLifecycle.
// Errors are logged but do not stop disconnection of remaining plugins.
func (r *PluginSet) DisconnectAll() {
	for _, p := range r.plugins {
		if lc, ok := p.(api.PluginWithLifecycle); ok {
			lc.Disconnect()
		}
	}
}

// BuildHooks collects all hooks from all plugins into a merged HookChain.
func (r *PluginSet) BuildHooks() *HookChain {
	chain := &HookChain{}
	for _, p := range r.plugins {
		chain.Add(p.Hooks())
	}
	return chain
}

// HookChain holds merged hook functions from all plugins in registration order.
type HookChain struct {
	OnTurnStart    []func(api.LoopContext, int) error
	OnPreAPICall   []func(api.LoopContext, []api.Message) error
	OnModelStream  []func(api.LoopContext, api.StreamChunk) error
	OnPostAPICall  []func(api.LoopContext, api.Message) error
	OnPreToolExec  []func(api.LoopContext, string, map[string]any) (*api.ToolDecision, error)
	OnPostToolExec []func(api.LoopContext, []api.ToolResult) error
	OnDecision     []func(api.LoopContext, *api.Decision) error
	OnTurnEnd      []func(api.LoopContext, int) error
	OnError        []func(api.LoopContext, *api.LoopError) error
	OnAbort        []func(api.LoopContext) error
	OnResult       []func(api.LoopContext, *api.LoopResult) error
}

// Add appends non-nil hooks from a PluginHooks to the chain.
func (c *HookChain) Add(h api.PluginHooks) {
	if h.OnTurnStart != nil {
		c.OnTurnStart = append(c.OnTurnStart, h.OnTurnStart)
	}
	if h.OnPreAPICall != nil {
		c.OnPreAPICall = append(c.OnPreAPICall, h.OnPreAPICall)
	}
	if h.OnModelStream != nil {
		c.OnModelStream = append(c.OnModelStream, h.OnModelStream)
	}
	if h.OnPostAPICall != nil {
		c.OnPostAPICall = append(c.OnPostAPICall, h.OnPostAPICall)
	}
	if h.OnPreToolExec != nil {
		c.OnPreToolExec = append(c.OnPreToolExec, h.OnPreToolExec)
	}
	if h.OnPostToolExec != nil {
		c.OnPostToolExec = append(c.OnPostToolExec, h.OnPostToolExec)
	}
	if h.OnDecision != nil {
		c.OnDecision = append(c.OnDecision, h.OnDecision)
	}
	if h.OnTurnEnd != nil {
		c.OnTurnEnd = append(c.OnTurnEnd, h.OnTurnEnd)
	}
	if h.OnError != nil {
		c.OnError = append(c.OnError, h.OnError)
	}
	if h.OnAbort != nil {
		c.OnAbort = append(c.OnAbort, h.OnAbort)
	}
	if h.OnResult != nil {
		c.OnResult = append(c.OnResult, h.OnResult)
	}
}
