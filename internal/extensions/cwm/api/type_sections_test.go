// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const sectionedType = `
name: decision
capabilities: [content]
context:
  propagation: inherit-from-ancestors
  briefing_fields: [topic]
pane: {}
`

// A type's sections are every top-level key no field takes, captured ONCE for
// both readers: the pack (YAML) and the catalog (JSON) see the same sections,
// and a type written back carries them inline, as in its file.
func TestTypeSectionsAreCapturedAndRoundTrip(t *testing.T) {
	var y ObjectType
	if err := yaml.Unmarshal([]byte(sectionedType), &y); err != nil {
		t.Fatal(err)
	}
	if y.Name != "decision" || len(y.Capabilities) != 1 {
		t.Fatalf("fields lost to the capture: %+v", y)
	}
	if len(y.Sections) != 2 || y.Sections["context"]["propagation"] != "inherit-from-ancestors" || y.Sections["pane"] == nil {
		t.Fatalf("sections = %#v", y.Sections)
	}
	b, err := json.Marshal(y)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"context":{`) || strings.Contains(string(b), `"Sections"`) {
		t.Fatalf("sections are not inline on the wire: %s", b)
	}
	var j ObjectType
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(j.Sections["context"]["briefing_fields"], []any{"topic"}) || j.Name != "decision" {
		t.Fatalf("catalog read lost the section: %#v", j)
	}
	// A type with no section is the struct alone.
	plain, _ := json.Marshal(ObjectType{Name: "x"})
	if string(plain) != `{"name":"x"}` {
		t.Errorf("plain type = %s", plain)
	}
}

// A top-level key that is no field and no map is a misspelt field, never a
// section: the declaration is refused, naming it.
func TestAScalarUnknownKeyIsRefused(t *testing.T) {
	var y ObjectType
	err := yaml.Unmarshal([]byte("name: x\nfoldr: goals\n"), &y)
	if err == nil || !strings.Contains(err.Error(), "foldr") {
		t.Fatalf("err = %v, want foldr named", err)
	}
	if err := json.Unmarshal([]byte(`{"name":"x","foldr":"goals"}`), &y); err == nil {
		t.Fatal("the catalog read accepted a scalar unknown key")
	}
}

// max_summary_chars is gone (declared, never read): a pack still carrying it
// is refused like any misspelt key, through YAML and JSON alike.
func TestTheRemovedMaxSummaryCharsIsRefused(t *testing.T) {
	var y ObjectType
	if err := yaml.Unmarshal([]byte("name: x\nmax_summary_chars: 300\n"), &y); err == nil || !strings.Contains(err.Error(), "max_summary_chars") {
		t.Fatalf("yaml: err = %v, want max_summary_chars named", err)
	}
	if err := json.Unmarshal([]byte(`{"name":"x","max_summary_chars":300}`), &y); err == nil {
		t.Fatal("json: the catalog read accepted max_summary_chars")
	}
}

// TypeSection is the one reader an extension decodes its section with; the
// section's capability is in force on the type.
func TestTypeSectionDecodes(t *testing.T) {
	var ot ObjectType
	if err := yaml.Unmarshal([]byte(sectionedType), &ot); err != nil {
		t.Fatal(err)
	}
	type ctxSettings struct {
		Propagation    string   `json:"propagation"`
		BriefingFields []string `json:"briefing_fields"`
	}
	got, ok, err := TypeSection[ctxSettings](ot, "context")
	if err != nil || !ok || got.Propagation != "inherit-from-ancestors" || len(got.BriefingFields) != 1 {
		t.Fatalf("TypeSection = %+v %v %v", got, ok, err)
	}
	if _, ok, _ := TypeSection[ctxSettings](ot, "verification"); ok {
		t.Error("a section the type does not write was found")
	}
	type wrong struct {
		Propagation int `json:"propagation"`
	}
	if _, _, err := TypeSection[wrong](ot, "context"); err == nil {
		t.Error("a section that does not decode was read without an error")
	}
	if got := ot.CapabilitiesInForce(); !reflect.DeepEqual(got, []string{"content", "context", "pane"}) {
		t.Errorf("in force = %v, want the list then each section's capability", got)
	}
}
