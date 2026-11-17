// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"encoding/json"
	"testing"
)

// What ToJSONSchema writes, FromJSONSchema reads back: a nested object, an
// array of objects, an enum, a content type, optional fields, labels — so a
// method's input, which travels only as JSON Schema, renders as the same form.
func TestFromJSONSchemaReadsBackWhatToJSONSchemaWrites(t *testing.T) {
	str := func(content string) *Type { t := NewType(KindString); t.Content = content; return t }
	opt := func(t *Type) *Type { t.Optional = true; return t }
	addr := NewType(KindObject)
	addr.Fields = map[string]*Type{"city": str(""), "zip": opt(str(""))}
	line := NewType(KindObject)
	line.Fields = map[string]*Type{"label": str("")}
	lines := NewType(KindArray)
	lines.Items = RefInline(line)
	lines.Optional = true
	prio := NewType(KindString)
	prio.Enum = []any{"low", "high"}
	prio.Label = "Priority"
	root := NewType(KindObject)
	root.Fields = map[string]*Type{
		"title": str("markdown"), "address": addr, "lines": lines, "priority": prio,
		"count": opt(NewType(KindNumber)), "done": opt(NewType(KindBoolean)),
	}
	js, err := root.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	s, err := FromJSONSchema(js, "FileRequest")
	if err != nil {
		t.Fatal(err)
	}
	got := s.Types[s.Root]
	if s.Root != "FileRequest" || got == nil || got.Kind != KindObject {
		t.Fatalf("root = %q %+v", s.Root, got)
	}
	if f := got.Fields["title"]; f.Kind != KindString || f.Content != "markdown" || f.Optional {
		t.Errorf("title = %+v", f)
	}
	if f := got.Fields["address"]; f.Kind != KindObject || f.Fields["zip"] == nil || !f.Fields["zip"].Optional || f.Fields["city"].Optional {
		t.Errorf("address = %+v", f)
	}
	if f := got.Fields["lines"]; f.Kind != KindArray || !f.Optional || f.Items.Inline == nil || f.Items.Inline.Fields["label"] == nil {
		t.Errorf("lines = %+v", f)
	}
	if f := got.Fields["priority"]; len(f.Enum) != 2 || f.Label != "Priority" {
		t.Errorf("priority = %+v", f)
	}
	if !got.Fields["count"].Optional || got.Fields["count"].Kind != KindNumber {
		t.Errorf("count = %+v", got.Fields["count"])
	}
	// A schema rooted at a $ref to its $defs keeps the named types.
	named := &Schema{Name: "x", Root: "Order", Types: map[string]*Type{"Order": root}}
	js2, _ := named.ToJSONSchema()
	s2, err := FromJSONSchema(js2, "")
	if err != nil || s2.Root != "Order" || s2.Types["Order"].Fields["title"] == nil {
		t.Fatalf("$defs root: %v %+v", err, s2)
	}
	if _, err := FromJSONSchema(json.RawMessage(`{"$ref":"#/$defs/Nope"}`), ""); err == nil {
		t.Error("a root $ref to no definition must be refused")
	}
}
