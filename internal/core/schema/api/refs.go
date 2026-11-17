// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"fmt"
	"sort"
	"strings"
)

// A field may hold a reference to an object of the project — `object-ref`,
// its value the object's id — or to one panel of an object — `panel-ref`, its
// value `<id>#<panel>`. Its content params say which objects it may pick: the
// ObjectFilter of CWM (types, capabilities, incoming, outgoing, when), so the
// form's picker searches exactly what a panel or a policy would admit.
//
//	owner:   string@object-ref(types:person|team)
//	feature: string@object-ref(types:feature, capabilities:verification)
//	view:    string@panel-ref(types:module)
//	related: {type: string, content: {type: object-ref, types: [decision], when: "state == 'accepted'"}}
//
// In the shorthand a list is written with `|` (`types:feature|use-case`); a
// `when` holding a comma or a colon is written in the block form.

// The reference content types.
const (
	ContentObjectRef = "object-ref"
	ContentPanelRef  = "panel-ref"
)

// RefFilterKeys are the params a reference field takes — an ObjectFilter's
// fields; the first four are lists, `when` an el predicate.
var RefFilterKeys = []string{"types", "capabilities", "incoming", "outgoing", "when"}

// IsRefContent reports a reference content type.
func IsRefContent(content string) bool {
	return content == ContentObjectRef || content == ContentPanelRef
}

// NormalizeRefFilter turns a reference field's params into its filter: each
// list param a list of strings (a string splits on `|` or `,`), `when` a
// string; an unknown key, or a value of the wrong shape, is refused, naming
// it. Params a host computed on its own (`options`) are kept as they are.
func NormalizeRefFilter(params map[string]any) (map[string]any, error) {
	out := map[string]any{}
	var bad []string
	for k, v := range params {
		switch k {
		case "when":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("when must be an el predicate (a string), got %T", v)
			}
			if s != "" {
				out[k] = s
			}
		case "types", "capabilities", "incoming", "outgoing":
			list, err := stringList(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			if len(list) > 0 {
				out[k] = list
			}
		case "options", OptionsFromKey:
			out[k] = v
		default:
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return nil, fmt.Errorf("%s is no filter of a reference (%s)", strings.Join(bad, ", "), strings.Join(RefFilterKeys, ", "))
	}
	return out, nil
}

// RefFilter is the filter part of a reference field's params (no host
// computed key), as JSON Schema's `x-filter` carries it.
func RefFilter(params map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range RefFilterKeys {
		if v, ok := params[k]; ok {
			out[k] = v
		}
	}
	return out
}

func stringList(v any) ([]any, error) {
	switch t := v.(type) {
	case string:
		var out []any
		for _, p := range strings.FieldsFunc(t, func(r rune) bool { return r == '|' || r == ',' }) {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(t))
		for _, x := range t {
			s, ok := x.(string)
			if !ok {
				return nil, fmt.Errorf("a list of names, got %T in it", x)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		out := make([]any, 0, len(t))
		for _, s := range t {
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("a name or a list of names, got %T", v)
}
