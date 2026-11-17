// SPDX-License-Identifier: Apache-2.0

package report

import (
	"testing"
)

func TestFilterRows_Eq(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{"name": "Alice", "status": "active"},
			{"name": "Bob", "status": "inactive"},
			{"name": "Charlie", "status": "active"},
		},
	}

	result, err := filterRows(input, map[string]any{
		"field": "status",
		"op":    "eq",
		"value": "active",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}
}

func TestFilterRows_Exclude(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{"name": "Alice", "status": "active"},
			{"name": "Bob", "status": "inactive"},
		},
	}

	result, err := filterRows(input, map[string]any{
		"field":   "status",
		"op":      "eq",
		"value":   "inactive",
		"exclude": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0]["name"] != "Alice" {
		t.Errorf("expected Alice only, got %v", result.Rows)
	}
}

func TestFilterRows_Contains(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{"desc": "hello world"},
			{"desc": "goodbye"},
			{"desc": "world peace"},
		},
	}

	result, err := filterRows(input, map[string]any{
		"field": "desc",
		"op":    "contains",
		"value": "world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}
}

func TestFilterRows_Empty(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{"name": "Alice", "email": "alice@example.com"},
			{"name": "Bob", "email": ""},
		},
	}

	result, err := filterRows(input, map[string]any{
		"field": "email",
		"op":    "not_empty",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(result.Rows))
	}
}

func TestFilterRows_MissingParams(t *testing.T) {
	input := &StepResult{Rows: nil}

	_, err := filterRows(input, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing field")
	}

	_, err = filterRows(input, map[string]any{"field": "x"})
	if err == nil {
		t.Fatal("expected error for missing op")
	}

	_, err = filterRows(input, map[string]any{"field": "x", "op": "bad"})
	if err == nil {
		t.Fatal("expected error for unknown op")
	}
}

func TestDeriveColumn_Concat(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{"first": "John", "last": "Doe"},
			{"first": "Jane", "last": "Smith"},
		},
		Columns: []Column{
			{ID: "first", Label: "First Name"},
			{ID: "last", Label: "Last Name"},
		},
	}

	result, err := deriveColumn(input, map[string]any{
		"column":    "fullName",
		"label":     "Full Name",
		"op":        "concat",
		"fields":    []any{"first", "last"},
		"separator": " ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}
	if result.Rows[0]["fullName"] != "John Doe" {
		t.Errorf("fullName = %q, want 'John Doe'", result.Rows[0]["fullName"])
	}
	if len(result.Columns) != 3 {
		t.Errorf("columns = %d, want 3", len(result.Columns))
	}
	if result.Columns[2].ID != "fullName" {
		t.Errorf("column[2].ID = %q", result.Columns[2].ID)
	}
}

func TestDeriveColumn_CountLines(t *testing.T) {
	input := &StepResult{
		Rows: []map[string]string{
			{"refs": "FL001\nFL002\nFL003"},
			{"refs": "FL004"},
			{"refs": ""},
		},
		Columns: []Column{{ID: "refs", Label: "References"}},
	}

	result, err := deriveColumn(input, map[string]any{
		"column": "refCount",
		"op":     "count_lines",
		"field":  "refs",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Rows[0]["refCount"] != "3" {
		t.Errorf("row[0] refCount = %q, want 3", result.Rows[0]["refCount"])
	}
	if result.Rows[1]["refCount"] != "1" {
		t.Errorf("row[1] refCount = %q, want 1", result.Rows[1]["refCount"])
	}
	if result.Rows[2]["refCount"] != "0" {
		t.Errorf("row[2] refCount = %q, want 0", result.Rows[2]["refCount"])
	}
}

func TestDeriveColumn_Literal(t *testing.T) {
	input := &StepResult{
		Rows:    []map[string]string{{"a": "1"}, {"a": "2"}},
		Columns: []Column{{ID: "a", Label: "A"}},
	}

	result, err := deriveColumn(input, map[string]any{
		"column": "source",
		"op":     "literal",
		"value":  "example.org",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, row := range result.Rows {
		if row["source"] != "example.org" {
			t.Errorf("source = %q, want example.org", row["source"])
		}
	}
}

func TestDeriveColumn_MissingParams(t *testing.T) {
	input := &StepResult{Rows: nil, Columns: nil}

	_, err := deriveColumn(input, map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing column")
	}

	_, err = deriveColumn(input, map[string]any{"column": "x"})
	if err == nil {
		t.Fatal("expected error for missing op")
	}

	_, err = deriveColumn(input, map[string]any{"column": "x", "op": "bad"})
	if err == nil {
		t.Fatal("expected error for unknown op")
	}
}

func TestAllTransformersRegistered(t *testing.T) {
	expected := []string{"group-by-relation", "filter-rows", "derive-column"}
	for _, name := range expected {
		if _, err := BuiltinTransformers().Get(name); err != nil {
			t.Errorf("transformer %q not registered: %v", name, err)
		}
	}
}

func TestRegistryUnknown(t *testing.T) {
	if _, err := BuiltinTransformers().Get("nonexistent"); err == nil {
		t.Fatal("expected error for unknown transformer")
	}
}
