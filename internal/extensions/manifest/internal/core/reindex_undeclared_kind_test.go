// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// twoMemberCatalog seeds a catalog with two members in the given order, each
// with its own contributions, and reindexes it under the given config.
func twoMemberCatalog(t *testing.T, cfg contribapi.ContributionsConfig, first, second string, firstCs, secondCs []contribapi.Contribution) *CatalogImpl {
	t.Helper()
	cache := NewCache(t.TempDir())
	for _, m := range []struct {
		name string
		cs   []contribapi.Contribution
	}{{first, firstCs}, {second, secondCs}} {
		man := makeManifest(m.name, "1.0.0", "sha256:"+m.name)
		man.Contributions = m.cs
		if err := SeedCacheSave(t, cache, man); err != nil {
			t.Fatalf("seed %s: %v", m.name, err)
		}
	}
	cat, err := NewCatalog(cache, MembersOf(cache, first, second))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	if err := cat.SetSchema(&validatorStub{}); err != nil {
		t.Fatalf("SetSchema: %v", err)
	}
	cat.SetValidationConfig(cfg)
	if err := cat.Reindex(context.Background(), nil); err != nil {
		t.Fatalf("Reindex: %v", err)
	}
	return cat
}

func memberKindDecl(provider, kind, schema, validate string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: contribapi.ResourceTypeContributionType, ID: kind}
	c.SetData(json.RawMessage(`{"name":"` + kind + `","schema":"` + schema + `","validate":"` + validate + `"}`))
	return c
}

func entryAt(t *testing.T, cat *CatalogImpl, uri string) contribapi.Contribution {
	t.Helper()
	c, ok := cat.ContributionAt(uri)
	if !ok {
		t.Fatalf("%s is not in the catalog", uri)
	}
	return c
}

// The aggregator is where "declared nowhere" is decided (blind audit H3): a
// member's builder keeps entries of a kind it does not declare, and here —
// with every member's declarations in view — the entry whose kind none
// declares is marked invalid with its reason, and kept. An entry of a kind
// ANOTHER member declares validates against that member's declaration.
func TestReindex_UndeclaredKindIsRefusedOnTheEntry_ForeignKindResolves(t *testing.T) {
	recipe := contribapi.Contribution{Provider: "go-producer", Type: "recipe", ID: "crepes",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"Crepes"}`))}
	orphan := contribapi.Contribution{Provider: "go-producer", Type: "gadget", ID: "g1",
		Layers: contribapi.InlineData(json.RawMessage(`{"name":"g1"}`))}
	cat := twoMemberCatalog(t, contribapi.ContributionsConfig{},
		"go-producer", "files-only-pack",
		[]contribapi.Contribution{recipe, orphan},
		[]contribapi.Contribution{memberKindDecl("files-only-pack", "recipe", "files-only-pack:/data-type/recipe", "error")})

	got := entryAt(t, cat, "go-producer:/recipe/crepes")
	if got.Valid == nil || !*got.Valid || len(got.ValidationErrors) != 0 {
		t.Errorf("foreign kind declared by another member: verdict valid=%v errors=%v, want an explicit pass", got.Valid, got.ValidationErrors)
	}
	bad := entryAt(t, cat, "go-producer:/gadget/g1")
	if bad.Valid == nil || *bad.Valid {
		t.Errorf("kind declared by no member: Valid = %v, want *false (marked, kept)", bad.Valid)
	}
	if len(bad.ValidationErrors) != 1 || !strings.Contains(bad.ValidationErrors[0], `kind "gadget" is declared by no member`) ||
		!strings.Contains(bad.ValidationErrors[0], "go-producer") {
		t.Errorf("the reason must name the kind and the member, got %v", bad.ValidationErrors)
	}
	// Central `off` skips it, like every other check.
	off := twoMemberCatalog(t, contribapi.ContributionsConfig{Validate: "off"},
		"go-producer", "files-only-pack", []contribapi.Contribution{orphan}, nil)
	if got := entryAt(t, off, "go-producer:/gadget/g1"); got.Valid != nil {
		t.Errorf("under off nothing is checked, got Valid=%v", got.Valid)
	}
}

// A member declaring `skill` no longer shadows `system:/contribution-type/skill`
// however the membership is ordered (blind audit H4): the system declaration
// wins by origin, the member's is a finding, and an entry validates under the
// system kind's severity, not the member's.
func TestReindex_SystemKindWinsOverAMemberRedeclarationInAnyMemberOrder(t *testing.T) {
	skill := contribapi.Contribution{Provider: "ktw-rogue", Type: "skill", ID: "s1",
		Layers: contribapi.InlineData(json.RawMessage(`{}`))}
	rogueDecl := memberKindDecl("ktw-rogue", "skill", "ktw-rogue:/data-type/skill", "off")
	systemDecl := memberKindDecl("system", "skill", "system:/data-type/skill", "error")
	stub := &validatorStub{bad: map[string]bool{`{}`: true}}

	for name, order := range map[string][2]string{"rogue first": {"ktw-rogue", "ktw-hub"}, "hub first": {"ktw-hub", "ktw-rogue"}} {
		cs := map[string][]contribapi.Contribution{
			"ktw-rogue": {rogueDecl, skill},
			"ktw-hub":   {systemDecl},
		}
		cache := NewCache(t.TempDir())
		for _, member := range order {
			man := makeManifest(member, "1.0.0", "sha256:"+member)
			man.Contributions = cs[member]
			if err := SeedCacheSave(t, cache, man); err != nil {
				t.Fatal(err)
			}
		}
		cat, err := NewCatalog(cache, MembersOf(cache, order[0], order[1]))
		if err != nil {
			t.Fatal(err)
		}
		if err := cat.SetSchema(stub); err != nil {
			t.Fatal(err)
		}
		if err := cat.Reindex(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
		kinds := cat.Kinds()
		if got := kinds.DeclaredBy("skill"); got != "system:/contribution-type/skill" {
			t.Errorf("%s: skill resolves to %q, want the system declaration", name, got)
		}
		if got := kinds.SchemaFor("skill"); got != "system:/data-type/skill" {
			t.Errorf("%s: SchemaFor(skill) = %q", name, got)
		}
		var redeclares []contribapi.KindFinding
		for _, f := range kinds.Check() {
			if f.Rule == contribapi.KindRuleRedeclaresSystem {
				redeclares = append(redeclares, f)
			}
		}
		if len(redeclares) != 1 || redeclares[0].URI != "ktw-rogue:/contribution-type/skill" {
			t.Errorf("%s: want one redeclares-system finding on the member URI, got %v", name, redeclares)
		}
		// The member's `validate: off` did not win: the entry fails under the system kind's `error`.
		got := entryAt(t, cat, "ktw-rogue:/skill/s1")
		if got.Valid == nil || *got.Valid {
			t.Errorf("%s: entry validated under the member's shadowing declaration (Valid=%v); the system kind says error", name, got.Valid)
		}
	}
}
