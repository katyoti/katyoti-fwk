// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

// The planning marks are known content types — status
// object-id, priority, stage on a string; progress-segments, gate-dots,
// stepper on an array — and a `slots` ui-view and a compact ui-table keep
// their whole payload (slots with a dotted path and actions, select, menu).
func TestPlanningContentTypesAndExtensions(t *testing.T) {
	dsl := `name: rows
root: Row
types:
  Row:
    type: object
    extensions:
      - type: ui-table
        name: default
        density: compact
        header: false
        select: increment
        columns: [{id: priority, width: 48}, {id: title}]
        menu:
          - {id: priority, label: Priority, uri: "ktw-agents:requests/assess", args: {id: "${item.id}", priority: "${value}"}, choice: "P0|P1|P2|P3"}
          - {id: hold, label: "Put on hold", uri: "ktw-agents:increments/hold", prompt: "Why does it wait?"}
      - type: ui-view
        name: header
        layout: slots
        slots:
          title: title
          badges: [stage]
          footer: [steps, release.gates]
          actions:
            - {id: mode, label: Release mode, uri: "ktw-agents:streams/set", choice: "manual|periodic", value: mode}
    fields:
      id: {type: string, content: object-id}
      title: string
      mode: string
      standing: {type: string, content: status}
      priority: {type: string, content: priority}
      stage: {type: string, content: stage}
      needs: {type: string, content: alert}
      score: "number@rating(half)"
      progress: {type: array, items: number, content: progress-segments}
      gates:
        type: array
        content: gate-dots
        items: {type: object, fields: {name: string, passed: "boolean?", why: "string?"}}
      steps:
        type: array
        content: stepper
        items: {type: object, fields: {name: string, state: string}}
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(dsl)})
	if err != nil {
		t.Fatalf("parse: %v (%+v)", err, out.Diagnostics)
	}
	for _, d := range out.Diagnostics {
		if d.Code == "UNKNOWN_CONTENT" || d.Code == "CONTENT_NOT_ALLOWED" {
			t.Errorf("a planning mark is a known content type: %+v", d)
		}
	}
	raw, err := json.Marshal(out.Schema.Types["Row"].Extensions)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"density":"compact"`, `"header":false`, `"select":"increment"`, `"choice":"P0|P1|P2|P3"`,
		`"prompt":"Why does it wait?"`, `"layout":"slots"`, `"release.gates"`, `"value":"mode"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("extensions lost %s: %s", want, raw)
		}
	}
}

// A bare inline param is a flag: `number@rating(half)` carries half: true;
// a malformed one is still refused.
func TestInlineParamFlag(t *testing.T) {
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte("name: f\nroot: R\ntypes:\n  R:\n    type: object\n    fields:\n      score: \"number@rating(half, max:10)\"\n")})
	if err != nil {
		t.Fatalf("parse: %v (%+v)", err, out.Diagnostics)
	}
	p := out.Schema.Types["R"].Fields["score"].ContentParams
	if p["half"] != true || p["max"] != int64(10) {
		t.Errorf("params = %#v", p)
	}
	if _, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte("name: f\nroot: R\ntypes:\n  R:\n    type: object\n    fields:\n      score: \"number@rating(a b)\"\n")}); err == nil {
		t.Error("a param that is neither key:value nor a name must be refused")
	}
}
