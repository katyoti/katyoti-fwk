// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// TestValidateAgainst_EnforcesRequiredAndTypes pins the body-vs-schema
// check ValidateAgainst gained: a registered data-type with required
// fields (required-by-absence-of-`optional`, the DSL convention) must
// REJECT a body missing one of them, and REJECT a type mismatch — not
// just confirm the body is JSON. This is what makes the US-4 node
// Error header (and US-S1's task-form gate) actually fire.
func TestValidateAgainst_EnforcesRequiredAndTypes(t *testing.T) {
	// Mirrors the dossier shape: project_code + project_name required,
	// subsystem optional.
	body := []byte(`name: dossier-like
root: DossierLike
types:
  DossierLike:
    type: object
    fields:
      project_code:
        type: string
        label: Project code
      project_name:
        type: string
        label: Project name
      subsystem:
        type: string
        optional: true
`)
	ctx := context.Background()
	svc, err := New(ctx)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	uri := "ktw-acme:/data-type/dossier-like"
	if err := svc.RegisterDataType(ctx, api.RegisterDataTypeInput{URI: uri, Body: body}); err != nil {
		t.Fatalf("RegisterDataType: %v", err)
	}

	validate := func(jsonBody string) schemaapi.ValidateOutput {
		out, err := svc.ValidateAgainst(ctx, api.ValidateAgainstInput{
			URI: uri, Body: json.RawMessage(jsonBody),
		})
		if err != nil {
			t.Fatalf("ValidateAgainst(%s): %v", jsonBody, err)
		}
		return out
	}

	// Empty body → both required fields missing → INVALID.
	if out := validate(`{}`); out.Valid {
		t.Error("empty body passed validation; required fields not enforced")
	}

	// Missing one required field → INVALID, and the diagnostic names it.
	out := validate(`{"project_name":"Acme"}`)
	if out.Valid {
		t.Fatal("body missing project_code passed validation")
	}
	joined := ""
	for _, d := range out.Diagnostics {
		joined += d.Message + " " + d.Path + " "
	}
	if !strings.Contains(joined, "project_code") {
		t.Errorf("diagnostic does not mention the missing field: %q", joined)
	}

	// Wrong type → INVALID.
	if out := validate(`{"project_code":"X","project_name":7}`); out.Valid {
		t.Error("body with a numeric project_name passed validation")
	}

	// All required present, optional omitted → VALID.
	if out := validate(`{"project_code":"X","project_name":"Acme"}`); !out.Valid {
		t.Errorf("complete body rejected: %+v", out.Diagnostics)
	}
}
