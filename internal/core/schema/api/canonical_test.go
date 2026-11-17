// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestCanonical_ByteStable verifies that a schema marshals to the exact same
// bytes across repeated invocations, regardless of map iteration order.
func TestCanonical_ByteStable(t *testing.T) {
	s := newSampleSchema()
	first, err := s.MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("first marshal: %v", err)
	}
	for i := 0; i < 100; i++ {
		got, err := s.MarshalCanonicalJSON()
		if err != nil {
			t.Fatalf("iter %d marshal: %v", i, err)
		}
		if !bytes.Equal(first, got) {
			t.Fatalf("iter %d: byte drift\nfirst: %s\ngot:   %s", i, first, got)
		}
	}
}

// TestCanonical_AlphaSortedKeys verifies object keys are emitted alphabetically.
func TestCanonical_AlphaSortedKeys(t *testing.T) {
	s := NewSchema("alpha")
	s.Description = "x"
	s.Version = "1.0"
	raw, err := s.MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// keys: $schema, description, name, types, version — that's the alpha order.
	want := []string{`"$schema"`, `"description"`, `"name"`, `"types"`, `"version"`}
	pos := 0
	for _, k := range want {
		idx := bytes.Index(raw[pos:], []byte(k))
		if idx < 0 {
			t.Fatalf("key %s not found at/after pos %d in %s", k, pos, raw)
		}
		pos += idx + len(k)
	}
}

// TestCanonical_TypeRefAsString verifies a name TypeRef marshals as a bare string.
func TestCanonical_TypeRefAsString(t *testing.T) {
	s := NewSchema("ref-test")
	user := NewType(KindObject)
	user.Fields = map[string]*Type{
		"manager": {Kind: KindLink, Target: "User", LinkMode: LinkMany2One},
	}
	post := NewType(KindObject)
	post.Fields = map[string]*Type{
		"author": {Kind: KindArray, Items: RefName("User")},
	}
	s.Types["User"] = user
	s.Types["Post"] = post

	raw, err := s.MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// TypeRef name must serialize as a bare string "User"
	if !bytes.Contains(raw, []byte(`"items":"User"`)) {
		t.Fatalf("expected items:\"User\" bare-string ref, got: %s", raw)
	}
}

// TestCanonical_ParsesBackToValidJSON verifies output is itself valid JSON.
func TestCanonical_ParsesBackToValidJSON(t *testing.T) {
	s := newSampleSchema()
	raw, err := s.MarshalCanonicalJSON()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("output not valid JSON: %v\noutput: %s", err, raw)
	}
}

// TestCanonical_OmitsEmpty verifies zero-valued fields are not emitted.
func TestCanonical_OmitsEmpty(t *testing.T) {
	t1 := &Type{Kind: KindString}
	out, err := canonicalize(t1)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// only the discriminator should appear
	if string(raw) != `{"type":"string"}` {
		t.Fatalf("expected minimal output, got %s", raw)
	}
}

func newSampleSchema() *Schema {
	s := NewSchema("sample")
	s.Version = "0.1.0"
	s.Description = "sample for tests"

	maxLen := 100.0
	emailType := &Type{
		Kind:    KindString,
		Content: "email",
		Max:     &maxLen,
		Label:   "Email",
	}
	nameType := &Type{Kind: KindString, Content: "text"}
	user := &Type{
		Kind:       KindObject,
		IDField:    []string{"id"},
		TitleField: []string{"name"},
		Fields: map[string]*Type{
			"id":    {Kind: KindString, Content: "uuid"},
			"email": emailType,
			"name":  nameType,
			"tags":  {Kind: KindArray, Items: RefInline(&Type{Kind: KindString})},
			"manager": {
				Kind:     KindLink,
				Target:   "User",
				LinkMode: LinkMany2One,
			},
		},
	}
	s.Types["User"] = user
	return s
}
