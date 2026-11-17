// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// OptionSourceCatalog is the `options_from` source name of CatalogOptions.
const OptionSourceCatalog = "catalog"

// CatalogOptions is the `catalog` option source: the contribution entries of
// one kind that q reaches, as records a form field chooses from
// (schemaapi.ResolveOptions; modules/framework/schema/dsl-content-types.md).
//
// Parameters: `kind` (required), `providers` (only these sidecars).
// Record: `id`, `uri`, `kind`, `provider`, the picker fields (`name`,
// `description`, `icon`, `hashtags`, `category`) and `data` — the entry's
// body, so a selection can read or filter on any field of it
// (`where: {data.aspect: "16:9"}`).
func CatalogOptions(q Q) schemaapi.OptionSource {
	return schemaapi.OptionSourceFunc(func(ctx context.Context, params map[string]any) ([]map[string]any, error) {
		kind, _ := params["kind"].(string)
		if kind == "" {
			return nil, fmt.Errorf("the catalog source needs a kind")
		}
		qq := q
		qq.Kind = kind
		if ps, ok := params["providers"]; ok {
			list, err := nameList(ps)
			if err != nil {
				return nil, fmt.Errorf("providers: %w", err)
			}
			qq.Filter.Providers = list
		}
		entries, err := QueryTyped[map[string]any](ctx, qq)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			r := map[string]any{
				"uri":         e.URI,
				"kind":        kind,
				"provider":    e.Provider,
				"name":        e.Meta.Name,
				"description": e.Meta.Description,
				"category":    e.Meta.Category,
				"data":        e.Body,
			}
			if u, err := contribapi.ParseResourceURI(e.URI); err == nil {
				r["id"] = u.ID
			}
			if len(e.Meta.Hashtags) > 0 {
				tags := make([]any, len(e.Meta.Hashtags))
				for i, h := range e.Meta.Hashtags {
					tags[i] = h
				}
				r["hashtags"] = tags
			}
			if e.Meta.Icon != nil && e.Meta.Icon.Tabler != "" {
				r["icon"] = e.Meta.Icon.Tabler
			}
			out = append(out, r)
		}
		return out, nil
	})
}

// nameList reads a list of names from YAML/JSON (a list, or one string).
func nameList(v any) ([]string, error) {
	switch t := v.(type) {
	case string:
		return []string{t}, nil
	case []string:
		return t, nil
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("a list of names expected")
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("a list of names expected")
}
