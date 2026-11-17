// SPDX-License-Identifier: Apache-2.0

package reflect

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestJSONRawMessage_IsOpenAny pins the systemic fix: a json.RawMessage field
// is an arbitrary-JSON payload and MUST schema-render as an open `any` ({}),
// NOT as a base64 byte string ({"type":"string","format":"byte"}). The byte
// rendering told LLMs/clients to send a STRINGIFIED body, which downstream
// decoders then silently dropped. json.RawMessage is []byte under the hood, so
// the guard must sit before the slice branch (a struct-only check never fires).
func TestJSONRawMessage_IsOpenAny(t *testing.T) {
	type withRaw struct {
		ID   string           `json:"id"`
		Body json.RawMessage  `json:"body,omitempty"`
		Ptr  *json.RawMessage `json:"ptr,omitempty"`
	}
	schema := MustJSONSchema[withRaw]()
	s := string(schema)
	if strings.Contains(s, `"byte"`) {
		t.Fatalf("json.RawMessage still rendered as a byte string:\n%s", s)
	}

	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"body", "ptr"} {
		got := strings.TrimSpace(string(doc.Properties[field]))
		// KindAny renders as an open schema (no "type") — must not be a string type.
		if strings.Contains(got, `"type":"string"`) || strings.Contains(got, `"type": "string"`) {
			t.Errorf("%s schema is a string, want open any: %s", field, got)
		}
	}
}
