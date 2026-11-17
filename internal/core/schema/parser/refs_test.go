// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// A reference field — object-ref, panel-ref — is a string whose params are an
// ObjectFilter: the shorthand's `|` lists and the block's lists read the
// same; JSON Schema carries the format and the filter (x-filter); a wrong
// key, a wrong shape or a non-string is refused, naming it.
func TestReferenceContentTypes(t *testing.T) {
	dsl := `name: pick
root: In
types:
  In:
    type: object
    fields:
      owner: string@object-ref(types:person|team, capabilities:lifecycle)
      view: string@panel-ref(types:module)?
      related:
        type: string
        content: {type: object-ref, types: [decision], incoming: [supersedes], when: "state == 'accepted'"}
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(dsl)})
	if err != nil {
		t.Fatalf("parse: %v (%+v)", err, out.Diagnostics)
	}
	for _, d := range out.Diagnostics {
		if d.Code == "UNKNOWN_CONTENT" {
			t.Errorf("a reference is a known content type: %+v", d)
		}
	}
	f := out.Schema.Types["In"].Fields
	if got := f["owner"].ContentParams; f["owner"].Content != "object-ref" || len(got["types"].([]any)) != 2 || got["capabilities"].([]any)[0] != "lifecycle" {
		t.Errorf("owner = %+v", f["owner"])
	}
	if f["view"].Content != "panel-ref" || !f["view"].Optional {
		t.Errorf("view = %+v", f["view"])
	}
	if got := f["related"].ContentParams; got["when"] != "state == 'accepted'" || got["incoming"].([]any)[0] != "supersedes" {
		t.Errorf("related = %+v", f["related"])
	}
	js, err := out.Schema.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), `"format":"object-ref"`) || !strings.Contains(string(js), `"x-filter":{"capabilities":["lifecycle"],"types":["person","team"]}`) {
		t.Errorf("JSON Schema: %s", js)
	}
	// Read back (a method's input): the filter comes back as the params.
	back, err := schemaapi.FromJSONSchema(js, "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(back.Types[back.Root].Fields["owner"])
	if !strings.Contains(string(b), `"content":"object-ref"`) || !strings.Contains(string(b), `"types":["person","team"]`) {
		t.Errorf("read back: %s", b)
	}

	for name, tc := range map[string]struct{ dsl, want string }{
		"unknown key":  {"owner: string@object-ref(kinds:person)", "kinds is no filter of a reference"},
		"not a string": {"n: number@object-ref(types:person)", "it goes on a string"},
		"bad when":     {"o: {type: string, content: {type: object-ref, when: [x]}}", "when must be an el predicate"},
	} {
		t.Run(name, func(t *testing.T) {
			body := "name: x\nroot: In\ntypes:\n  In:\n    type: object\n    fields:\n      " + tc.dsl + "\n"
			_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body)})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("want %q, got %v", tc.want, err)
			}
		})
	}
}
