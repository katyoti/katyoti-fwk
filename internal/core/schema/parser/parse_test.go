// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"context"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
)

const flatYAML = `$schema: ktw-schema/v1
name: flat
version: "0.1.0"
description: "test schema"
types:
  User:
    type: object
  Post:
    type: object
`

const flatJSON = `{
  "$schema": "ktw-schema/v1",
  "name": "flat",
  "version": "0.1.0",
  "types": {
    "User": {"type": "object"},
    "Post": {"type": "object"}
  }
}`

func TestParse_OuterYAML(t *testing.T) {
	out, err := Parse(context.Background(), schemaapi.ParseInput{
		Body:   []byte(flatYAML),
		Format: schemaapi.FormatYAML,
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustHaveTwoTypes(t, out.Schema)
	if out.Schema.DSLVersion != "ktw-schema/v1" {
		t.Errorf("DSLVersion: got %q want ktw-schema/v1", out.Schema.DSLVersion)
	}
	if out.Schema.Description != "test schema" {
		t.Errorf("Description: got %q", out.Schema.Description)
	}
}

func TestParse_OuterJSON(t *testing.T) {
	out, err := Parse(context.Background(), schemaapi.ParseInput{
		Body:   []byte(flatJSON),
		Format: schemaapi.FormatJSON,
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	mustHaveTwoTypes(t, out.Schema)
}

func TestParse_AutoDetect(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"yaml", flatYAML},
		{"json", flatJSON},
		{"yaml-with-bom", "\xEF\xBB\xBF" + flatYAML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Parse(context.Background(), schemaapi.ParseInput{
				Body:   []byte(tc.body),
				Format: schemaapi.FormatAuto,
			})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			mustHaveTwoTypes(t, out.Schema)
		})
	}
}

func TestParse_RejectsMissingName(t *testing.T) {
	bad := `$schema: ktw-schema/v1
types:
  User:
    type: object
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(bad), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected error to mention 'name', got: %v", err)
	}
}

func TestParse_RejectsMissingTypes(t *testing.T) {
	bad := `$schema: ktw-schema/v1
name: empty
`
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(bad), Format: schemaapi.FormatYAML})
	if err == nil {
		t.Fatal("expected error for missing types, got nil")
	}
}

func TestParse_RejectsEmptyBody(t *testing.T) {
	_, err := Parse(context.Background(), schemaapi.ParseInput{Body: nil})
	if err == nil {
		t.Fatal("expected error for empty body, got nil")
	}
}

func TestParse_ExpandsTopLevelShorthand(t *testing.T) {
	body := `$schema: ktw-schema/v1
name: shorthand
types:
  email: string@email?
`
	out, err := Parse(context.Background(), schemaapi.ParseInput{Body: []byte(body), Format: schemaapi.FormatYAML})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := out.Schema.Types["email"]; got == nil ||
		got.Kind != schemaapi.KindString || got.Content != "email" || !got.Optional {
		t.Errorf("top-level shorthand not expanded: %+v", got)
	}
}

func mustHaveTwoTypes(t *testing.T, s *schemaapi.Schema) {
	t.Helper()
	if s == nil {
		t.Fatalf("schema is nil")
	}
	if got := len(s.Types); got != 2 {
		t.Fatalf("len(Types): got %d want 2", got)
	}
	for _, name := range []string{"User", "Post"} {
		ty, ok := s.Types[name]
		if !ok {
			t.Fatalf("missing type %q", name)
		}
		if ty.Kind != schemaapi.KindObject {
			t.Errorf("type %q: kind=%q want %q", name, ty.Kind, schemaapi.KindObject)
		}
	}
}
