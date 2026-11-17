// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// MarshalCanonicalJSON emits the schema as deterministic, byte-stable JSON.
//
// Determinism rules:
//   - Object keys are emitted in alphabetical order
//   - Type.Kind is emitted as the JSON key "type"
//   - Empty / nil / zero fields are omitted (matches the json:"...,omitempty" tags)
//   - TypeRef marshals as a bare string when Name is set, else as the inline Type
//   - Maps (Fields, ContentParams) sort by key
//   - Arrays preserve their input order (the parser is responsible for any
//     normalization on the way in)
//   - Compact (no extra whitespace) — for byte-level snapshot comparison
//
// Used by corpus tests (Parse(input) → MarshalCanonicalJSON → assert == expected).
func (s *Schema) MarshalCanonicalJSON() ([]byte, error) {
	if s == nil {
		return []byte("null"), nil
	}
	v, err := canonicalize(s)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder appends a trailing newline; strip it for byte-stable output.
	out := buf.Bytes()
	if len(out) > 0 && out[len(out)-1] == '\n' {
		out = out[:len(out)-1]
	}
	return out, nil
}

// MarshalCanonicalJSONIndent is like MarshalCanonicalJSON but pretty-printed
// for human-readable corpus snapshots.
func (s *Schema) MarshalCanonicalJSONIndent(indent string) ([]byte, error) {
	raw, err := s.MarshalCanonicalJSON()
	if err != nil {
		return nil, err
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", indent); err != nil {
		return nil, err
	}
	return pretty.Bytes(), nil
}

// canonicalize converts a value into a form that produces deterministic JSON
// when round-tripped through encoding/json. It walks Schema, Type, TypeRef,
// and the standard map / slice / primitive carriers.
func canonicalize(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case *Schema:
		if x == nil {
			return nil, nil
		}
		return canonicalizeSchema(x)
	case *Type:
		if x == nil {
			return nil, nil
		}
		return canonicalizeType(x)
	case *TypeRef:
		if x == nil {
			return nil, nil
		}
		return canonicalizeRef(x)
	case Extension:
		return canonicalizeExtension(x)
	case map[string]any:
		return canonicalizeMap(x)
	case []any:
		return canonicalizeSlice(x)
	case map[string]*Type:
		return canonicalizeTypeMap(x)
	case []*TypeRef:
		out := make([]any, len(x))
		for i, ref := range x {
			cv, err := canonicalize(ref)
			if err != nil {
				return nil, err
			}
			out[i] = cv
		}
		return out, nil
	case []Extension:
		out := make([]any, len(x))
		for i, e := range x {
			cv, err := canonicalize(e)
			if err != nil {
				return nil, err
			}
			out[i] = cv
		}
		return out, nil
	default:
		// strings, numbers, bools, etc. — pass through unchanged.
		return x, nil
	}
}

// orderedMap is an array of key/value pairs that preserves insertion order
// when marshaled to JSON. Used to enforce alphabetical key ordering.
type orderedMap []orderedEntry
type orderedEntry struct {
	K string
	V any
}

func (m orderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, e := range m {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(e.K)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(e.V)
		if err != nil {
			return nil, fmt.Errorf("marshal key %q: %w", e.K, err)
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func canonicalizeSchema(s *Schema) (any, error) {
	out := orderedMap{}
	if s.DSLVersion != "" {
		out = append(out, orderedEntry{"$schema", s.DSLVersion})
	}
	if s.Description != "" {
		out = append(out, orderedEntry{"description", s.Description})
	}
	if len(s.Imports) > 0 {
		out = append(out, orderedEntry{"imports", s.Imports})
	}
	out = append(out, orderedEntry{"name", s.Name})
	if s.Root != "" {
		out = append(out, orderedEntry{"root", s.Root})
	}
	tm, err := canonicalizeTypeMap(s.Types)
	if err != nil {
		return nil, err
	}
	out = append(out, orderedEntry{"types", tm})
	if s.Version != "" {
		out = append(out, orderedEntry{"version", s.Version})
	}
	// re-sort by key (description/imports are between $schema and name alphabetically).
	sort.SliceStable(out, func(i, j int) bool { return out[i].K < out[j].K })
	return out, nil
}

func canonicalizeType(t *Type) (any, error) {
	if t == nil {
		return nil, nil
	}
	pairs := orderedMap{
		{"type", string(t.Kind)},
	}
	add := func(k string, v any) { pairs = append(pairs, orderedEntry{k, v}) }

	// --- meta ---
	if t.Label != "" {
		add("label", t.Label)
	}
	if t.Placeholder != "" {
		add("placeholder", t.Placeholder)
	}
	if t.Description != "" {
		add("description", t.Description)
	}
	if t.Informations != "" {
		add("informations", t.Informations)
	}
	if t.Optional {
		add("optional", true)
	}
	if t.Default != nil {
		add("default", t.Default)
	}
	if len(t.Visibility) > 0 {
		add("visibility", t.Visibility)
	}
	if t.Detail != 0 {
		add("detail", t.Detail)
	}
	if len(t.Alias) > 0 {
		add("alias", t.Alias)
	}
	if len(t.Extensions) > 0 {
		exts := make([]any, len(t.Extensions))
		for i, e := range t.Extensions {
			cv, err := canonicalizeExtension(e)
			if err != nil {
				return nil, err
			}
			exts[i] = cv
		}
		add("extensions", exts)
	}

	// --- primitives ---
	if t.Content != "" {
		add("content", t.Content)
	}
	if len(t.ContentParams) > 0 {
		cp, err := canonicalizeMap(t.ContentParams)
		if err != nil {
			return nil, err
		}
		add("contentParams", cp)
	}
	if len(t.Enum) > 0 {
		add("enum", t.Enum)
	}

	// --- object ---
	if len(t.Fields) > 0 {
		fm, err := canonicalizeTypeMap(t.Fields)
		if err != nil {
			return nil, err
		}
		add("fields", fm)
	}
	if len(t.IDField) > 0 {
		add("idField", t.IDField)
	}
	if len(t.TitleField) > 0 {
		add("titleField", t.TitleField)
	}
	if t.Catchall != nil {
		cv, err := canonicalizeRef(t.Catchall)
		if err != nil {
			return nil, err
		}
		add("catchall", cv)
	}
	if len(t.Extends) > 0 {
		add("extends", t.Extends)
	}

	// --- array ---
	if t.Items != nil {
		cv, err := canonicalizeRef(t.Items)
		if err != nil {
			return nil, err
		}
		add("items", cv)
	}
	if t.Min != nil {
		add("min", *t.Min)
	}
	if t.Max != nil {
		add("max", *t.Max)
	}

	// --- choice ---
	if len(t.Cases) > 0 {
		cs := make([]any, len(t.Cases))
		for i, c := range t.Cases {
			cv, err := canonicalizeRef(c)
			if err != nil {
				return nil, err
			}
			cs[i] = cv
		}
		add("cases", cs)
	}

	// --- link / tree ---
	if t.Target != "" {
		add("target", t.Target)
	}
	if t.LinkMode != "" {
		add("mode", string(t.LinkMode))
	}
	if t.LinkType != "" {
		add("linkType", t.LinkType)
	}
	if t.TargetID != "" {
		add("targetId", t.TargetID)
	}
	if t.Reverse != "" {
		add("reverse", t.Reverse)
	}
	if t.Display != "" {
		add("display", t.Display)
	}
	if t.AllowCreate != nil {
		add("allowCreate", *t.AllowCreate)
	}
	if t.Searchable != nil {
		add("searchable", *t.Searchable)
	}

	// --- tree extras ---
	if t.RootValue != nil {
		add("rootValue", t.RootValue)
	}
	if t.MaxDepth != 0 {
		add("maxDepth", t.MaxDepth)
	}
	if t.AllowCircular {
		add("allowCircular", true)
	}
	if t.ShowPath != nil {
		add("showPath", *t.ShowPath)
	}
	if t.DragDrop != nil {
		add("dragDrop", *t.DragDrop)
	}

	// alphabetize for byte-stable output regardless of insertion order.
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].K < pairs[j].K })
	return pairs, nil
}

func canonicalizeRef(r *TypeRef) (any, error) {
	if r == nil {
		return nil, nil
	}
	if r.IsName() {
		return r.Name, nil
	}
	return canonicalizeType(r.Inline)
}

func canonicalizeExtension(e Extension) (any, error) {
	pairs := orderedMap{{"type", e.Kind}}
	for _, k := range sortedKeys(e.Payload) {
		cv, err := canonicalize(e.Payload[k])
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, orderedEntry{k, cv})
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].K < pairs[j].K })
	return pairs, nil
}

func canonicalizeMap(m map[string]any) (any, error) {
	pairs := make(orderedMap, 0, len(m))
	for _, k := range sortedKeys(m) {
		cv, err := canonicalize(m[k])
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, orderedEntry{k, cv})
	}
	return pairs, nil
}

func canonicalizeTypeMap(m map[string]*Type) (any, error) {
	pairs := make(orderedMap, 0, len(m))
	for _, k := range sortedKeys(m) {
		cv, err := canonicalizeType(m[k])
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, orderedEntry{k, cv})
	}
	return pairs, nil
}

func canonicalizeSlice(s []any) (any, error) {
	out := make([]any, len(s))
	for i, e := range s {
		cv, err := canonicalize(e)
		if err != nil {
			return nil, err
		}
		out[i] = cv
	}
	return out, nil
}
