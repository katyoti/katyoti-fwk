// SPDX-License-Identifier: Apache-2.0

package report

import (
	"strings"
	"testing"
)

func TestGroupByRelation_SingleField(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{
				"id":                   "iface1",
				"document.id":          "do1",
				"document.displayName": "Invoice",
				"flow.externalId":      "FL0001",
			},
			{
				"id":                   "iface2",
				"document.id":          "do1",
				"document.displayName": "Invoice",
				"flow.externalId":      "FL0002",
			},
			{
				"id":                   "iface3",
				"document.id":          "do2",
				"document.displayName": "Order",
				"flow.externalId":      "FL0003",
			},
		},
	}

	result, err := groupByRelation(input, map[string]any{
		"group_by": []any{
			map[string]any{"name": "Document", "field": "document"},
		},
		"collect": []any{"flow.externalId"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}

	// Should be sorted by displayName: Invoice, Order
	if result.Rows[0]["displayName"] != "Invoice" {
		t.Errorf("row[0] displayName = %q, want Invoice", result.Rows[0]["displayName"])
	}
	if result.Rows[1]["displayName"] != "Order" {
		t.Errorf("row[1] displayName = %q, want Order", result.Rows[1]["displayName"])
	}

	// Invoice should have 2 references
	invoiceRefs := result.Rows[0]["flow.count"]
	if invoiceRefs != "2" {
		t.Errorf("Invoice ref count = %q, want 2", invoiceRefs)
	}

	// Order should have 1 reference
	orderRefs := result.Rows[1]["flow.count"]
	if orderRefs != "1" {
		t.Errorf("Order ref count = %q, want 1", orderRefs)
	}
}

func TestGroupByRelation_MultiField(t *testing.T) {
	// Simulate grouping interfaces by provider AND consumer application
	input := &StepResult{
		Rows: []map[string]string{
			{
				"supplier.id":          "app1",
				"supplier.displayName": "AppA",
				"customer.id":          "app2",
				"customer.displayName": "AppB",
				"flow.externalId":      "FL0001",
			},
			{
				"supplier.id":          "app2",
				"supplier.displayName": "AppB",
				"customer.id":          "app1",
				"customer.displayName": "AppA",
				"flow.externalId":      "FL0002",
			},
		},
	}

	result, err := groupByRelation(input, map[string]any{
		"group_by": []any{
			map[string]any{"name": "Supplier", "field": "supplier"},
			map[string]any{"name": "Customer", "field": "customer"},
		},
		"collect": []any{"flow.externalId"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 (AppA and AppB)", len(result.Rows))
	}

	// Both apps should appear, each with 2 total references (1 as provider, 1 as consumer)
	for _, row := range result.Rows {
		total := row["flow.count"]
		if total != "2" {
			t.Errorf("%s total refs = %q, want 2", row["displayName"], total)
		}
		// Should have Supplier and Customer direction columns
		if row["Supplier.count"] == "" || row["Customer.count"] == "" {
			t.Errorf("%s missing direction columns: %v", row["displayName"], row)
		}
	}
}

func TestGroupByRelation_MultilineValues(t *testing.T) {
	// An interface connected to multiple data objects (multiline)
	input := &StepResult{
		Rows: []map[string]string{
			{
				"document.id":          "do1\ndo2",
				"document.displayName": "Invoice\nOrder",
				"flow.externalId":      "FL0001",
			},
		},
	}

	result, err := groupByRelation(input, map[string]any{
		"group_by": []any{
			map[string]any{"name": "Document", "field": "document"},
		},
		"collect": []any{"flow.externalId"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2 (multiline split)", len(result.Rows))
	}
}

func TestGroupByRelation_MissingParams(t *testing.T) {
	input := &StepResult{Rows: nil}

	_, err := groupByRelation(input, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing params")
	}

	_, err = groupByRelation(input, map[string]any{
		"group_by": []any{map[string]any{"field": "someField"}},
	})
	if err == nil {
		t.Fatal("expected error for missing collect")
	}

	// Plain string should fail (must be {name, field} object)
	_, err = groupByRelation(input, map[string]any{
		"group_by": []any{"plainString"},
		"collect":  []any{"x"},
	})
	if err == nil {
		t.Fatal("expected error for plain string in group_by")
	}
}

func TestSplitMultiline(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"single", 1},
		{"a\nb\nc", 3},
		{"a\n\nb", 2}, // empty lines filtered
		{"  a  \n  b  ", 2},
	}
	for _, tt := range tests {
		got := splitMultiline(tt.input)
		if len(got) != tt.want {
			t.Errorf("splitMultiline(%q) = %d items, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestSortedSetJoin(t *testing.T) {
	set := map[string]bool{"c": true, "a": true, "b": true}
	got := sortedSetJoin(set)
	if got != "a\nb\nc" {
		t.Errorf("sortedSetJoin = %q, want %q", got, strings.Join([]string{"a", "b", "c"}, "\n"))
	}

	// Empty set
	got = sortedSetJoin(nil)
	if got != "" {
		t.Errorf("sortedSetJoin(nil) = %q, want empty", got)
	}
}
