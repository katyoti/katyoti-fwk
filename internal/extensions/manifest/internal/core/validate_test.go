// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// validatorStub is a CatalogSchemaService whose ValidateBody rejects any body
// it is told to. GetDataTypeByURI returns a trivial single-type schema so edge
// extraction in Reindex doesn't bail before validation runs.
//
// It keys on the BODY, not on the schema URI. It used to key on the URI, which
// worked only while a schema was a per-entry field: now a schema is the kind's,
// so two entries of one kind share it, and a stub that answered per URI could
// not say that one of them is malformed and the other is not. Keying on the
// body is also what a real validator does.
type validatorStub struct {
	bad map[string]bool // exact body text that should fail validation
}

func (v *validatorStub) GetDataTypeByURI(_ context.Context, _ string) (json.RawMessage, error) {
	// Minimal well-formed schema doc (object root) so resolveDataType +
	// ExtractEdges proceed without error.
	return json.RawMessage(`{"root":"T","types":{"T":{"kind":"object","fields":{}}}}`), nil
}

func (v *validatorStub) ValidateBody(_ context.Context, _ string, body json.RawMessage) error {
	if v.bad[string(body)] {
		return errors.New("missing required field: name")
	}
	return nil
}

// seedCatalog builds a catalog from a single manifest with the given
// contributions, attaches the validator stub + validation config, and
// returns the reindexed catalog.
func seedCatalog(t *testing.T, cfg contribapi.ContributionsConfig, stub *validatorStub, contributions []contribapi.Contribution) *CatalogImpl {
	t.Helper()
	cache := NewCache(t.TempDir())
	m := makeManifest("test", "1.0.0", "sha256:x")
	m.Contributions = contributions
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "test"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if err := cat.SetSchema(stub); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	cat.SetValidationConfig(cfg)
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	return cat
}

// reindexedContribution pulls the (single-provider) contribution with
// the given ID back out of the catalog after Reindex stamped it.
func reindexedContribution(t *testing.T, cat *CatalogImpl, typ, id string) contribapi.Contribution {
	t.Helper()
	got, err := cat.GetContributionsByType(context.Background(), typ)
	if err != nil {
		t.Fatalf("GetContributionsByType: %v", err)
	}
	for _, c := range got {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("contribution %s/%s not found after reindex", typ, id)
	return contribapi.Contribution{}
}

// TestReindex_FlagsMalformedBodies_MultipleTypes covers SC#5: a malformed
// body for TWO types (data-type + a CWM item-type) is flagged invalid at
// the single checkpoint under error severity, while a valid body passes.
// kindDecl seeds the kind declaration a contribution's schema now comes from.
//
// Tests used to set `Schema:` on the entry. They cannot any more, and that is
// the point of c22: a schema is the KIND's statement, so a test that wants an
// entry validated has to say which kind declares what — the same thing a real
// pack has to say.
func kindDecl(kind, schema string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "system", Type: "contribution-type", ID: kind}
	c.SetData(json.RawMessage(`{"name":"` + kind + `","schema":"` + schema + `"}`))
	return c
}

// strictKindDecl is a kind that declares `validate: error` — the severity
// comes from the KIND now, not from a hard-coded list of strict types, so a
// test that wants an invalid verdict has to say so where a real kind would.
func strictKindDecl(kind, schema string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: "system", Type: "contribution-type", ID: kind}
	c.SetData(json.RawMessage(`{"name":"` + kind + `","schema":"` + schema + `","validate":"error"}`))
	return c
}

func TestReindex_FlagsMalformedBodies_MultipleTypes(t *testing.T) {
	stub := &validatorStub{bad: map[string]bool{`{}`: true}}
	// Both kinds DECLARE error severity; the config (zero value) overrides nothing.
	cat := seedCatalog(t, contribapi.ContributionsConfig{}, stub, []contribapi.Contribution{
		strictKindDecl("data-type", "system:/data-type/data-type"),
		strictKindDecl("item-type", "system:/data-type/item-type"),
		{Provider: "test", Type: "data-type", ID: "broken", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
		{Provider: "test", Type: "data-type", ID: "ok", Layers: contribapi.InlineData(json.RawMessage(`{"name":"ok"}`))},
		{Provider: "test", Type: "item-type", ID: "item-broken", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
	})

	broken := reindexedContribution(t, cat, "data-type", "broken")
	if broken.Valid == nil || *broken.Valid {
		t.Errorf("data-type/broken: Valid = %v, want *false", broken.Valid)
	}
	if len(broken.ValidationErrors) == 0 {
		t.Errorf("data-type/broken: expected ValidationErrors, got none")
	}

	okDT := reindexedContribution(t, cat, "data-type", "ok")
	if okDT.Valid == nil || !*okDT.Valid {
		t.Errorf("data-type/ok: Valid = %v, want *true", okDT.Valid)
	}
	if len(okDT.ValidationErrors) != 0 {
		t.Errorf("data-type/ok: expected no errors, got %v", okDT.ValidationErrors)
	}

	itemBroken := reindexedContribution(t, cat, "item-type", "item-broken")
	if itemBroken.Valid == nil || *itemBroken.Valid {
		t.Errorf("item-type/item-broken: Valid = %v, want *false", itemBroken.Valid)
	}
}

// TestReindex_EmptySchemaFlagged covers SC#1: a contribution whose KIND is
// declared but names no Schema is flagged under a strict (error) severity.
// (A kind declared by nobody is a different verdict — the aggregator's
// undeclared-kind refusal, TestReindex_UndeclaredKindIsRefusedOnTheEntry.)
func TestReindex_EmptySchemaFlagged(t *testing.T) {
	stub := &validatorStub{}
	cfg := contribapi.ContributionsConfig{PerType: map[string]string{"skill": contribapi.ValidateError}}
	noSchema := contribapi.Contribution{Provider: "system", Type: "contribution-type", ID: "skill"}
	noSchema.SetData(json.RawMessage(`{"name":"skill"}`))
	cat := seedCatalog(t, cfg, stub, []contribapi.Contribution{
		noSchema,
		{Provider: "test", Type: "skill", ID: "no-schema", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
	})
	c := reindexedContribution(t, cat, "skill", "no-schema")
	if c.Valid == nil || *c.Valid {
		t.Errorf("empty-schema skill: Valid = %v, want *false", c.Valid)
	}
	if len(c.ValidationErrors) == 0 || !strings.Contains(c.ValidationErrors[0], "no data-type") {
		t.Errorf("empty-schema skill: expected 'no data-type' diagnostic, got %v", c.ValidationErrors)
	}
}

// TestReindex_ToggleSeverity covers the toggle matrix: off skips (Valid
// stays nil), warn keeps valid (Valid nil) but records errors, error
// marks invalid (Valid=*false).
func TestReindex_ToggleSeverity(t *testing.T) {
	cases := []struct {
		name       string
		severity   string
		wantValid  *bool // nil = expect nil pointer
		wantErrors bool
	}{
		{"off", contribapi.ValidateOff, nil, false},
		{"warn", contribapi.ValidateWarn, nil, true},
		{"error", contribapi.ValidateError, ptrBool(false), true},
		{"on", contribapi.ValidateOn, ptrBool(false), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &validatorStub{bad: map[string]bool{`{}`: true}}
			cfg := contribapi.ContributionsConfig{PerType: map[string]string{"skill": tc.severity}}
			cat := seedCatalog(t, cfg, stub, []contribapi.Contribution{
				kindDecl("skill", "system:/data-type/x"),
				{Provider: "test", Type: "skill", ID: "s", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
			})
			c := reindexedContribution(t, cat, "skill", "s")
			if tc.wantValid == nil {
				if c.Valid != nil {
					t.Errorf("%s: Valid = %v, want nil", tc.name, c.Valid)
				}
			} else {
				if c.Valid == nil || *c.Valid != *tc.wantValid {
					t.Errorf("%s: Valid = %v, want %v", tc.name, c.Valid, *tc.wantValid)
				}
			}
			if got := len(c.ValidationErrors) > 0; got != tc.wantErrors {
				t.Errorf("%s: hasErrors = %v, want %v (%v)", tc.name, got, tc.wantErrors, c.ValidationErrors)
			}
		})
	}
}

// TestReindex_VirtualServiceBody covers SC#6 (source-agnostic): a virtual /
// reflected `service` contribution flows through the SAME Reindex validation
// path as a file-backed one — validation keys off the Body, never the source.
func TestReindex_VirtualServiceBody(t *testing.T) {
	stub := &validatorStub{bad: map[string]bool{`"not-an-object"`: true}}
	cfg := contribapi.ContributionsConfig{PerType: map[string]string{"service": contribapi.ValidateError}}
	cat := seedCatalog(t, cfg, stub, []contribapi.Contribution{
		kindDecl("service", "sc:/data-type/svc"),
		{Provider: "sc", Type: "service", ID: "svc", Layers: contribapi.InlineData(json.RawMessage(`"not-an-object"`))},
	})
	c := reindexedContribution(t, cat, "service", "svc")
	if c.Valid == nil || *c.Valid {
		t.Errorf("virtual service body: Valid = %v, want *false", c.Valid)
	}
	if len(c.ValidationErrors) == 0 {
		t.Errorf("virtual service body: expected ValidationErrors, got none")
	}
}

func ptrBool(b bool) *bool { return &b }

// A data-type's verdict is whether its DSL parsed at registration — and it
// TRAVELS: after Phase 2 hands the outcomes to the catalog, Reindex stamps
// them, so a listing no longer shows every data-type as "unchecked" and a
// schema that does not parse is visibly invalid, kept, with the reason.
func TestReindex_DataTypeVerdictTravels(t *testing.T) {
	good := contribapi.Contribution{Provider: "test", Type: contribapi.ResourceTypeDataType, ID: "good"}
	good.SetSource([]byte("$schema: ktw-schema/v1\nname: good\n"), "application/yaml")
	bad := contribapi.Contribution{Provider: "test", Type: contribapi.ResourceTypeDataType, ID: "bad"}
	bad.SetSource([]byte("$schema: ktw-schema/v1\nname: [\n"), "application/yaml")
	unknown := contribapi.Contribution{Provider: "test", Type: contribapi.ResourceTypeDataType, ID: "unknown"}
	unknown.SetSource([]byte("name: unknown\n"), "application/yaml")

	cache := NewCache(t.TempDir())
	m := makeManifest("test", "1.0.0", "sha256:x")
	m.Contributions = []contribapi.Contribution{
		strictKindDecl(contribapi.ResourceTypeDataType, "system:/data-type/data-type"), good, bad, unknown,
	}
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("seed: %v", err)
	}
	cat, err := NewCatalog(cache, MembersOf(cache, "test"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if err := cat.SetSchema(&validatorStub{}); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	cat.SetRegistrationOutcomes(map[string]mbapi.RegistrationOutcome{
		"test:/data-type/good": {Registered: true},
		"test:/data-type/bad":  {Registered: false, Reason: "yaml: line 2: did not find expected node content"},
	})
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}

	g := reindexedContribution(t, cat, contribapi.ResourceTypeDataType, "good")
	if g.Valid == nil || !*g.Valid {
		t.Errorf("registered data-type: Valid = %v, want *true", g.Valid)
	}
	b := reindexedContribution(t, cat, contribapi.ResourceTypeDataType, "bad")
	if b.Valid == nil || *b.Valid {
		t.Errorf("unparseable data-type: Valid = %v, want *false (marked, kept)", b.Valid)
	}
	if len(b.ValidationErrors) == 0 || !strings.Contains(b.ValidationErrors[0], "did not find expected node") {
		t.Errorf("unparseable data-type: reason must travel, got %v", b.ValidationErrors)
	}
	u := reindexedContribution(t, cat, contribapi.ResourceTypeDataType, "unknown")
	if u.Valid != nil {
		t.Errorf("a data-type with no registration outcome stays unchecked, got %v", u.Valid)
	}
}
