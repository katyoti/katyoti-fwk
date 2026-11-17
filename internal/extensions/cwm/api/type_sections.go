// SPDX-License-Identifier: Apache-2.0

package api

// TYPE SECTIONS — a capability's settings on a type.
//
// A capability has three levels, as a strategy does: the capability (declared
// by the extension that gives it meaning), its SECTION on a type, its GROUP on
// an object. A type file stays one file, its settings grouped by the
// capability that owns them:
//
//	name: decision
//	capabilities: [content, lifecycle]
//	context:
//	  propagation: inherit-from-ancestors
//	  resolution_strategy: closest-wins
//
// A section is a top-level key of the declaration that is not one of the
// type's own fields, named after its capability. It is decoded here, ONCE, for
// every reader — the pack loader (YAML) and the catalog (JSON) — and written
// back inline, so a type on the wire looks like the file it came from.
//
// CWM keeps the section as written and checks it against the capability's
// `type_schema`; the extension reads it with TypeSection. CWM gives it no
// meaning.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// objectTypeFields is ObjectType without its codec — what the fields decode
// into before the sections are captured.
type objectTypeFields ObjectType

// objectTypeKeys are the top-level keys ObjectType's own fields take; every
// other key is a section. The key set is every declaration's (declaration_keys.go).
func objectTypeKeys() map[string]bool {
	return declarationKeys(reflect.TypeOf(objectTypeFields{}), "json")
}

// captureSections keeps every key of a decoded declaration that is not a field
// of the type as a section. A section is a map (an empty key is an empty
// section); any other value is refused — a misspelt field is a key no field
// takes, and taking it silently would make the typo a setting nobody reads.
func captureSections(name string, all map[string]any) (map[string]map[string]any, error) {
	known := objectTypeKeys()
	var out map[string]map[string]any
	var bad []string
	for k, v := range all {
		if known[k] {
			continue
		}
		if out == nil {
			out = map[string]map[string]any{}
		}
		switch m := v.(type) {
		case nil:
			out[k] = map[string]any{}
		case map[string]any:
			out[k] = m
		default:
			bad = append(bad, k)
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		return nil, fmt.Errorf("object type %q: %s is no field of a type, and a capability's section is a map named after the capability", name, strings.Join(bad, ", "))
	}
	return out, nil
}

// UnmarshalJSON decodes the fields, then captures the sections.
func (t *ObjectType) UnmarshalJSON(b []byte) error {
	var f objectTypeFields
	if err := json.Unmarshal(b, &f); err != nil {
		return err
	}
	var all map[string]any
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	sections, err := captureSections(f.Name, all)
	if err != nil {
		return err
	}
	*t = ObjectType(f)
	t.Sections = sections
	return nil
}

// UnmarshalYAML decodes the fields, then captures the sections.
func (t *ObjectType) UnmarshalYAML(n *yaml.Node) error {
	var f objectTypeFields
	if err := n.Decode(&f); err != nil {
		return err
	}
	var all map[string]any
	if err := n.Decode(&all); err != nil {
		return err
	}
	sections, err := captureSections(f.Name, all)
	if err != nil {
		return err
	}
	*t = ObjectType(f)
	t.Sections = sections
	return nil
}

// MarshalJSON writes the sections inline, beside the fields.
func (t ObjectType) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(objectTypeFields(t))
	if err != nil || len(t.Sections) == 0 {
		return b, err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	for k, v := range t.Sections {
		if objectTypeKeys()[k] {
			return nil, fmt.Errorf("object type %q: section %q has the name of a field", t.Name, k)
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("object type %q: section %q: %w", t.Name, k, err)
		}
		all[k] = raw
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(all); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// MarshalYAML writes the sections inline, beside the fields.
func (t ObjectType) MarshalYAML() (any, error) {
	b, err := t.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var all map[string]any
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// CapabilitiesInForce is every capability the type puts on its objects: its
// list, then the capability of each section it writes (sorted), once each.
func (t ObjectType) CapabilitiesInForce() []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range t.Capabilities {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	names := make([]string, 0, len(t.Sections))
	for k := range t.Sections {
		if !seen[k] {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	return append(out, names...)
}

// TypeSection decodes the type's section of one capability into T — the ONE
// reader an extension uses for its settings on a type. ok is false when the
// type writes no such section; an error says the section does not decode.
func TypeSection[T any](t ObjectType, capability string) (T, bool, error) {
	var v T
	sec, ok := t.Sections[capability]
	if !ok {
		return v, false, nil
	}
	raw, err := json.Marshal(sec)
	if err != nil {
		return v, true, fmt.Errorf("object type %q: section %q: %w", t.Name, capability, err)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, true, fmt.Errorf("object type %q: section %q: %w", t.Name, capability, err)
	}
	return v, true, nil
}
