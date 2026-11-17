// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// PluginRef is a unified plugin reference.
//
// Two forms:
//   - String: "type:name" or "type" (e.g., "group:standard", "workspace")
//   - Map: {type: "mcp", name: "my-server", members: [...], server: "npx ..."}
//
// Skills and tools are NOT referenced here — declare them in a task's dedicated
// `skills:` / `tools:` lists, which desugar into props-carrying refs.
//
// The "type" field is mandatory in map form. All fields except "type" and "name"
// become Props (strategy-specific configuration).
type PluginRef struct {
	// Type is the plugin strategy name (e.g., "skill", "task", "mcp", "group", "workspace").
	Type string `json:"type" yaml:"type"`

	// Name is the optional instance name or pattern (strategy-specific).
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Props holds strategy-specific configuration (flat, not nested).
	Props map[string]any `json:"props,omitempty" yaml:"props,omitempty"`
}

// Ref returns the canonical string representation "type:name" or just "type".
func (r PluginRef) Ref() string {
	if r.Name == "" {
		return r.Type
	}
	return r.Type + ":" + r.Name
}

// ParsePluginRef parses a string plugin reference.
// Format: "type:name" or "type" (no name).
func ParsePluginRef(ref string) PluginRef {
	if idx := strings.Index(ref, ":"); idx >= 0 {
		return PluginRef{Type: ref[:idx], Name: ref[idx+1:]}
	}
	return PluginRef{Type: ref}
}

// UnmarshalYAML handles string and mapping forms.
func (r *PluginRef) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*r = ParsePluginRef(value.Value)
		return nil
	case yaml.MappingNode:
		var raw map[string]any
		if err := value.Decode(&raw); err != nil {
			return fmt.Errorf("plugin ref: %w", err)
		}
		typeName, ok := raw["type"].(string)
		if !ok || typeName == "" {
			return fmt.Errorf("plugin ref mapping requires 'type' field")
		}
		r.Type = typeName
		delete(raw, "type")
		if name, ok := raw["name"].(string); ok {
			r.Name = name
			delete(raw, "name")
		}
		if len(raw) > 0 {
			r.Props = raw
		}
		return nil
	default:
		return fmt.Errorf("plugin ref must be a string or mapping, got kind %d", value.Kind)
	}
}

// UnmarshalJSON handles string and object forms.
func (r *PluginRef) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*r = ParsePluginRef(s)
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("plugin ref: expected string or object, got %s", string(data))
	}
	typeName, ok := raw["type"].(string)
	if !ok || typeName == "" {
		return fmt.Errorf("plugin ref object requires 'type' field")
	}
	r.Type = typeName
	delete(raw, "type")
	if name, ok := raw["name"].(string); ok {
		r.Name = name
		delete(raw, "name")
	}
	if len(raw) > 0 {
		r.Props = raw
	}
	return nil
}

// MarshalYAML emits string form when simple, map form when props exist.
func (r PluginRef) MarshalYAML() (any, error) {
	if len(r.Props) == 0 {
		return r.Ref(), nil
	}
	m := make(map[string]any, len(r.Props)+2)
	m["type"] = r.Type
	if r.Name != "" {
		m["name"] = r.Name
	}
	for k, v := range r.Props {
		m[k] = v
	}
	return m, nil
}
