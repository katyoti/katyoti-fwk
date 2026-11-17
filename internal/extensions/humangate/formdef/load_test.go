// SPDX-License-Identifier: Apache-2.0

package formdef

import (
	"testing"
)

func TestParse_SimpleFlat(t *testing.T) {
	yaml := []byte(`
screen:
  title: Simple Form
fields:
  - key: name
    type: string
    label: Name
    required: true
  - key: email
    type: string
    label: Email
    validation:
      format: email
  - key: age
    type: integer
    label: Age
    validation:
      minimum: 0
      maximum: 150
`)
	def, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if def.Screen.Title != "Simple Form" {
		t.Errorf("title = %q, want %q", def.Screen.Title, "Simple Form")
	}
	if len(def.Fields) != 3 {
		t.Fatalf("fields count = %d, want 3", len(def.Fields))
	}
	if !def.Fields[0].Required {
		t.Error("name.required should be true")
	}
	if def.Fields[1].Validation.Format != "email" {
		t.Error("email.validation.format should be 'email'")
	}
	if *def.Fields[2].Validation.Maximum != 150 {
		t.Error("age.validation.maximum should be 150")
	}
}

func TestParse_Tabbed(t *testing.T) {
	yaml := []byte(`
screen:
  title: Data Flow Specification
  width: 900
  height: 700
groups:
  - key: flow-info
    label: Flow Info
    fields:
      - key: flow_name
        type: string
        label: Flow Name
        required: true
      - key: source_app
        type: string
        label: Source Application
        required: true
      - key: target_app
        type: string
        label: Target Application
        required: true
  - key: data-details
    label: Data Details
    fields:
      - key: description
        type: text
        label: Data Description
      - key: format
        type: enum
        label: Format
        options:
          - value: json
            label: JSON
          - value: xml
            label: XML
          - value: csv
            label: CSV
          - value: binary
            label: Binary
      - key: frequency
        type: enum
        label: Frequency
        options:
          - value: realtime
            label: Real-time
          - value: hourly
            label: Hourly
          - value: daily
            label: Daily
          - value: weekly
            label: Weekly
  - key: sla
    label: SLA
    fields:
      - key: volume
        type: string
        label: Expected Volume
      - key: max_latency
        type: integer
        label: Max Latency (ms)
        validation:
          minimum: 0
      - key: availability
        type: number
        label: Availability (%)
        default: 99.9
        validation:
          minimum: 0
          maximum: 100
  - key: dicp
    label: DICP Classification
    fields:
      - key: disponibilite
        type: enum
        label: Disponibilité
        required: true
        options:
          - value: "1"
            label: "1 - Standard"
          - value: "2"
            label: "2 - Enhanced"
          - value: "3"
            label: "3 - High"
          - value: "4"
            label: "4 - Critical"
      - key: integrite
        type: enum
        label: Intégrité
        required: true
        options:
          - value: "1"
            label: "1 - Standard"
          - value: "2"
            label: "2 - Enhanced"
          - value: "3"
            label: "3 - High"
          - value: "4"
            label: "4 - Critical"
      - key: confidentialite
        type: enum
        label: Confidentialité
        required: true
        options:
          - value: "1"
            label: "1 - Public"
          - value: "2"
            label: "2 - Internal"
          - value: "3"
            label: "3 - Confidential"
          - value: "4"
            label: "4 - Secret"
      - key: preuve
        type: enum
        label: Preuve
        required: true
        options:
          - value: "1"
            label: "1 - Standard"
          - value: "2"
            label: "2 - Enhanced"
          - value: "3"
            label: "3 - High"
          - value: "4"
            label: "4 - Critical"
`)
	def, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if def.Screen.Title != "Data Flow Specification" {
		t.Errorf("title = %q", def.Screen.Title)
	}
	if len(def.Groups) != 4 {
		t.Fatalf("groups = %d, want 4", len(def.Groups))
	}
	if def.Groups[0].Key != "flow-info" {
		t.Errorf("group[0].key = %q", def.Groups[0].Key)
	}
	all := def.AllFields()
	if len(all) != 13 {
		t.Errorf("AllFields() = %d, want 13", len(all))
	}
}

func TestParse_AllFieldTypes(t *testing.T) {
	yaml := []byte(`
screen:
  title: All Types
fields:
  - key: f_string
    type: string
  - key: f_text
    type: text
  - key: f_markdown
    type: markdown
  - key: f_integer
    type: integer
  - key: f_number
    type: number
  - key: f_boolean
    type: boolean
  - key: f_enum
    type: enum
    options:
      - value: a
        label: A
      - value: b
        label: B
  - key: f_date
    type: date
  - key: f_time
    type: time
  - key: f_datetime
    type: datetime
  - key: f_object
    type: object
    fields:
      - key: nested_key
        type: string
  - key: f_array
    type: array
    items:
      key: item
      type: string
  - key: f_table
    type: table
    columns:
      - key: col1
        type: string
        label: Column 1
      - key: col2
        type: integer
        label: Column 2
`)
	def, err := Parse(yaml)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(def.Fields) != 13 {
		t.Errorf("fields = %d, want 13", len(def.Fields))
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "no title",
			yaml: "fields:\n  - key: x\n    type: string",
			want: "screen.title is required",
		},
		{
			name: "no fields or groups",
			yaml: "screen:\n  title: T",
			want: "form must have at least one field or group",
		},
		{
			name: "both fields and groups",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: string\ngroups:\n  - key: g\n    label: G\n    fields:\n      - key: y\n        type: string",
			want: "form must use either top-level fields or groups, not both",
		},
		{
			name: "missing field key",
			yaml: "screen:\n  title: T\nfields:\n  - type: string",
			want: "key is required",
		},
		{
			name: "unknown field type",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: unknown",
			want: "unknown type",
		},
		{
			name: "enum without options",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: enum",
			want: "enum field must have options",
		},
		{
			name: "object without fields",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: object",
			want: "object field must have sub-fields",
		},
		{
			name: "array without items",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: array",
			want: "array field must have items",
		},
		{
			name: "table without columns",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: table",
			want: "table field must have columns",
		},
		{
			name: "duplicate field key",
			yaml: "screen:\n  title: T\nfields:\n  - key: x\n    type: string\n  - key: x\n    type: string",
			want: "duplicate field key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.yaml))
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !contains(got, tt.want) {
				t.Errorf("error = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
