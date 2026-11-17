// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"

	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
)

// TestActionService_Describe_Mutation pins the mutations/ slot
// inspector: set-field describes with a flat input schema (the
// auto-generated parameter dialog consumes it) and IsAsync=false.
func TestActionService_Describe_Mutation(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	out, err := svc.Describe(context.Background(), actionsapi.DescribeInput{
		URI: "ktw-agents:mutations/set-field",
	})
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if out.IsAsync {
		t.Error("mutations are synchronous; IsAsync = true")
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(out.InputSchema, &schema); err != nil {
		t.Fatalf("InputSchema not valid JSON: %v", err)
	}
	for _, p := range []string{"node_id", "field", "value"} {
		if _, ok := schema.Properties[p]; !ok {
			t.Errorf("InputSchema missing property %q", p)
		}
	}

	if _, err := svc.Describe(context.Background(), actionsapi.DescribeInput{
		URI: "ktw-agents:mutations/no-such-mutation",
	}); err == nil {
		t.Error("unknown mutation described without error")
	}
}

// TestActionService_Run_Mutation_Validation pins the guard rails:
// required fields, the mutable-field allowlist, and per-field value
// typing all reject BEFORE any substrate access.
func TestActionService_Run_Mutation_Validation(t *testing.T) {
	svc, _ := New(context.Background(), &Config{})
	run := func(input string) error {
		_, err := svc.Run(context.Background(), actionsapi.RunInput{
			URI:   "ktw-agents:mutations/set-field",
			Input: json.RawMessage(input),
		})
		return err
	}

	cases := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"missing node_id", `{"field":"name","value":"x"}`, "node_id (or id) and field are required"},
		{"missing field", `{"node_id":"m.n1","value":"x"}`, "node_id (or id) and field are required"},
		{"identity field rejected", `{"node_id":"m.n1","field":"id","value":"x"}`, "not mutable"},
		{"structure field rejected", `{"node_id":"m.n1","field":"parent","value":"x"}`, "structure"},
		{"retired group rejected", `{"node_id":"m.n1","field":"container.name","value":"x"}`, "own fields"},
		{"type field rejected", `{"node_id":"m.n1","field":"type","value":"x"}`, "not mutable"},
		{"name needs string", `{"node_id":"m.n1","field":"name","value":7}`, "requires a string"},
		{"tags needs list", `{"node_id":"m.n1","field":"tags","value":"oops"}`, "requires a string list"},
		{"tags entries strings", `{"node_id":"m.n1","field":"tags","value":["a",1]}`, "must be strings"},
		{"bare fields rejected", `{"node_id":"m.n1","field":"fields.","value":1}`, "not mutable"},
		{"body needs string", `{"node_id":"m.n1","field":"body","value":7}`, "requires a string"},
		{"bare capability rejected", `{"node_id":"m.n1","field":"module.","value":1}`, "not mutable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

// TestSetObjectField — the object vocabulary: an own field, a capability field,
// one of the type's fields, null deletes, the body is the core's prose.
func TestSetObjectField(t *testing.T) {
	obj := cwmapi.Object{ID: "m.x", Type: "doc"}
	obj.Name = "X"
	obj.SetFieldsJSON(json.RawMessage(`{"review_state":"pending"}`))
	for _, c := range []struct {
		field string
		value any
	}{{"name", "Renamed"}, {"fields.review_state", "in-review"}, {"module.decisions", "open"}} {
		if err := setObjectField(&obj, c.field, c.value); err != nil {
			t.Fatalf("%s: %v", c.field, err)
		}
	}
	if obj.Name != "Renamed" || obj.Capabilities["module"]["decisions"] != "open" {
		t.Errorf("capability fields: %v", obj.Capabilities)
	}
	ct := obj
	if !strings.Contains(string(ct.FieldsJSON()), `"in-review"`) {
		t.Errorf("fields key: %s", ct.FieldsJSON())
	}
	if err := setObjectField(&obj, "fields.review_state", nil); err != nil {
		t.Fatal(err)
	}
	ct = obj
	if strings.Contains(string(ct.FieldsJSON()), "review_state") {
		t.Errorf("null must delete the key: %s", ct.FieldsJSON())
	}
	prose := cwmapi.Object{ID: "m.p", Type: "note"}
	if err := setObjectField(&prose, "body", "some prose"); err != nil || prose.Body != "some prose" {
		t.Errorf("the body is the core's prose: %v %q", err, prose.Body)
	}
}

// TestSetFieldRefusesTheLifecycle: a state changes only through a transition,
// which checks the workflow and its gate — set-field cannot walk around it.
func TestSetFieldRefusesTheLifecycle(t *testing.T) {
	for _, f := range []string{"lifecycle.state", "lifecycle.history", "lifecycle.workflow"} {
		if _, err := normalizeSetField(f, "accepted"); err == nil {
			t.Errorf("set-field %s accepted; a lifecycle changes only through ObjectTransition", f)
		}
	}
}
