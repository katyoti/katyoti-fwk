// SPDX-License-Identifier: Apache-2.0

package cwmprojection

import (
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// parseRFC3339 parses an RFC3339 / time.DateTime / ISO-8601 string.
// Returns zero time on parse failure (caller decides whether to treat
// as missing or surface an error).
func parseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(time.DateTime, s)
}

// anyToStringSlice converts an interface value from a YAML-decoded
// TypeSpecific map to a []string. Returns nil if the value is
// missing, nil, or not a slice.
func anyToStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// anyToFacets decodes the `facets` TypeSpecific entry — a list of
// {kind, path, role?} maps from YAML/JSON decoding — into typed
// Facets. Entries that aren't maps or lack a kind/path are skipped
// (lint's InvalidFacets surfaces the bad ones from the typed view).
// Handles both map[string]any (JSON) and map[any]any (YAML v2-style
// nested maps — the known normalization gotcha).
func anyToFacets(v any) []gcapi.Facet {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []gcapi.Facet
	for _, item := range list {
		var get func(key string) string
		switch m := item.(type) {
		case map[string]any:
			get = func(key string) string {
				s, _ := m[key].(string)
				return s
			}
		case map[any]any:
			get = func(key string) string {
				s, _ := m[key].(string)
				return s
			}
		default:
			continue
		}
		f := gcapi.Facet{
			Kind: gcapi.FacetKind(get("kind")),
			Path: get("path"),
			Role: get("role"),
		}
		if f.Kind == "" && f.Path == "" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// anyToInt extracts an int from interfaces produced by YAML/JSON
// decoding (which give us int, int64, float64, or string-int).
func anyToInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	}
	return 0, false
}
