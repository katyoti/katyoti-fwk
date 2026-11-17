// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A ShapeFilter decodes through ObjectFilter and keeps its shape fields; a
// content field (when, incoming, outgoing) is refused naming it, in JSON and
// in YAML — what an object carries or must satisfy may not depend on what it
// holds. An unknown key is refused as ObjectFilter refuses it.
func TestShapeFilterRefusesContentFields(t *testing.T) {
	var f ShapeFilter
	if err := yaml.Unmarshal([]byte("types: [goal]\ncapabilities: [want]\n"), &f); err != nil || len(f.Types) != 1 || len(f.Capabilities) != 1 {
		t.Fatalf("types and capabilities must decode: %+v, %v", f, err)
	}
	for _, c := range []struct{ field, yamlSrc, jsonSrc string }{
		{"when", "types: [goal]\nwhen: \"true\"\n", `{"when":"true"}`},
		{"incoming", "incoming: [challenges]\n", `{"incoming":["challenges"]}`},
		{"outgoing", "outgoing: [serves]\n", `{"outgoing":["serves"]}`},
	} {
		if err := yaml.Unmarshal([]byte(c.yamlSrc), &f); err == nil || !strings.Contains(err.Error(), c.field+" refused") {
			t.Errorf("YAML %s: must be refused naming it, got %v", c.field, err)
		}
		if err := json.Unmarshal([]byte(c.jsonSrc), &f); err == nil || !strings.Contains(err.Error(), c.field+" refused") {
			t.Errorf("JSON %s: must be refused naming it, got %v", c.field, err)
		}
	}
	if err := yaml.Unmarshal([]byte("typs: [goal]\n"), &f); err == nil || !strings.Contains(err.Error(), "typs") {
		t.Errorf("an unknown key must be refused naming it, got %v", err)
	}
}

// Each selector moved onto the filter: a match, and a miss answering why.
func TestShapeSelectorsMatchAndSayWhyNot(t *testing.T) {
	// Capability attach: by type, by a capability carried before attachment.
	byType := Capability{Name: "memory", Attach: &ShapeFilter{Types: []string{"*"}}}
	if ok, why := byType.AttachesTo("goal", nil); !ok {
		t.Errorf("attach types [*] must attach to a goal: %s", why)
	}
	byCap := Capability{Name: "probe-log", Attach: &ShapeFilter{Capabilities: []string{"checkable"}}}
	if ok, why := byCap.AttachesTo("goal", []string{"checkable"}); !ok {
		t.Errorf("attach capabilities [checkable] must attach to a checkable object: %s", why)
	}
	if ok, why := byCap.AttachesTo("goal", []string{"want"}); ok || !strings.Contains(why, "carries none of [checkable]") {
		t.Errorf("attach must miss an object without checkable, saying so: %v %q", ok, why)
	}
	if ok, why := (Capability{Name: "tag"}).AttachesTo("goal", nil); ok || why == "" {
		t.Errorf("a capability without attach attaches to nothing, saying so: %v %q", ok, why)
	}

	// Policy applies: by types and capabilities; an empty applies applies to none.
	p := Policy{Name: "require-tests", Applies: ShapeFilter{Types: []string{"feature", "use-case"}}}
	if ok, why := p.AppliesTo("feature", nil); !ok {
		t.Errorf("policy must apply to a feature: %s", why)
	}
	if ok, why := p.AppliesTo("goal", nil); ok || !strings.Contains(why, `type "goal" is not one of [feature, use-case]`) {
		t.Errorf("policy must miss a goal, naming its type: %v %q", ok, why)
	}
	pc := Policy{Name: "checked", Applies: ShapeFilter{Capabilities: []string{"checkable"}}}
	if ok, why := pc.AppliesTo("goal", []string{"checkable"}); !ok {
		t.Errorf("policy by capability must apply to a checkable goal: %s", why)
	}
	if ok, why := pc.AppliesTo("goal", []string{"want"}); ok || !strings.Contains(why, "carries none of [checkable]") {
		t.Errorf("policy by capability must miss a goal without checkable, saying so: %v %q", ok, why)
	}
	if ok, why := (Policy{Name: "empty"}).AppliesTo("goal", nil); ok || !strings.Contains(why, "applies is empty") {
		t.Errorf("an empty applies applies to no object, saying so: %v %q", ok, why)
	}
}

// The old shapes are gone: a policy's applies with a content field is refused
// at decode, as a capability's attach is.
func TestPolicyAndAttachRefuseAContentFilter(t *testing.T) {
	var p Policy
	if err := yaml.Unmarshal([]byte("name: p\napplies: {types: [goal], when: \"true\"}\n"), &p); err == nil || !strings.Contains(err.Error(), "when refused") {
		t.Errorf("policy applies.when must be refused, got %v", err)
	}
	var c Capability
	if err := yaml.Unmarshal([]byte("name: c\nattach: {incoming: [challenges]}\n"), &c); err == nil || !strings.Contains(err.Error(), "incoming refused") {
		t.Errorf("capability attach.incoming must be refused, got %v", err)
	}
}
