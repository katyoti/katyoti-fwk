// SPDX-License-Identifier: Apache-2.0

package report

import (
	"context"
	"testing"
)

// fakeQuery answers canned rows per query name, with two columns, and records
// the variables of its last call.
func fakeQuery(data map[string][]map[string]string, lastVars *map[string]string) QueryFunc {
	return func(_ context.Context, query string, vars map[string]string) ([]map[string]string, []Column, error) {
		if lastVars != nil {
			*lastVars = vars
		}
		rows, ok := data[query]
		if !ok {
			return nil, nil, nil
		}
		cols := []Column{
			{ID: "id", Label: "ID"},
			{ID: "displayName", Label: "Display Name", Sets: []string{"default"}},
		}
		return rows, cols, nil
	}
}

// testTransformers holds a pass-through and an add-column transformer.
func testTransformers() Transformers {
	t := Transformers{}
	t.Register("pass-through", func(input *StepResult, params map[string]any) (*StepResult, error) {
		return &StepResult{
			Rows:    input.Rows,
			Columns: input.Columns,
		}, nil
	})
	t.Register("add-column", func(input *StepResult, params map[string]any) (*StepResult, error) {
		colID, _ := params["column"].(string)
		colVal, _ := params["value"].(string)
		rows := make([]map[string]string, len(input.Rows))
		for i, row := range input.Rows {
			newRow := make(map[string]string, len(row)+1)
			for k, v := range row {
				newRow[k] = v
			}
			newRow[colID] = colVal
			rows[i] = newRow
		}
		cols := append(input.Columns, Column{ID: colID, Label: colID})
		return &StepResult{Rows: rows, Columns: cols}, nil
	})
	return t
}

func TestRunner_QueryStep(t *testing.T) {
	qr := fakeQuery(map[string][]map[string]string{
		"all-apps": {
			{"id": "1", "displayName": "App A"},
			{"id": "2", "displayName": "App B"},
		},
	}, nil)

	def := &ReportDef{
		Name: "Test",
		Steps: []StepDef{
			{Name: "apps", Type: "query", Query: "all-apps"},
		},
	}

	engine := NewRunner(qr, testTransformers())
	results, err := engine.Run(context.Background(), def, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	apps, ok := results["apps"]
	if !ok {
		t.Fatal("step 'apps' not in results")
	}
	if len(apps.Rows) != 2 {
		t.Errorf("rows = %d, want 2", len(apps.Rows))
	}
	if apps.Rows[0]["displayName"] != "App A" {
		t.Errorf("row[0] displayName = %q", apps.Rows[0]["displayName"])
	}
}

func TestRunner_TransformStep(t *testing.T) {
	qr := fakeQuery(map[string][]map[string]string{
		"all-apps": {
			{"id": "1", "displayName": "App A"},
		},
	}, nil)

	def := &ReportDef{
		Name: "Test",
		Steps: []StepDef{
			{Name: "apps", Type: "query", Query: "all-apps"},
			{Name: "tagged", Type: "transform", Transformer: "add-column", Input: "apps", Params: map[string]any{
				"column": "source",
				"value":  "example.org",
			}},
		},
	}

	engine := NewRunner(qr, testTransformers())
	results, err := engine.Run(context.Background(), def, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tagged := results["tagged"]
	if tagged == nil {
		t.Fatal("step 'tagged' not in results")
	}
	if len(tagged.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(tagged.Rows))
	}
	if tagged.Rows[0]["source"] != "example.org" {
		t.Errorf("source = %q, want example.org", tagged.Rows[0]["source"])
	}
}

func TestRunner_VariableResolution(t *testing.T) {
	var capturedVars map[string]string
	qr := fakeQuery(map[string][]map[string]string{"test": {}}, &capturedVars)

	// Wrap to capture vars
	def := &ReportDef{
		Name: "Test",
		Variables: map[string]VarDef{
			"app_ids": {Required: true},
		},
		Steps: []StepDef{
			{Name: "data", Type: "query", Query: "test", Vars: map[string]string{"ids": "${app_ids}"}},
		},
	}

	engine := NewRunner(qr, testTransformers())

	// Missing required var should fail
	_, err := engine.Run(context.Background(), def, nil)
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}

	// With var provided
	_, err = engine.Run(context.Background(), def, map[string]string{"app_ids": "abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedVars["ids"] != "abc123" {
		t.Errorf("query vars = %v, want ids=abc123", capturedVars)
	}
}

func TestRunner_SubOutputReference(t *testing.T) {
	qr := fakeQuery(map[string][]map[string]string{
		"test": {{"id": "1"}},
	}, nil)

	// Transformer that produces sub-outputs
	registry := Transformers{
		"split": func(input *StepResult, params map[string]any) (*StepResult, error) {
			return &StepResult{
				Rows: input.Rows,
				SubOutputs: map[string]*StepResult{
					"valid":  {Rows: input.Rows, Columns: []Column{{ID: "id", Label: "ID"}}},
					"errors": {Rows: nil, Columns: []Column{{ID: "error", Label: "Error"}}},
				},
			}, nil
		},
	}

	def := &ReportDef{
		Name: "Test",
		Steps: []StepDef{
			{Name: "data", Type: "query", Query: "test"},
			{Name: "split", Type: "transform", Transformer: "split", Input: "data"},
		},
		Output: OutputDef{
			XLSX: &XLSXOutputDef{
				Sheets: []SheetDef{
					{Name: "Valid", Data: "split.valid"},
					{Name: "Errors", Data: "split.errors"},
				},
			},
		},
	}

	engine := NewRunner(qr, registry)
	results, err := engine.Run(context.Background(), def, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Resolve output references
	valid, err := ResolveOutputData("split.valid", results)
	if err != nil {
		t.Fatalf("resolve split.valid: %v", err)
	}
	if len(valid.Rows) != 1 {
		t.Errorf("valid rows = %d, want 1", len(valid.Rows))
	}

	errors, err := ResolveOutputData("split.errors", results)
	if err != nil {
		t.Fatalf("resolve split.errors: %v", err)
	}
	if len(errors.Rows) != 0 {
		t.Errorf("error rows = %d, want 0", len(errors.Rows))
	}

	// Invalid reference
	_, err = ResolveOutputData("nonexistent", results)
	if err == nil {
		t.Fatal("expected error for nonexistent step")
	}

	_, err = ResolveOutputData("split.nonexistent", results)
	if err == nil {
		t.Fatal("expected error for nonexistent sub-output")
	}
}

func TestRunner_QueryColumnsKeepTheirSets(t *testing.T) {
	qr := fakeQuery(map[string][]map[string]string{"q": {{"id": "1"}}}, nil)
	def := &ReportDef{Name: "Test", Steps: []StepDef{{Name: "s", Type: "query", Query: "q"}}}
	results, err := NewRunner(qr, nil).Run(context.Background(), def, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cols := results["s"].Columns
	if len(cols) != 2 || cols[1].Label != "Display Name" || len(cols[1].Sets) != 1 || cols[1].Sets[0] != "default" {
		t.Errorf("columns = %+v, want the query's columns with their sets", cols)
	}
}

func TestRunner_UnknownTransformer(t *testing.T) {
	qr := fakeQuery(map[string][]map[string]string{"q": {{"id": "1"}}}, nil)
	def := &ReportDef{Name: "Test", Steps: []StepDef{
		{Name: "s", Type: "query", Query: "q"},
		{Name: "t", Type: "transform", Transformer: "nope", Input: "s"},
	}}
	if _, err := NewRunner(qr, nil).Run(context.Background(), def, nil); err == nil {
		t.Fatal("expected an error for an unknown transformer")
	}
}

func TestHeaderColumns(t *testing.T) {
	got := HeaderColumns(&StepResult{Columns: []Column{{ID: "a"}, {ID: "b", Label: "B"}}})
	if len(got) != 2 || got[0].Label != "a" || got[1].Label != "B" {
		t.Errorf("columns = %+v, want labels a and B", got)
	}
	got = HeaderColumns(&StepResult{Rows: []map[string]string{{"z": "1", "a": "2"}}})
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "z" {
		t.Errorf("columns from rows = %+v, want a then z", got)
	}
	if got := HeaderColumns(&StepResult{}); got != nil {
		t.Errorf("empty result columns = %+v, want none", got)
	}
}
