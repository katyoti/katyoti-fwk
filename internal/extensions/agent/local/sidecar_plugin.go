// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// SenderFactory returns a KTSP Sender for a given sidecar (container) name.
type SenderFactory func(containerName string) (ktsp.Sender, error)

// NewSidecarPluginFactory creates a sidecar PluginFactory from manifest + slot info.
func NewSidecarPluginFactory(name string, manifest appcore.EndpointManifest, slot ktsp.SlotInfo, factory SenderFactory) agentapi.PluginFactory {
	return &sidecarPluginType{
		name:          name,
		manifest:      manifest,
		slot:          slot,
		senderFactory: factory,
	}
}

type sidecarPluginType struct {
	name          string
	manifest      appcore.EndpointManifest
	slot          ktsp.SlotInfo
	senderFactory SenderFactory
}

func (t *sidecarPluginType) Name() string { return t.name }

func (t *sidecarPluginType) CreatePlugins(ctx context.Context, loop agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
	return []agentapi.AgentPlugin{
		newSidecarPlugin(t.name, t.manifest, t.slot, t.senderFactory),
	}, nil
}

type sidecarPlugin struct {
	name          string
	manifest      appcore.EndpointManifest
	slot          ktsp.SlotInfo
	senderFactory SenderFactory
}

func newSidecarPlugin(name string, manifest appcore.EndpointManifest, slot ktsp.SlotInfo, factory SenderFactory) *sidecarPlugin {
	return &sidecarPlugin{
		name:          name,
		manifest:      manifest,
		slot:          slot,
		senderFactory: factory,
	}
}

func (p *sidecarPlugin) Name() string { return p.name }

func (p *sidecarPlugin) SystemMessage() string {
	if p.slot.Description != "" {
		return p.slot.Description
	}
	return ""
}

func (p *sidecarPlugin) ToolDefs() []agentapi.ToolDef {
	var defs []agentapi.ToolDef
	for _, method := range p.methods() {
		if method.Tags[ktsp.TagMethodMCP] == "false" {
			continue
		}
		toolName := method.Name
		if customName, ok := method.Tags[ktsp.TagMethodMCPName]; ok && customName != "" {
			toolName = customName
		}
		defs = append(defs, agentapi.ToolDef{
			Name:        toolName,
			Description: sidecarDescriptionFor(toolName, &method),
			InputSchema: sidecarSchemaFor(&method),
		})
	}
	return defs
}

func (p *sidecarPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	containerName := p.manifest.Name
	slotName := p.slot.Name

	for _, method := range p.methods() {
		if method.Tags[ktsp.TagMethodMCP] == "false" {
			continue
		}
		toolName := method.Name
		if customName, ok := method.Tags[ktsp.TagMethodMCPName]; ok && customName != "" {
			toolName = customName
		}
		if toolName != name {
			continue
		}

		sender, err := p.senderFactory(containerName)
		if err != nil {
			return nil, fmt.Errorf("connect to %s: %w", containerName, err)
		}
		// Named-slot URI form: <provider>:/Slot/Method. The provider
		// is the target sidecar's name (from its manifest); the slot
		// is the unkebabed Go-source form (kebab normalization is
		// applied by the path parser). Instance defaults to "default".
		path := containerName + ":/" + slotName + "/" + method.Name
		result, err := sender.Call(ctx, path, json.RawMessage(input))
		if err != nil {
			return nil, err
		}
		return agentapi.StringResult(string(result)), nil
	}

	return nil, fmt.Errorf("unknown tool: %q", name)
}

// methods returns the method contract backing this slot. Typed slots resolve
// through the manifest's SlotTypes table (the canonical, post-dedup source of
// truth with full tags and schemas). Untyped slots fall back to the inline
// list on the slot itself (legacy shape).
func (p *sidecarPlugin) methods() []ktsp.MethodInfo {
	if p.slot.SlotType != "" {
		for _, st := range p.manifest.KTSP.SlotTypes {
			if st.Type == p.slot.SlotType {
				return st.Methods
			}
		}
	}
	return p.slot.Methods
}

func (p *sidecarPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p *sidecarPlugin) Stable() bool                { return true }

func sidecarDescriptionFor(toolName string, m *ktsp.MethodInfo) string {
	if m != nil && m.Description != "" {
		return m.Description
	}
	return toolName
}

func sidecarSchemaFor(m *ktsp.MethodInfo) json.RawMessage {
	if m != nil && m.InputSchema != nil {
		// InputSchema is stored as a decoded tree (map/slice). Re-marshal
		// it to JSON for the ToolDef which expects json.RawMessage bytes.
		if raw, err := json.Marshal(m.InputSchema); err == nil {
			return raw
		}
	}
	return json.RawMessage(`{"type": "object"}`)
}
