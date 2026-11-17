// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// DesugarPluginLists folds the `Skills` and `Tools` sugar lists into
// `Plugins` as ONE ref each, carrying the list in Props:
//
//	skills: [cwm-*, ktw-methods:/skill/render-data-as-ui]
//	  → plugins += {type: skill, props: {skills: [...]}}
//	tools:  [ktw-weather:weather-service/*]
//	  → plugins += {type: tool,  props: {tools:  [...]}}
//
// The factory for each type reads the list from Props and resolves the
// wildcards. Carrying the raw strings in a Props array (vs N `skill:<x>` refs)
// keeps a provider-scoped URI's `:` intact — it is never run through
// ParsePluginRef. Call once after decoding the request from a TASK.md
// `config:` block; it clears Skills/Tools so a second call is a no-op.
func (r *AgentRunRequest) DesugarPluginLists() {
	if list := trimmedNonEmpty(r.Skills); len(list) > 0 {
		r.Plugins = append(r.Plugins, PluginRef{Type: "skill", Props: map[string]any{"skills": list}})
	}
	if list := trimmedNonEmpty(r.Tools); len(list) > 0 {
		r.Plugins = append(r.Plugins, PluginRef{Type: "tool", Props: map[string]any{"tools": list}})
	}
	r.Skills = nil
	r.Tools = nil
}

func trimmedNonEmpty(in []string) []any {
	var out []any
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// StringList coerces a Props value (`[]any` from JSON/YAML decode, or `[]string`
// when set in Go) into a `[]string`. Non-string / non-list values yield nil.
// Used by plugin-list factories to read `props.skills` / `props.tools`.
func StringList(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
