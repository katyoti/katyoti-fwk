// SPDX-License-Identifier: Apache-2.0

package schemaapi

import (
	"encoding/json"
	"testing"
)

// jsonschema_determinism_test.go — the same type compiles to the same bytes.
//
// A Type's Fields are a map, and Go randomises map iteration order per run. Any
// array built by ranging over it therefore came out differently each time, so
// the SAME type compiled twice produced different bytes — and anything that
// hashes a derived schema (a content address, a cache key, a golden file)
// churned for no reason at all.
//
// `properties` was never affected: encoding/json sorts map keys. `required` is
// an array, and arrays keep whatever order they were built in.

// manyFields has enough required fields that random ordering shows up almost
// immediately — with five, two consecutive runs agree by chance about 1 time in
// 120.
func manyFields() *Type {
	req := func(k TypeKind) *Type { return &Type{Kind: k} }
	return &Type{
		Kind: KindObject,
		Fields: map[string]*Type{
			"alpha":   req(KindString),
			"beta":    req(KindNumber),
			"gamma":   req(KindBoolean),
			"delta":   req(KindString),
			"epsilon": req(KindNumber),
		},
	}
}

// TestToJSONSchema_IsDeterministic compiles one type repeatedly and asserts
// every result is byte-identical.
func TestToJSONSchema_IsDeterministic(t *testing.T) {
	first, err := manyFields().ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		again, err := manyFields().ToJSONSchema()
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(first) {
			t.Fatalf("compile %d produced different bytes:\n first = %s\n again = %s", i, first, again)
		}
	}
}

// TestToJSONSchema_RequiredIsSorted pins the reason it is stable, so a future
// change that keeps determinism by accident does not look like it kept it on
// purpose.
func TestToJSONSchema_RequiredIsSorted(t *testing.T) {
	raw, err := manyFields().ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha", "beta", "delta", "epsilon", "gamma"}
	if len(doc.Required) != len(want) {
		t.Fatalf("required = %v, want %v", doc.Required, want)
	}
	for i := range want {
		if doc.Required[i] != want[i] {
			t.Fatalf("required = %v, want %v", doc.Required, want)
		}
	}
}
