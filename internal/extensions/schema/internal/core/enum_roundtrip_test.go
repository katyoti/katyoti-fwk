// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// TestHelloWorldEnumSurvivesSchemaRoundtrip pins the question that has
// been blocking the ms-schema-driven-ui US-S1 enum→Select work: does
// the `enum: [...]` on an inline object-field type make it all the way
// from the YAML source through `RegisterDataType` and out of
// `GetDataType` in the same shape display-lib's TypeScript renderer
// expects?
//
// The display-lib's text renderer reads `info.type.enum` (Schema struct
// → Types[Root].Fields[name].Enum). If this test passes, the AST is
// fine and the gap is on the TS side; if it fails, the parser /
// canonical encoder / store dropped enum on a non-root field somewhere.
func TestHelloWorldEnumSurvivesSchemaRoundtrip(t *testing.T) {
	body := []byte(`name: hello-world-input
root: HelloWorldInput
types:
  HelloWorldInput:
    type: object
    fields:
      name:
        type: string
        label: "Your name"
      greeting:
        type: string
        label: "Greeting style"
        default: "Hello"
        enum: ["Hello", "Hi", "Bonjour", "Hola", "Ciao"]
      loud:
        type: boolean
        default: false
`)

	ctx := context.Background()
	svc, err := New(ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// An opaque registry key: this test is about the AST surviving a
	// register/get round-trip, not about how a task's schema is addressed.
	// (Spelling the task-schema URI here would put a third writer of that
	// shape in the tree — see mbapi.TaskSchemaURI.)
	uri := "ktw-demo:/data-type/hello-world-input"
	if err := svc.RegisterDataType(ctx, api.RegisterDataTypeInput{URI: uri, Body: body}); err != nil {
		t.Fatalf("RegisterDataType: %v", err)
	}
	got, err := svc.GetDataType(ctx, api.GetDataTypeInput{URI: uri})
	if err != nil {
		t.Fatalf("GetDataType: %v", err)
	}
	if got.Schema == nil {
		t.Fatalf("GetDataType returned nil schema")
	}

	// Dump the AST so we can SEE what crosses the wire to display-lib.
	dump, err := got.Schema.MarshalCanonicalJSONIndent("  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	t.Logf("hello-world AST:\n%s", string(dump))

	// Walk to HelloWorldInput.fields.greeting and verify enum is present.
	rootType, ok := got.Schema.Types["HelloWorldInput"]
	if !ok {
		t.Fatalf("Types[HelloWorldInput] missing")
	}
	greeting, ok := rootType.Fields["greeting"]
	if !ok {
		t.Fatalf("HelloWorldInput.fields.greeting missing")
	}
	if greeting.Kind != "string" {
		t.Errorf("greeting.Kind = %q, want %q", greeting.Kind, "string")
	}
	if len(greeting.Enum) != 5 {
		t.Errorf("greeting.Enum has %d entries, want 5; values=%v", len(greeting.Enum), greeting.Enum)
	}

	// Confirm enum survives JSON-marshalling (the canonical form the GUI
	// receives on the wire) — re-decode and re-walk.
	raw, err := json.Marshal(got.Schema)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"enum"`) {
		t.Errorf("canonical JSON has no `enum` key:\n%s", string(raw))
	}
	if !strings.Contains(string(raw), `"Bonjour"`) {
		t.Errorf("canonical JSON missing 'Bonjour' enum value:\n%s", string(raw))
	}
}
