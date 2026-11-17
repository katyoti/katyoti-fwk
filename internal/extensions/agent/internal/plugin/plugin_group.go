// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// GroupRegistry maps group names to their member refs.
type GroupRegistry map[string][]api.PluginRef

// ExtractGroups separates group definitions from regular refs.
// Group refs (type: "group" with "members" in Props) are extracted into the registry.
// Non-group refs are returned as-is.
func ExtractGroups(refs []api.PluginRef) (regular []api.PluginRef, groups GroupRegistry) {
	groups = make(GroupRegistry)
	for _, ref := range refs {
		if ref.Type == "group" && ref.Props != nil {
			if members, ok := ref.Props["members"]; ok {
				parsed, err := parseInlineMembers(members)
				if err == nil && len(parsed) > 0 {
					groups[ref.Name] = parsed
					continue
				}
			}
		}
		regular = append(regular, ref)
	}
	return regular, groups
}

// ExpandGroups recursively expands group refs into concrete refs.
// Layer 1 operation — runs before strategy resolution.
func ExpandGroups(refs []api.PluginRef, groups GroupRegistry) ([]api.PluginRef, error) {
	var result []api.PluginRef
	seen := make(map[string]bool)

	for _, ref := range refs {
		expanded, err := expandRef(ref, groups, seen, nil)
		if err != nil {
			return nil, err
		}
		result = append(result, expanded...)
	}
	return result, nil
}

func expandRef(ref api.PluginRef, groups GroupRegistry, seen map[string]bool, path []string) ([]api.PluginRef, error) {
	key := ref.Ref()
	// A group already on the path is a CYCLE, and must be said before the
	// dedup below swallows it: until 2026-09-22 `a → b → a` returned nil,
	// because `a` was "seen" by the time it came round.
	if ref.Type == "group" || groups[ref.Type] != nil {
		name := ref.Name
		if ref.Type != "group" {
			name = ref.Type
		}
		for _, p := range path {
			if p == name {
				return nil, fmt.Errorf("group cycle: %s → %s", strings.Join(path, " → "), name)
			}
		}
	}

	if seen[key] {
		return nil, nil // dedup
	}

	// A group ref can be either:
	// - {Type: "group", Name: "standard"} — explicit group type
	// - {Type: "standard"} — shorthand where Type matches a group name
	groupName := ref.Name
	if ref.Type == "group" {
		// explicit: type=group, name=standard
	} else if _, ok := groups[ref.Type]; ok {
		// shorthand: type=standard (matches a group name)
		groupName = ref.Type
	} else {
		// not a group — pass through
		seen[key] = true
		return []api.PluginRef{ref}, nil
	}

	// Check cycle
	for _, p := range path {
		if p == groupName {
			return nil, fmt.Errorf("group cycle: %s → %s", strings.Join(path, " → "), groupName)
		}
	}

	members, ok := groups[groupName]
	if !ok {
		return nil, fmt.Errorf("unknown group %q", groupName)
	}

	seen[key] = true
	newPath := append(path, groupName) // the group by its NAME: a shorthand ref has an empty Name

	var result []api.PluginRef
	for _, member := range members {
		expanded, err := expandRef(member, groups, seen, newPath)
		if err != nil {
			return nil, fmt.Errorf("group %q: %w", ref.Name, err)
		}
		result = append(result, expanded...)
	}
	return result, nil
}

// parseInlineMembers converts the "members" prop ([]any from YAML) into []PluginRef.
func parseInlineMembers(raw any) ([]api.PluginRef, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("group members must be a list")
	}
	refs := make([]api.PluginRef, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			refs = append(refs, api.ParsePluginRef(v))
		case map[string]any:
			typeName, _ := v["type"].(string)
			name, _ := v["name"].(string)
			if typeName == "" {
				return nil, fmt.Errorf("group member mapping requires 'type' field")
			}
			ref := api.PluginRef{Type: typeName, Name: name}
			delete(v, "type")
			delete(v, "name")
			if len(v) > 0 {
				ref.Props = v
			}
			refs = append(refs, ref)
		default:
			return nil, fmt.Errorf("group member must be string or mapping")
		}
	}
	return refs, nil
}
