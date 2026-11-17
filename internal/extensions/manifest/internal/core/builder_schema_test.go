// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A contribution's schema is the KIND's statement, resolved by a lookup.
//
// It used to be a field the producer set and the builder then overwrote — a
// value with two writers, which is a contradiction rather than a contract. The
// field is gone; what remains is the resolution, and this is it.
func TestSchemaResolvesFromTheKindThatDeclaresIt(t *testing.T) {
	ctx := context.Background()
	providers := []contribapi.ContributionProvider{
		// Kind declarations. Each NAMES its schema — that is the contract: a
		// kind says what its contributions validate against, rather than the
		// schema being inferred from who declared the kind.
		staticProvider{provider: "system", typ: "contribution-type", entries: []contribapi.Contribution{
			{Provider: "system", Type: "contribution-type", ID: "skill",
				Layers: contribapi.InlineData(json.RawMessage(`{"name":"skill","schema":"system:/data-type/skill"}`))},
			// The bootstrap self-reference: contribution-type is itself a kind.
			// It cannot name its own schema without circularity, so resolution
			// falls to the system data-type of the same name — the base case
			// where the chain terminates.
			{Provider: "system", Type: "contribution-type", ID: "contribution-type"},
		}},
		staticProvider{provider: "ktw-compose", typ: "contribution-type", entries: []contribapi.Contribution{
			{Provider: "ktw-compose", Type: "contribution-type", ID: "triage-candidate",
				Layers: contribapi.InlineData(json.RawMessage(`{"name":"triage-candidate","schema":"ktw-compose:/data-type/triage-candidate"}`))},
		}},
		staticProvider{provider: "ktw-docs", typ: "skill", entries: []contribapi.Contribution{
			{Provider: "ktw-docs", Type: "skill", ID: "alpha", Layers: contribapi.InlineData(json.RawMessage(`{"name":"A"}`))},
		}},
		staticProvider{provider: "ktw-compose", typ: "triage-candidate", entries: []contribapi.Contribution{
			{Provider: "ktw-compose", Type: "triage-candidate", ID: "t1", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
			{Provider: "ktw-compose", Type: "triage-candidate", ID: "t2", Layers: contribapi.InlineData(json.RawMessage(`{}`))},
		}},
	}

	m, err := NewBuilder("test", "dev").BuildFromProviders(ctx, providers)
	if err != nil {
		t.Fatalf("BuildFromProviders: %v", err)
	}

	kinds := map[string]string{}
	for _, c := range m.Contributions {
		if c.Type != "contribution-type" || len(c.Data()) == 0 {
			continue
		}
		var decl contribapi.KindDeclaration
		if err := json.Unmarshal(c.Data(), &decl); err != nil || decl.Schema == "" {
			continue
		}
		name := decl.Name
		if name == "" {
			name = c.ID
		}
		kinds[name] = decl.Schema
	}

	for _, tc := range []struct{ kind, want string }{
		{"skill", "system:/data-type/skill"},
		{"triage-candidate", "ktw-compose:/data-type/triage-candidate"},
		// The meta-model, whichever sidecar declared the kind.
		{"contribution-type", "system:/data-type/contribution-type"},
		{"data-type", "system:/data-type/data-type"},
	} {
		if got := mbapi.SchemaFor(kinds, tc.kind); got != tc.want {
			t.Errorf("kind %q: schema = %q, want %q", tc.kind, got, tc.want)
		}
	}

	// A kind nobody declared resolves to nothing, and saying nothing is the
	// honest answer — inferring a schema from the kind's name would produce a
	// URI that 404s and a check that silently never ran. (The builder refuses
	// such a kind outright, which is why this is asked of the resolution
	// rather than of a built manifest.)
	if got := mbapi.SchemaFor(kinds, "undeclared-kind"); got != "" {
		t.Errorf("an undeclared kind resolved to %q", got)
	}

	// Two entries of one kind resolve to the SAME schema — which is the whole
	// point, and the thing a per-entry field could not guarantee.
	if mbapi.SchemaFor(kinds, "triage-candidate") == "" {
		t.Fatal("triage-candidate resolved to nothing")
	}
	var seen int
	for _, c := range m.Contributions {
		if c.Type == "triage-candidate" {
			seen++
		}
	}
	if seen != 2 {
		t.Errorf("built %d triage-candidate entries, want 2", seen)
	}
}
