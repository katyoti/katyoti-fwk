// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestObjectFilterMatches — each field on its own, AND across fields, and the
// reason naming the field that refused (goal cwm.g12 AC1).
func TestObjectFilterMatches(t *testing.T) {
	yes := func(string) (bool, error) { return true, nil }
	no := func(string) (bool, error) { return false, nil }
	broken := func(string) (bool, error) { return false, errors.New("unknown name node") }
	subj := func(eval func(string) (bool, error)) FilterSubject {
		return FilterSubject{Type: "use-case", Capabilities: []string{"want", "checkable"}, Eval: eval}.WithEdges("m.u", []Edge{
			{From: "m.c", To: "m.u", Kind: "challenges"},
			{From: "m.u", To: "m.g", Kind: "serves"},
		})
	}
	for _, c := range []struct {
		name   string
		f      ObjectFilter
		eval   func(string) (bool, error)
		want   bool
		reason string
	}{
		{"zero filter admits everything", ObjectFilter{}, nil, true, ""},
		{"type listed", ObjectFilter{Types: []string{"goal", "use-case"}}, nil, true, ""},
		{"type wildcard", ObjectFilter{Types: []string{"*"}}, nil, true, ""},
		{"type refused", ObjectFilter{Types: []string{"goal"}}, nil, false, "type"},
		{"any capability", ObjectFilter{Capabilities: []string{"module", "checkable"}}, nil, true, ""},
		{"no capability", ObjectFilter{Capabilities: []string{"module"}}, nil, false, "carries none"},
		{"when true", ObjectFilter{When: "x"}, yes, true, ""},
		{"when false", ObjectFilter{When: "x"}, no, false, "when is false"},
		{"when unreadable refuses", ObjectFilter{When: "x"}, broken, false, "unknown name node"},
		{"AND: type ok, capability refuses", ObjectFilter{Types: []string{"*"}, Capabilities: []string{"module"}}, nil, false, "carries none"},
		{"incoming of a listed kind", ObjectFilter{Incoming: []string{"refines", "challenges"}}, nil, true, ""},
		{"no incoming of the kind", ObjectFilter{Incoming: []string{"refines"}}, nil, false, "no incoming edge of kind [refines]"},
		{"an outgoing edge is not incoming", ObjectFilter{Incoming: []string{"serves"}}, nil, false, "no incoming edge of kind [serves]"},
		{"outgoing of a listed kind", ObjectFilter{Outgoing: []string{"serves"}}, nil, true, ""},
		{"an incoming edge is not outgoing", ObjectFilter{Outgoing: []string{"challenges"}}, nil, false, "no outgoing edge of kind [challenges]"},
		{"AND: incoming ok, outgoing refuses", ObjectFilter{Incoming: []string{"challenges"}, Outgoing: []string{"refines"}}, nil, false, "outgoing"},
		{"AND: all ok, when refuses", ObjectFilter{Types: []string{"use-case"}, Capabilities: []string{"want"}, When: "x"}, no, false, "when"},
	} {
		got, reason := c.f.Matches(subj(c.eval))
		if got != c.want {
			t.Errorf("%s: matched=%v, want %v (%s)", c.name, got, c.want, reason)
		}
		if !c.want && !strings.Contains(reason, c.reason) {
			t.Errorf("%s: reason %q must name %q", c.name, reason, c.reason)
		}
	}
}

// TestObjectFilterCarriesOnlyItsFields — goal cwm.g12 AC2: types,
// capabilities, incoming, outgoing, when, and nothing else. requires (an
// include), aspects (a list of capabilities) and modules (a runtime name) were
// rejected by name; another field is a decision to revisit, not an edit to
// slip in.
func TestObjectFilterCarriesOnlyItsFields(t *testing.T) {
	typ := reflect.TypeOf(ObjectFilter{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		got = append(got, typ.Field(i).Name)
	}
	if strings.Join(got, ",") != "Types,Capabilities,Incoming,Outgoing,When" {
		t.Errorf("ObjectFilter fields = %v, want exactly Types, Capabilities, Incoming, Outgoing, When", got)
	}
}

// A filter refuses a key none of its fields takes, naming it, in JSON and in
// YAML — a misspelt `incomming:` would otherwise be an empty filter, admitting
// every object.
func TestObjectFilterRefusesAnUnknownKey(t *testing.T) {
	var f ObjectFilter
	if err := json.Unmarshal([]byte(`{"incoming":["challenges"],"outgoing":["serves"]}`), &f); err != nil || len(f.Incoming) != 1 || len(f.Outgoing) != 1 {
		t.Fatalf("the edge keys must decode: %+v, %v", f, err)
	}
	if err := json.Unmarshal([]byte(`{"incomming":["challenges"]}`), &f); err == nil || !strings.Contains(err.Error(), "incomming") {
		t.Errorf("JSON: an unknown key must be refused naming it, got %v", err)
	}
	if err := yaml.Unmarshal([]byte("types: [x]\nincomming: [challenges]\n"), &f); err == nil || !strings.Contains(err.Error(), "incomming") {
		t.Errorf("YAML: an unknown key must be refused naming it, got %v", err)
	}
}
