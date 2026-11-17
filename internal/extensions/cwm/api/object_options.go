// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// OptionSourceObjects is the `options_from` source answering the project's
// objects — a form's choice among them, computed when the form is served
// (modules/framework/schema/dsl-content-types.md, "Options from a source"):
//
//	options_from: {source: objects, types: [feature], when: "state != 'done'", label: name}
//
// Its params are an ObjectFilter (types, capabilities, incoming, outgoing,
// when) and `module` / `query`; each record is {id, name, type, module,
// description}. A reference field (`string@object-ref`) searches live through
// its host instead; this is the fallback where there is no host to ask.
const OptionSourceObjects = "objects"

// ObjectOptions is the `objects` option source over a CWM service: one
// ObjectSearch with the params' filter.
func ObjectOptions(svc CWMService) schemaapi.OptionSource {
	return schemaapi.OptionSourceFunc(func(ctx context.Context, params map[string]any) ([]map[string]any, error) {
		if svc == nil {
			return nil, fmt.Errorf("objects: no CWM service here")
		}
		filter, err := schemaapi.NormalizeRefFilter(schemaapi.RefFilter(params))
		if err != nil {
			return nil, fmt.Errorf("objects: %w", err)
		}
		in := ObjectSearchInput{}
		if q, _ := params["query"].(string); q != "" {
			in.Query = q
		}
		if m, _ := params["module"].(string); m != "" {
			in.Modules = []string{m}
		}
		if len(filter) > 0 {
			in.Filter = &ObjectFilter{
				Types: names(filter["types"]), Capabilities: names(filter["capabilities"]),
				Incoming: names(filter["incoming"]), Outgoing: names(filter["outgoing"]),
			}
			in.Filter.When, _ = filter["when"].(string)
		}
		out, err := svc.ObjectSearch(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("objects: %w", err)
		}
		records := make([]map[string]any, 0, len(out.Objects))
		for _, o := range out.Objects {
			module, _ := ModuleOfObjectID(o.ID)
			records = append(records, map[string]any{
				"id": o.ID, "name": o.Name, "type": o.Type, "module": module, "description": o.Description,
			})
		}
		return records, nil
	})
}

func names(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
