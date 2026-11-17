// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"testing"

	yaml "gopkg.in/yaml.v3"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestObjectType_Actions_RoundTrip pins the YAML round-trip
// for the new `actions:` field added by cwm-v2-providers US-15
// (2026-06-08). Greenfield addition — no legacy alias.
func TestObjectType_Actions_RoundTrip(t *testing.T) {
	src := []byte(`
name: note
actions:
  - ktw-agents:tasks/note/promote
  - ktw-agents:tasks/note/abandon
`)
	var reg api.ObjectType
	if err := yaml.Unmarshal(src, &reg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if reg.Name != "note" {
		t.Errorf("Name = %q, want note", reg.Name)
	}
	want := []string{
		"ktw-agents:tasks/note/promote",
		"ktw-agents:tasks/note/abandon",
	}
	if len(reg.Actions) != len(want) {
		t.Fatalf("len(Actions) = %d, want %d (got %v)", len(reg.Actions), len(want), reg.Actions)
	}
	for i, a := range want {
		if reg.Actions[i].URI != a {
			t.Errorf("Actions[%d].URI = %q, want %q", i, reg.Actions[i].URI, a)
		}
	}
}

// TestObjectType_Actions_ObjectForm pins the dual spelling:
// an entry may be a bare URI string (shorthand) or an object carrying
// binding-site metadata (label / confirm / when / input).
func TestObjectType_Actions_ObjectForm(t *testing.T) {
	src := []byte(`
name: note
actions:
  - ktw-agents:tasks/note/promote
  - uri: ktw-agents:mutations/set-field
    label: Abandon
    confirm: true
    when: "object.fields.status != 'abandoned'"
    input:
      field: status
      value: abandoned
`)
	var reg api.ObjectType
	if err := yaml.Unmarshal(src, &reg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(reg.Actions) != 2 {
		t.Fatalf("len(Actions) = %d, want 2 (got %v)", len(reg.Actions), reg.Actions)
	}
	bare := reg.Actions[0]
	if bare.URI != "ktw-agents:tasks/note/promote" || bare.Label != "" || bare.Confirm {
		t.Errorf("bare entry = %+v, want URI-only shorthand", bare)
	}
	obj := reg.Actions[1]
	if obj.URI != "ktw-agents:mutations/set-field" {
		t.Errorf("obj.URI = %q", obj.URI)
	}
	if obj.Label != "Abandon" || !obj.Confirm {
		t.Errorf("obj label/confirm = %q/%v, want Abandon/true", obj.Label, obj.Confirm)
	}
	if obj.When != "object.fields.status != 'abandoned'" {
		t.Errorf("obj.When = %q", obj.When)
	}
	if obj.Input["field"] != "status" || obj.Input["value"] != "abandoned" {
		t.Errorf("obj.Input = %v", obj.Input)
	}
}

// TestObjectType_Actions_OmitWhenEmpty confirms the
// omitempty tag works — an item kind YAML that omits `actions:`
// gets a nil slice, not an empty one.
func TestObjectType_Actions_OmitWhenEmpty(t *testing.T) {
	src := []byte(`
name: rule
`)
	var reg api.ObjectType
	if err := yaml.Unmarshal(src, &reg); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if reg.Actions != nil {
		t.Errorf("Actions = %v, want nil (kind declared no actions)", reg.Actions)
	}
}
