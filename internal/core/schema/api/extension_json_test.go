// SPDX-License-Identifier: Apache-2.0

package schemaapi_test

import (
	"encoding/json"
	"reflect"
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// A schema's ui-form survives a plain JSON round trip — what a KTSP answer
// (the hub's GetDataTypeWithExtensions) and the GUI's re-marshal do. It used
// to arrive as {"type":"ui-form"} alone: no layout, no groups, no labels.
func TestExtensionSurvivesJSON(t *testing.T) {
	s := &api.Schema{Name: "f", Root: "In", Types: map[string]*api.Type{
		"In": {Kind: api.KindObject, Extensions: []api.Extension{{
			Kind: "ui-form",
			Payload: map[string]any{
				"name": "default", "layout": "tabs", "submit_label": "Créer",
				"groups": []any{map[string]any{"title": "Essentiel", "fields": []any{"subject"}}},
			},
		}}},
	}}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back api.Schema
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	got := back.Types["In"].Extensions
	if len(got) != 1 || got[0].Kind != "ui-form" || !reflect.DeepEqual(got[0].Payload, s.Types["In"].Extensions[0].Payload) {
		t.Fatalf("round trip lost the extension: %+v\n%s", got, b)
	}

	// The plain form is the canonical one: same keys, flat.
	canon, err := s.MarshalCanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var fromCanon api.Schema
	if err := json.Unmarshal(canon, &fromCanon); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromCanon.Types["In"].Extensions[0].Payload, s.Types["In"].Extensions[0].Payload) {
		t.Fatalf("canonical JSON does not read back: %+v", fromCanon.Types["In"].Extensions)
	}

	if err := json.Unmarshal([]byte(`{"layout":"tabs"}`), &api.Extension{}); err == nil {
		t.Fatal("an extension without a type must be refused")
	}
}
