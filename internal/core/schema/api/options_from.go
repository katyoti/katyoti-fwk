// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// A field's choices, computed when a form is served rather than written in
// the schema: `content: {type: text, options_from: {...}}`. See
// modules/framework/schema/dsl-content-types.md, "Options from a source".
//
//	options_from:
//	  source: catalog              # which OptionSource produces the records
//	  kind: deck-theme             # …the source's own parameters
//	  where: {hashtags: corporate} # keep records whose path equals (or contains) the value
//	  value: name                  # the option's value — a path in the record (default id)
//	  label: description           # its label (default: the value)
//	  description: description     # an optional second line
//	  icon: icon
//	  sort: label                  # a path, or "-path" descending (default label)
//	  limit: 50
//
// Every source answers the same question — "the records to choose from" —
// and the selection (where, sort, limit, value, label…) is applied here, the
// same for all. A source is therefore a few lines: the catalog's entries, a
// workspace's objects, a fixed list, a method's result.

// OptionsFromKey is the contentParams key holding a field's option source.
const OptionsFromKey = "options_from"

// OptionsFrom is a parsed `options_from` block.
type OptionsFrom struct {
	Source      string
	Value       string
	Label       string
	Description string
	Icon        string
	Where       map[string]any
	Sort        string
	Limit       int
	// Params are the source's own keys — everything the selection does not
	// read (`kind`, `providers`, `uri`, `input`, `items`…).
	Params map[string]any
}

// OptionSource produces the records a field chooses from. A record is a flat
// or nested map; the selection reads it by dotted paths.
type OptionSource interface {
	Records(ctx context.Context, params map[string]any) ([]map[string]any, error)
}

// OptionSourceFunc adapts a function to OptionSource.
type OptionSourceFunc func(ctx context.Context, params map[string]any) ([]map[string]any, error)

// Records implements OptionSource.
func (f OptionSourceFunc) Records(ctx context.Context, params map[string]any) ([]map[string]any, error) {
	return f(ctx, params)
}

// ParseOptionsFrom reads an `options_from` block.
func ParseOptionsFrom(raw any) (OptionsFrom, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		return OptionsFrom{}, fmt.Errorf("options_from must be a mapping, got %T", raw)
	}
	o := OptionsFrom{Params: map[string]any{}}
	str := func(k string) string { s, _ := m[k].(string); return s }
	for k, v := range m {
		switch k {
		case "source", "value", "label", "description", "icon", "sort":
			if _, ok := v.(string); !ok {
				return OptionsFrom{}, fmt.Errorf("options_from.%s must be a string", k)
			}
		case "where":
			w, ok := v.(map[string]any)
			if !ok {
				return OptionsFrom{}, fmt.Errorf("options_from.where must be a mapping of path to value")
			}
			o.Where = w
		case "limit":
			n, err := toInt(v)
			if err != nil || n < 0 {
				return OptionsFrom{}, fmt.Errorf("options_from.limit must be a positive number")
			}
			o.Limit = n
		default:
			o.Params[k] = v
		}
	}
	o.Source, o.Value, o.Label, o.Description, o.Icon, o.Sort =
		str("source"), str("value"), str("label"), str("description"), str("icon"), str("sort")
	if o.Source == "" {
		return OptionsFrom{}, fmt.Errorf("options_from.source is required")
	}
	if o.Value == "" {
		o.Value = "id"
	}
	if o.Label == "" {
		o.Label = o.Value
	}
	if o.Sort == "" {
		o.Sort = "label"
	}
	return o, nil
}

// Options turns a source's records into a field's options: filtered by
// Where, projected by Value / Label / Description / Icon, sorted, limited.
// A record without a value is skipped; the first record of a value wins.
func (o OptionsFrom) Options(records []map[string]any) []map[string]any {
	type row struct {
		opt  map[string]any
		sort string
	}
	var rows []row
	seen := map[string]bool{}
	for _, r := range records {
		if !o.matches(r) {
			continue
		}
		v := Lookup(r, o.Value)
		key := text(v)
		if v == nil || key == "" || seen[key] {
			continue
		}
		seen[key] = true
		opt := map[string]any{"value": v, "label": orText(Lookup(r, o.Label), key)}
		if o.Description != "" {
			if d := text(Lookup(r, o.Description)); d != "" {
				opt["description"] = d
			}
		}
		if o.Icon != "" {
			if i := text(Lookup(r, o.Icon)); i != "" {
				opt["icon"] = i
			}
		}
		by := strings.TrimPrefix(o.Sort, "-")
		var s string
		switch by {
		case "label":
			s = text(opt["label"])
		case "value":
			s = key
		default:
			s = text(Lookup(r, by))
		}
		rows = append(rows, row{opt, strings.ToLower(s)})
	}
	desc := strings.HasPrefix(o.Sort, "-")
	sort.SliceStable(rows, func(i, j int) bool {
		if desc {
			return rows[i].sort > rows[j].sort
		}
		return rows[i].sort < rows[j].sort
	})
	if o.Limit > 0 && len(rows) > o.Limit {
		rows = rows[:o.Limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.opt)
	}
	return out
}

// matches: every Where path equals its value; a list value means any of
// them; a list in the record matches when it contains the value.
func (o OptionsFrom) matches(r map[string]any) bool {
	for path, want := range o.Where {
		got := Lookup(r, path)
		if !matchValue(got, want) {
			return false
		}
	}
	return true
}

func matchValue(got, want any) bool {
	if ws, ok := want.([]any); ok {
		for _, w := range ws {
			if matchValue(got, w) {
				return true
			}
		}
		return false
	}
	if gs, ok := got.([]any); ok {
		for _, g := range gs {
			if text(g) == text(want) {
				return true
			}
		}
		return false
	}
	if gs, ok := got.([]string); ok {
		for _, g := range gs {
			if g == text(want) {
				return true
			}
		}
		return false
	}
	return got != nil && text(got) == text(want)
}

// Lookup reads a dotted path ("data.aspect") in a record; nil when absent.
func Lookup(r map[string]any, path string) any {
	var cur any = r
	for _, seg := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[seg]
	}
	return cur
}

func text(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		if reflect.TypeOf(v).Kind() == reflect.Map || reflect.TypeOf(v).Kind() == reflect.Slice {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func orText(v any, fallback string) string {
	if s := text(v); s != "" {
		return s
	}
	return fallback
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case float64:
		return int(n), nil
	case string:
		return strconv.Atoi(n)
	}
	return 0, fmt.Errorf("not a number")
}

// ResolveOptions fills, in place, every field of s that declares an
// `options_from`: its `contentParams.options` (what a form shows) and its
// `enum` (what a JSON Schema reader — the Actions modal, a validator —
// restricts to). A field whose source is unknown, or fails, is left as it was
// (free text) and reported; the form still opens.
func ResolveOptions(ctx context.Context, s *Schema, sources map[string]OptionSource) []error {
	if s == nil {
		return nil
	}
	var errs []error
	seen := map[*Type]bool{}
	var walk func(path string, t *Type)
	walkRef := func(path string, r *TypeRef) {
		if r != nil && r.Inline != nil {
			walk(path, r.Inline)
		}
	}
	walk = func(path string, t *Type) {
		if t == nil || seen[t] {
			return
		}
		seen[t] = true
		if raw, ok := t.ContentParams[OptionsFromKey]; ok {
			if err := resolveField(ctx, t, raw, sources); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", path, err))
			}
		}
		for name, f := range t.Fields {
			walk(path+"."+name, f)
		}
		walkRef(path+"[]", t.Items)
		walkRef(path+"{}", t.Catchall)
		for i, c := range t.Cases {
			walkRef(fmt.Sprintf("%s|%d", path, i), c)
		}
	}
	names := make([]string, 0, len(s.Types))
	for n := range s.Types {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		walk(n, s.Types[n])
	}
	return errs
}

func resolveField(ctx context.Context, t *Type, raw any, sources map[string]OptionSource) error {
	spec, err := ParseOptionsFrom(raw)
	if err != nil {
		return err
	}
	src, ok := sources[spec.Source]
	if !ok || src == nil {
		known := make([]string, 0, len(sources))
		for k := range sources {
			known = append(known, k)
		}
		sort.Strings(known)
		return fmt.Errorf("options_from: no source %q here (sources: %s)", spec.Source, strings.Join(known, ", "))
	}
	records, err := src.Records(ctx, spec.Params)
	if err != nil {
		return fmt.Errorf("options_from %s: %w", spec.Source, err)
	}
	opts := spec.Options(records)
	list := make([]any, 0, len(opts))
	enum := make([]any, 0, len(opts))
	for _, o := range opts {
		list = append(list, o)
		enum = append(enum, o["value"])
	}
	t.ContentParams["options"] = list
	if len(enum) > 0 {
		t.Enum = enum
	}
	return nil
}
