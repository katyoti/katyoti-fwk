// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

const cacheTestURI = "ktw-test:/data-type/cached"

func registerCached(t *testing.T, svc api.SchemaService, required string) {
	t.Helper()
	body := []byte(`name: cached
root: Cached
types:
  Cached:
    type: object
    fields:
      ` + required + `:
        type: string
`)
	if err := svc.RegisterDataType(context.Background(), api.RegisterDataTypeInput{URI: cacheTestURI, Body: body}); err != nil {
		t.Fatalf("RegisterDataType: %v", err)
	}
}

func valid(t *testing.T, svc api.SchemaService, body string) bool {
	t.Helper()
	out, err := svc.ValidateAgainst(context.Background(), api.ValidateAgainstInput{URI: cacheTestURI, Body: json.RawMessage(body)})
	if err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
	return out.Valid
}

// A data-type's validator is built once per registration and reused for
// every body: a catalog reindex validates one body per contribution, and
// compiling the JSON Schema per body made a hub boot spend seconds there.
func TestValidateAgainst_CompilesOncePerRegistration(t *testing.T) {
	ctx := context.Background()
	s, _ := New(ctx)
	svc := s.(*service)
	registerCached(t, svc, "title")

	if !valid(t, svc, `{"title":"a"}`) || valid(t, svc, `{}`) {
		t.Fatal("validation verdicts wrong before any caching question")
	}
	first, err := svc.store.getValidator(cacheTestURI)
	if err != nil || first == nil || first.compiled == nil {
		t.Fatalf("validator after first use = %+v, %v; want a compiled schema", first, err)
	}
	valid(t, svc, `{"title":"b"}`)
	if again, _ := svc.store.getValidator(cacheTestURI); again != first {
		t.Error("the validator was rebuilt for a second body; want the cached one")
	}
}

// Re-registering a data-type drops its cached validator: the next body is
// judged by the new schema, never the old one.
func TestValidateAgainst_ReRegistrationInvalidatesTheValidator(t *testing.T) {
	ctx := context.Background()
	svc, _ := New(ctx)
	registerCached(t, svc, "title")
	if !valid(t, svc, `{"title":"a"}`) {
		t.Fatal("body with title rejected under the first schema")
	}
	registerCached(t, svc, "name")
	if valid(t, svc, `{"title":"a"}`) {
		t.Error("body without the new required field accepted: the old schema's validator was reused")
	}
	if !valid(t, svc, `{"name":"a"}`) {
		t.Error("body matching the new schema rejected")
	}
}

// Diagnostics handed to one caller are its own: appending a body's errors
// must not leak into the cached AST diagnostics another caller reads.
func TestValidateAgainst_DiagnosticsAreNotShared(t *testing.T) {
	ctx := context.Background()
	svc, _ := New(ctx)
	registerCached(t, svc, "title")
	bad, _ := svc.ValidateAgainst(ctx, api.ValidateAgainstInput{URI: cacheTestURI, Body: json.RawMessage(`{}`)})
	good, _ := svc.ValidateAgainst(ctx, api.ValidateAgainstInput{URI: cacheTestURI, Body: json.RawMessage(`{"title":"x"}`)})
	if len(bad.Diagnostics) == 0 {
		t.Fatal("a body missing a required field got no diagnostic")
	}
	if !good.Valid || len(good.Diagnostics) >= len(bad.Diagnostics) {
		t.Errorf("a valid body carries %d diagnostics after an invalid one carried %d: the cached list was appended to",
			len(good.Diagnostics), len(bad.Diagnostics))
	}
}
