// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

// loadSchema compiles the embedded LogicalDiagram schema. Shared across
// the positive + negative tests.
func loadSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("logical_diagram.schema.json", bytes.NewReader(LogicalDiagramSchema())); err != nil {
		t.Fatalf("add resource: %v", err)
	}
	schema, err := compiler.Compile("logical_diagram.schema.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return schema
}

// validate parses + validates a JSON document against the schema.
// Returns nil on success, the validation error otherwise.
func validate(t *testing.T, schema *jsonschema.Schema, raw string) error {
	t.Helper()
	var instance any
	if err := json.Unmarshal([]byte(raw), &instance); err != nil {
		return err
	}
	return schema.Validate(instance)
}

// TestSchema_LoadsCompiles asserts the embedded schema itself is well-formed
// + compiles via santhosh-tekuri/jsonschema (the validator used elsewhere
// in the codebase, e.g. displaybridge).
func TestSchema_LoadsCompiles(t *testing.T) {
	if got := LogicalDiagramSchema(); len(got) == 0 {
		t.Fatal("embedded schema is empty")
	}
	_ = loadSchema(t) // panics via t.Fatalf if it doesn't compile
}

// TestSchema_PositiveCases — three canonical LogicalDiagram shapes drawn
// from Impl-B's golden fixtures (app-flux / uml-class / archimate-actor),
// adapted to the top-level LogicalDiagram envelope.
func TestSchema_PositiveCases(t *testing.T) {
	schema := loadSchema(t)
	cases := map[string]string{
		"app-flux-with-xls": `{
			"viewpoint": "drawio-test:/viewpoint/app-flux",
			"title": "App flux smoke",
			"nodes": [{
				"id": "billing", "label": "Billing", "kind": "Application",
				"attributes": {"domain":"source","markers":["xls"],"description":"Invoices and prices"}
			}]
		}`,
		"uml-class": `{
			"viewpoint": "ktw-docs:/viewpoint/uml2",
			"nodes": [{
				"id": "user", "label": "User", "kind": "Class",
				"attributes": {"attributes":["+name: string"],"operations":["+login()"]}
			}]
		}`,
		"archimate-actor": `{
			"viewpoint": "ktw-docs:/viewpoint/archimate3",
			"nodes": [{"id": "n1", "label": "Customer", "kind": "BusinessActor"}]
		}`,
		"with-zones-and-edges": `{
			"viewpoint": "drawio-test:/viewpoint/app-flux",
			"nodes": [
				{"id":"a", "label":"A", "kind":"Application", "zone":"z1"},
				{"id":"b", "label":"B", "kind":"Application", "zone":"z2"}
			],
			"edges": [{"id":"e1", "source":"a", "target":"b", "kind":"Flow"}],
			"zones": [{"id":"z1","label":"Sources","order":0}, {"id":"z2","label":"Sinks","order":1}],
			"layoutHints": {"preset":"archi-layered"}
		}`,
		"layout-algorithm-form": `{
			"viewpoint": "drawio-test:/viewpoint/app-flux",
			"nodes": [{"id":"n1","label":"A","kind":"Application"}],
			"layoutHints": {"algorithm":"layered","direction":"RIGHT","elkOptions":{"elk.spacing.nodeNode":"120"}}
		}`,
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if err := validate(t, schema, raw); err != nil {
				t.Errorf("expected valid, got: %v", err)
			}
		})
	}
}

// TestSchema_NegativeCases — each rejection case targets one error class.
type negCase struct {
	name      string
	doc       string
	mustError string // substring expected in the validation error
}

func TestSchema_NegativeCases(t *testing.T) {
	schema := loadSchema(t)
	cases := []negCase{
		{
			"missing viewpoint",
			`{"nodes":[{"id":"n1","label":"A","kind":"X"}]}`,
			"viewpoint",
		},
		{
			"bare shortname viewpoint — pattern mismatch",
			`{"viewpoint":"archimate3","nodes":[{"id":"n1","label":"A","kind":"X"}]}`,
			"pattern",
		},
		{
			"empty nodes — minItems violation",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[]}`,
			"minItems",
		},
		{
			"node missing kind",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[{"id":"n1","label":"A"}]}`,
			"kind",
		},
		{
			"edge missing source",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[{"id":"n1","label":"A","kind":"X"}],"edges":[{"id":"e1","target":"n1","kind":"F"}]}`,
			"source",
		},
		{
			"unknown top-level field",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[{"id":"n1","label":"A","kind":"X"}],"badField":"x"}`,
			"badField",
		},
		{
			"invalid layoutHints algorithm enum",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[{"id":"n1","label":"A","kind":"X"}],"layoutHints":{"algorithm":"orthogonal"}}`,
			"enum",
		},
		{
			"node id pattern violation (space)",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[{"id":"bad id","label":"A","kind":"X"}]}`,
			"pattern",
		},
		{
			"zone order negative",
			`{"viewpoint": "ktw-docs:/viewpoint/x","nodes":[{"id":"n1","label":"A","kind":"X"}],"zones":[{"id":"z1","label":"Z","order":-1}]}`,
			"minimum",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validate(t, schema, c.doc)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if c.mustError != "" && !strings.Contains(err.Error(), c.mustError) {
				t.Errorf("error message missing %q\n--- got ---\n%s", c.mustError, err.Error())
			}
		})
	}
}

// TestSchema_MatchesGoTypes: the schema's objects and the Go types they
// describe carry the same fields — a field the renderer reads is one the
// schema lets through, and the schema offers no field the renderer drops.
func TestSchema_MatchesGoTypes(t *testing.T) {
	var doc struct {
		Properties map[string]any `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]any `json:"properties"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(LogicalDiagramSchema(), &doc); err != nil {
		t.Fatal(err)
	}
	check := func(name string, props map[string]any, v any) {
		t.Helper()
		want := map[string]bool{}
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
			if tag != "" && tag != "-" {
				want[tag] = true
			}
		}
		for k := range props {
			if !want[k] {
				t.Errorf("%s: the schema offers %q, which %s does not carry", name, k, rt.Name())
			}
		}
		for k := range want {
			if _, ok := props[k]; !ok {
				t.Errorf("%s: %s carries %q, which the schema refuses", name, rt.Name(), k)
			}
		}
	}
	check("root", doc.Properties, LogicalDiagram{})
	check("Node", doc.Defs["Node"].Properties, LogicalNode{})
	check("Edge", doc.Defs["Edge"].Properties, LogicalEdge{})
	check("Zone", doc.Defs["Zone"].Properties, LogicalZone{})
	check("LayoutHints", doc.Defs["LayoutHints"].Properties, LayoutHints{})
	check("Page", doc.Defs["Page"].Properties, PageRef{})
}
