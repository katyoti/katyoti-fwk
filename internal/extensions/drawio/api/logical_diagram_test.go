// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestLogicalDiagram_RoundTrip asserts the canonical wire shape — a
// realistic app-flux-flavoured LogicalDiagram marshals to JSON + unmarshals
// back deep-equal. Catches accidental JSON-tag drift.
func TestLogicalDiagram_RoundTrip(t *testing.T) {
	orig := LogicalDiagram{
		Viewpoint: "drawio-test:/viewpoint/app-flux",
		Title:     "App flux order intake",
		Nodes: []LogicalNode{
			{
				ID: "billing", Label: "Billing", Kind: "Application",
				Attributes: map[string]any{
					"domain":             "source",
					"markers":            []any{"xls"},
					"description":        "Invoices and prices",
					"external_url":       "https://catalog.example.com/applications/billing",
					"external_url_label": "Catalog",
				},
				Zone: "z-sources",
			},
			{
				ID: "ledger", Label: "Ledger", Kind: "Application",
				Attributes: map[string]any{"domain": "accounting"},
				Zone:       "z-accounting",
			},
		},
		Edges: []LogicalEdge{
			{
				ID: "e1", Source: "billing", Target: "ledger", Kind: "Flow",
				Attributes: map[string]any{
					"flow_type":  "invoice",
					"data_types": []any{"Facture", "Ligne"},
				},
			},
		},
		Zones: []LogicalZone{
			{ID: "z-sources", Label: "Sources", Order: 0},
			{ID: "z-accounting", Label: "Accounting", Order: 1},
		},
		LayoutHints: &LayoutHints{Algorithm: LayoutLayered, Direction: "RIGHT"},
	}

	raw, err := json.Marshal(&orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Spot-check the JSON tag rename (kind/attributes — not type/data/factsheet).
	s := string(raw)
	for _, want := range []string{
		`"viewpoint":"drawio-test:/viewpoint/app-flux"`,
		`"kind":"Application"`,
		`"attributes":{`,
	} {
		if !contains(s, want) {
			t.Errorf("JSON missing %q (got: %s)", want, s)
		}
	}
	for _, banned := range []string{
		`"type":"Application"`, // would mean we forgot to use Kind
		`"data":{`,             // would mean we forgot to rename
		`"factsheet":{`,        // would mean we leaked the ArchGraph shape
	} {
		if contains(s, banned) {
			t.Errorf("JSON contains banned token %q (got: %s)", banned, s)
		}
	}

	var back LogicalDiagram
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(&orig, &back) {
		t.Errorf("round-trip diverged\norig: %+v\nback: %+v", orig, back)
	}
}

// TestLogicalNode_ResolverNodeMethods asserts the 4 methods that make
// LogicalNode duck-type-compatible with archdiagram.ResolverNode.
// (The compile-time interface assertion lives in docs/render/, which
// can import both packages without a cycle.)
func TestLogicalNode_ResolverNodeMethods(t *testing.T) {
	n := &LogicalNode{
		ID: "n1", Label: "Billing", Kind: "Application",
		Attributes: map[string]any{"domain": "source"},
	}
	if n.GetID() != "n1" {
		t.Errorf("GetID: got %q", n.GetID())
	}
	if n.GetLabel() != "Billing" {
		t.Errorf("GetLabel: got %q", n.GetLabel())
	}
	if n.GetKind() != "Application" {
		t.Errorf("GetKind: got %q", n.GetKind())
	}
	if n.GetAttributes()["domain"] != "source" {
		t.Errorf("GetAttributes: got %v", n.GetAttributes())
	}
}

// TestLogicalDiagram_EmptyFields tests that empty optional fields don't
// render in the JSON (omitempty tags work).
func TestLogicalDiagram_EmptyFields(t *testing.T) {
	minimal := LogicalDiagram{
		Viewpoint: "ktw-docs:/viewpoint/x",
		Nodes:     []LogicalNode{{ID: "n1", Label: "A", Kind: "X"}},
	}
	raw, _ := json.Marshal(&minimal)
	s := string(raw)
	for _, banned := range []string{`"title":""`, `"edges":[]`, `"zones":[]`, `"layoutHints":null`, `"attributes":{}`} {
		if contains(s, banned) {
			t.Errorf("JSON contains banned empty-field token %q (got: %s)", banned, s)
		}
	}
}

// TestLogicalDiagram_NilAttributes asserts that Attributes can be nil
// (LLM-emitted JSON often omits the field; resolvers must handle nil).
func TestLogicalDiagram_NilAttributes(t *testing.T) {
	n := &LogicalNode{ID: "n1", Label: "X", Kind: "Y"}
	if attrs := n.GetAttributes(); attrs != nil {
		t.Errorf("nil-attrs GetAttributes: got %v", attrs)
	}
}

// contains is a small substring helper to keep the asserts terse.
func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	n := len(needle)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(haystack); i++ {
		if haystack[i:i+n] == needle {
			return i
		}
	}
	return -1
}
