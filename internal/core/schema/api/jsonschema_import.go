// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// FromJSONSchema reads back a JSON Schema document into the canonical AST —
// the inverse of ToJSONSchema, for a contract that only travels as JSON
// Schema: a method's input in a manifest is projected there from its Go type,
// and a form that renders it needs the AST again (ActionService.Describe's
// input_schema_ast).
//
// It reads what ToJSONSchema writes and nothing more: type (object, string,
// number / integer, boolean, array, null), properties + required (absent =
// optional), additionalProperties, items, minItems / maxItems, enum, format
// (→ content), x-filter (→ the content's filter params), anyOf / oneOf
// (→ choice), title (→ label), description, default and $ref to `$defs` /
// `definitions` (→ named types). The root becomes the type named name (or
// `Input`), the root of the schema.
func FromJSONSchema(raw json.RawMessage, name string) (*Schema, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("json schema: empty")
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("json schema: %w", err)
	}
	if name == "" {
		name = "Input"
	}
	s := &Schema{Name: name, Types: map[string]*Type{}}
	defs := map[string]any{}
	for _, key := range []string{"$defs", "definitions"} {
		if d, ok := doc[key].(map[string]any); ok {
			for k, v := range d {
				defs[k] = v
			}
		}
	}
	names := make([]string, 0, len(defs))
	for k := range defs {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		m, _ := defs[k].(map[string]any)
		t, err := typeFromJSONSchema(m, k)
		if err != nil {
			return nil, err
		}
		s.Types[k] = t
	}
	if ref, ok := doc["$ref"].(string); ok && len(doc) <= 4 {
		// The root is a reference to one of its definitions.
		target := refName(ref)
		if _, known := s.Types[target]; !known {
			return nil, fmt.Errorf("json schema: root $ref %q names no definition", ref)
		}
		s.Root = target
		return s, nil
	}
	root, err := typeFromJSONSchema(doc, name)
	if err != nil {
		return nil, err
	}
	if root.Label == name {
		root.Label = ""
	}
	s.Types[name] = root
	s.Root = name
	return s, nil
}

func refName(ref string) string {
	for _, p := range []string{"#/$defs/", "#/definitions/"} {
		if strings.HasPrefix(ref, p) {
			return strings.TrimPrefix(ref, p)
		}
	}
	return ref
}

func typeFromJSONSchema(m map[string]any, at string) (*Type, error) {
	if m == nil {
		return NewType(KindAny), nil
	}
	if ref, ok := m["$ref"].(string); ok {
		t := NewType(TypeKind(refName(ref)))
		meta(t, m)
		return t, nil
	}
	for _, key := range []string{"anyOf", "oneOf"} {
		if cases, ok := m[key].([]any); ok {
			t := NewType(KindChoice)
			for i, c := range cases {
				cm, _ := c.(map[string]any)
				ct, err := typeFromJSONSchema(cm, fmt.Sprintf("%s|%d", at, i))
				if err != nil {
					return nil, err
				}
				t.Cases = append(t.Cases, refTo(ct))
			}
			meta(t, m)
			return t, nil
		}
	}
	kind := ""
	switch v := m["type"].(type) {
	case string:
		kind = v
	case []any:
		// ["string", "null"]: the non-null one, optional.
		for _, x := range v {
			if s, _ := x.(string); s != "" && s != "null" {
				kind = s
			}
		}
	}
	if kind == "" {
		switch {
		case m["properties"] != nil:
			kind = "object"
		case m["items"] != nil:
			kind = "array"
		case m["enum"] != nil:
			kind = "string"
		}
	}
	var t *Type
	switch kind {
	case "object":
		t = NewType(KindObject)
		required := map[string]bool{}
		if rs, ok := m["required"].([]any); ok {
			for _, r := range rs {
				if s, _ := r.(string); s != "" {
					required[s] = true
				}
			}
		}
		if props, ok := m["properties"].(map[string]any); ok {
			t.Fields = map[string]*Type{}
			for k, p := range props {
				pm, _ := p.(map[string]any)
				ft, err := typeFromJSONSchema(pm, at+"."+k)
				if err != nil {
					return nil, err
				}
				ft.Optional = !required[k]
				t.Fields[k] = ft
			}
		}
		if ap, ok := m["additionalProperties"].(map[string]any); ok {
			ct, err := typeFromJSONSchema(ap, at+"{}")
			if err != nil {
				return nil, err
			}
			t.Catchall = refTo(ct)
		}
	case "array":
		t = NewType(KindArray)
		if items, ok := m["items"].(map[string]any); ok {
			it, err := typeFromJSONSchema(items, at+"[]")
			if err != nil {
				return nil, err
			}
			t.Items = refTo(it)
		}
		if v, ok := m["minItems"].(float64); ok {
			t.Min = &v
		}
		if v, ok := m["maxItems"].(float64); ok {
			t.Max = &v
		}
	case "string":
		t = NewType(KindString)
		if f, _ := m["format"].(string); f != "" && f != "contribution-uri" {
			t.Content = f
		}
		if filter, ok := m["x-filter"].(map[string]any); ok && len(filter) > 0 {
			t.ContentParams = map[string]any{}
			for k, v := range filter {
				t.ContentParams[k] = v
			}
		}
	case "number", "integer":
		t = NewType(KindNumber)
	case "boolean":
		t = NewType(KindBoolean)
	case "null":
		t = NewType(KindNull)
	default:
		t = NewType(KindAny)
	}
	if e, ok := m["enum"].([]any); ok && (t.Kind == KindString || t.Kind == KindNumber) {
		t.Enum = e
	}
	meta(t, m)
	return t, nil
}

// refTo is an inline type, or a bare name reference for a reference to a
// named type.
func refTo(t *Type) *TypeRef {
	switch t.Kind {
	case KindString, KindNumber, KindBoolean, KindObject, KindArray, KindChoice, KindLink, KindTree, KindAny, KindNull:
		return RefInline(t)
	}
	if t.Label == "" && t.Description == "" && t.Default == nil {
		return RefName(string(t.Kind))
	}
	return RefInline(t)
}

func meta(t *Type, m map[string]any) {
	if s, _ := m["title"].(string); s != "" {
		t.Label = s
	}
	if s, _ := m["description"].(string); s != "" {
		t.Description = s
	}
	if d, ok := m["default"]; ok {
		t.Default = d
	}
}
