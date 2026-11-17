// SPDX-License-Identifier: Apache-2.0

package report

import (
	"testing"
)

func TestParseReportDef_Valid(t *testing.T) {
	yaml := `
version: "1"
name: "Test Report"
description: "A test report"
variables:
  app_ids:
    desc: "Application UUID(s)"
    required: true
  format:
    desc: "Output format"
    default: "xlsx"

steps:
  - name: interfaces
    type: query
    query: app-interfaces
    vars:
      ids: "${app_ids}"

  - name: applications
    type: transform
    transformer: group-by-relation
    input: interfaces
    params:
      group_by:
        - name: Supplier
          field: "supplier"

output:
  xlsx:
    sheets:
      - name: "Interfaces"
        data: interfaces
      - name: "Applications"
        data: applications
`
	def, err := ParseReportDef([]byte(yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if def.Name != "Test Report" {
		t.Errorf("name = %q, want %q", def.Name, "Test Report")
	}
	if len(def.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(def.Steps))
	}

	// Step 1: query
	s1 := def.Steps[0]
	if s1.Name != "interfaces" || s1.Type != "query" || s1.Query != "app-interfaces" {
		t.Errorf("step 1: got %+v", s1)
	}
	if s1.Vars["ids"] != "${app_ids}" {
		t.Errorf("step 1 vars: %v", s1.Vars)
	}

	// Step 2: transform
	s2 := def.Steps[1]
	if s2.Name != "applications" || s2.Type != "transform" || s2.Transformer != "group-by-relation" || s2.Input != "interfaces" {
		t.Errorf("step 2: got %+v", s2)
	}

	// Output
	if def.Output.XLSX == nil || len(def.Output.XLSX.Sheets) != 2 {
		t.Fatalf("xlsx sheets = %v", def.Output.XLSX)
	}
	if def.Output.XLSX.Sheets[1].Data != "applications" {
		t.Errorf("sheet 2 data = %q", def.Output.XLSX.Sheets[1].Data)
	}

	// Variables
	if v, ok := def.Variables["app_ids"]; !ok || !v.Required {
		t.Errorf("app_ids variable: %+v", v)
	}
	if v, ok := def.Variables["format"]; !ok || v.Default != "xlsx" {
		t.Errorf("format variable: %+v", v)
	}
}

func TestParseReportDef_MissingName(t *testing.T) {
	yaml := `
version: "1"
steps:
  - name: s1
    type: query
    query: test
output: {}
`
	_, err := ParseReportDef([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseReportDef_NoSteps(t *testing.T) {
	yaml := `
name: "Test"
steps: []
output: {}
`
	_, err := ParseReportDef([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for no steps")
	}
}

func TestParseReportDef_InvalidStepType(t *testing.T) {
	yaml := `
name: "Test"
steps:
  - name: s1
    type: unknown
output: {}
`
	_, err := ParseReportDef([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for unknown step type")
	}
}

func TestParseReportDef_TransformMissingInput(t *testing.T) {
	yaml := `
name: "Test"
steps:
  - name: s1
    type: transform
    transformer: group-by-relation
output: {}
`
	_, err := ParseReportDef([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for missing input")
	}
}

func TestResolveVariables(t *testing.T) {
	def := &ReportDef{
		Variables: map[string]VarDef{
			"app_ids": {Required: true},
			"format":  {Default: "xlsx"},
			"limit":   {Default: "100"},
		},
	}

	// With required var provided
	merged, err := ResolveVariables(def, map[string]string{"app_ids": "abc123", "limit": "50"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if merged["app_ids"] != "abc123" {
		t.Errorf("app_ids = %q", merged["app_ids"])
	}
	if merged["format"] != "xlsx" {
		t.Errorf("format = %q", merged["format"])
	}
	if merged["limit"] != "50" {
		t.Errorf("limit = %q, want 50 (override)", merged["limit"])
	}

	// Missing required var
	_, err = ResolveVariables(def, map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
}
