// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// TestValidateContribution_NoStructuredBodyIsSkipped pins the distinction
// between "we did not check this" and "this failed".
//
// A schema describes the body a consumer DECODES. A data-type has none — it IS
// its DSL, carried in the source layer since c24. Feeding the empty data layer
// to the validator made all 106 of them report `unexpected end of JSON input`:
// an entry called invalid for not having a thing its kind does not have.
func TestValidateContribution_NoStructuredBodyIsSkipped(t *testing.T) {
	called := false
	validate := func(context.Context, string, json.RawMessage) error {
		called = true
		return nil
	}
	dsl := contribapi.Contribution{Provider: "p", Type: contribapi.ResourceTypeDataType, ID: "notation"}
	dsl.SetSource([]byte("$schema: ktw-schema/v1\nname: notation\n"), "application/yaml")

	got := ValidateContribution(context.Background(), contribapi.ValidateWarn, dsl, "p:/data-type/data-type", validate)
	if !got.Skipped {
		t.Errorf("a source-only contribution must be SKIPPED, got %+v", got)
	}
	if got.Valid != nil {
		t.Error("skipped must not claim a verdict — nothing was checked")
	}
	if len(got.Errors) != 0 {
		t.Errorf("skipped must report no errors, got %v", got.Errors)
	}
	if called {
		t.Error("the validator was handed an empty body; it should never have been called")
	}
}

// The other half: an entry that DOES carry a structured body is still checked,
// or the skip above would have turned validation off for everything.
func TestValidateContribution_StructuredBodyStillValidated(t *testing.T) {
	var seen json.RawMessage
	validate := func(_ context.Context, _ string, body json.RawMessage) error {
		seen = body
		return nil
	}
	c := contribapi.Contribution{Provider: "p", Type: "node-type", ID: "dossier"}
	c.SetData([]byte(`{"name":"dossier"}`))

	got := ValidateContribution(context.Background(), contribapi.ValidateWarn,
		c, "p:/data-type/node-type", validate)
	if got.Skipped {
		t.Fatal("a contribution WITH a body must be validated, not skipped")
	}
	if got.Valid == nil || !*got.Valid {
		t.Errorf("expected a recorded pass, got %+v", got)
	}
	if string(seen) != `{"name":"dossier"}` {
		t.Errorf("validator saw %q", seen)
	}
}
