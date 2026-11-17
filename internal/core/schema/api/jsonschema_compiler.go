// SPDX-License-Identifier: Apache-2.0

// Compiler from the canonical Type AST → JSON Schema 2020-12.
//
// The Type AST is the source of truth for shape; JSON Schema is a
// derived artifact for polyglot validators that don't speak DSL.
// Both paths (DSL → Type AST and Go-struct-via-reflection → Type AST)
// converge into the same compiler.
//
// Symmetric with the DSL parser's output: every DSL feature that
// produces a Type AST node is representable as JSON Schema.

package schemaapi

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ToJSONSchema compiles this Schema to a JSON Schema 2020-12 document.
// The schema is keyed by the type names in s.Types; if s.Root is set,
// it becomes the top-level type reference.
func (s *Schema) ToJSONSchema() (json.RawMessage, error) {
	root := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
	}
	if s.Name != "" {
		root["title"] = s.Name
	}
	if s.Description != "" {
		root["description"] = s.Description
	}
	defs := map[string]any{}
	for name, t := range s.Types {
		js, err := t.compileJSONSchema()
		if err != nil {
			return nil, fmt.Errorf("type %q: %w", name, err)
		}
		defs[name] = js
	}
	if len(defs) > 0 {
		root["$defs"] = defs
	}
	if s.Root != "" {
		if _, ok := s.Types[s.Root]; !ok {
			return nil, fmt.Errorf("root type %q not found in Types map", s.Root)
		}
		root["$ref"] = "#/$defs/" + s.Root
	}
	return json.Marshal(root)
}

// MustToJSONSchema panics on compile error. For init-time use.
func (s *Schema) MustToJSONSchema() json.RawMessage {
	js, err := s.ToJSONSchema()
	if err != nil {
		panic(err)
	}
	return js
}

// ToJSONSchema compiles a single Type to a JSON Schema fragment. The
// result is a self-contained schema (no $defs); composite types
// inline their child references.
func (t *Type) ToJSONSchema() (json.RawMessage, error) {
	if t == nil {
		return nil, fmt.Errorf("nil Type")
	}
	js, err := t.compileJSONSchema()
	if err != nil {
		return nil, err
	}
	js["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	return json.Marshal(js)
}

// MustToJSONSchema panics on compile error.
func (t *Type) MustToJSONSchema() json.RawMessage {
	js, err := t.ToJSONSchema()
	if err != nil {
		panic(err)
	}
	return js
}

// compileJSONSchema emits the per-Type body without the $schema header.
// Recursive for object / array / choice / link.
func (t *Type) compileJSONSchema() (map[string]any, error) {
	out := map[string]any{}
	if t.Description != "" {
		out["description"] = t.Description
	}
	if t.Label != "" {
		out["title"] = t.Label
	}
	if t.Default != nil {
		out["default"] = t.Default
	}

	switch t.Kind {
	case KindString:
		out["type"] = "string"
		if t.Content != "" {
			// Content-type maps loosely to JSON Schema format.
			out["format"] = t.Content
		}
		if IsRefContent(t.Content) {
			// A reference carries the filter of the objects it may name.
			if f := RefFilter(t.ContentParams); len(f) > 0 {
				out["x-filter"] = f
			}
		}
		if len(t.Enum) > 0 {
			out["enum"] = t.Enum
		}
	case KindNumber:
		out["type"] = "number"
		if len(t.Enum) > 0 {
			out["enum"] = t.Enum
		}
	case KindBoolean:
		out["type"] = "boolean"
	case KindNull:
		out["type"] = "null"
	case KindAny:
		// JSON Schema 2020-12: empty schema accepts anything.
	case KindObject:
		out["type"] = "object"
		if len(t.Fields) > 0 {
			props := map[string]any{}
			required := []string{}
			for fname, ftype := range t.Fields {
				sub, err := ftype.compileJSONSchema()
				if err != nil {
					return nil, fmt.Errorf("field %q: %w", fname, err)
				}
				props[fname] = sub
				if !ftype.Optional {
					required = append(required, fname)
				}
			}
			out["properties"] = props
			if len(required) > 0 {
				// Sorted, because Fields is a map and its iteration order is
				// random per run. Without this the SAME type compiled twice
				// produces different bytes, so anything that hashes a derived
				// schema — a content address, a cache key, a golden file —
				// churns for no reason. `properties` is already stable because
				// encoding/json sorts map keys; this array is not a map.
				sort.Strings(required)
				out["required"] = required
			}
		}
		if t.Catchall != nil {
			ap, err := compileRef(t.Catchall)
			if err != nil {
				return nil, fmt.Errorf("catchall: %w", err)
			}
			out["additionalProperties"] = ap
		} else {
			// Be permissive by default — closed-schema is opt-in via DSL or future flag.
			out["additionalProperties"] = true
		}
	case KindArray:
		out["type"] = "array"
		if t.Items != nil {
			items, err := compileRef(t.Items)
			if err != nil {
				return nil, fmt.Errorf("items: %w", err)
			}
			out["items"] = items
		}
		if t.Min != nil {
			out["minItems"] = *t.Min
		}
		if t.Max != nil {
			out["maxItems"] = *t.Max
		}
	case KindChoice:
		anyOf := []any{}
		for i, c := range t.Cases {
			sub, err := compileRef(c)
			if err != nil {
				return nil, fmt.Errorf("choice case %d: %w", i, err)
			}
			anyOf = append(anyOf, sub)
		}
		out["anyOf"] = anyOf
	case KindLink, KindTree:
		// Links serialize as strings (URIs) in JSON Schema; the link
		// metadata (Target, LinkMode, ...) is preserved in the Type AST
		// for in-process consumers but not exposed in the compiled schema.
		out["type"] = "string"
		out["format"] = "contribution-uri"
		if t.Target != "" {
			out["description"] = fmt.Sprintf("URI of a %s Contribution", t.Target)
		}
	default:
		// A non-builtin Kind is a reference to another named type declared in
		// the schema's Types map. The DSL records such a field as a Type whose
		// Kind IS the target type name (the validator's checkKind admits it via
		// the known-user-type branch); here we compile it to a JSON Schema
		// $ref. An empty Kind is a genuine error.
		if t.Kind == "" {
			return nil, fmt.Errorf("empty TypeKind")
		}
		out["$ref"] = "#/$defs/" + string(t.Kind)
	}

	return out, nil
}

// compileRef compiles a TypeRef into a JSON Schema fragment. Name
// references emit `{"$ref": "#/$defs/<Name>"}`; inline types compile
// recursively.
func compileRef(r *TypeRef) (map[string]any, error) {
	if r == nil {
		return nil, fmt.Errorf("nil TypeRef")
	}
	if r.IsName() {
		return map[string]any{"$ref": "#/$defs/" + r.Name}, nil
	}
	if r.IsInline() {
		return r.Inline.compileJSONSchema()
	}
	return nil, fmt.Errorf("empty TypeRef")
}
