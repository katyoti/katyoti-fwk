// SPDX-License-Identifier: Apache-2.0

package census

import (
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// Falsifying the two kind guards the blind audit added (H4, H3) before
// trusting them: a guard that cannot fail proves nothing.

func declOf(provider, kind, schema string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: contribapi.ResourceTypeContributionType, ID: kind}
	c.SetData(json.RawMessage(`{"name":"` + kind + `","schema":"` + schema + `","validate":"error",` +
		`"layers":{"data":{"mediaType":"application/json","required":true,"default":true}},"meta":{"budget":256}}`))
	c.URI = c.ResolvedURI()
	return c
}

func entryOf(provider, kind, id string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: kind, ID: id}
	c.SetData(json.RawMessage(`{"name":"` + id + `"}`))
	c.URI = c.ResolvedURI()
	return c
}

// H4: a binary whose own contributions redeclare a system kind, or declare
// one kind twice, is refused — with the rule and both URIs.
func TestCensusRefusesARedeclaredSystemKindAndADuplicateDeclaration(t *testing.T) {
	system := declOf(contribapi.SystemProvider, "skill", "system:/data-type/skill")
	rogue := declOf("ktw-x", "skill", "ktw-x:/data-type/skill")
	twiceA := declOf("ktw-x", "widget", "ktw-x:/data-type/widget")
	twiceB := declOf("ktw-x", "widget", "ktw-x:/data-type/widget")
	twiceB.ID = "widget-again" // a second URI, same body
	twiceB.URI = "ktw-x:/contribution-type/widget-again"
	cs := []contribapi.Contribution{rogue, system, twiceA, twiceB, entryOf("ktw-x", "skill", "s"), entryOf("ktw-x", "widget", "w")}

	got := strings.Join(kindFindings(contribapi.KindsFrom(cs), cs), "\n")
	for _, want := range []string{
		"[" + contribapi.KindRuleRedeclaresSystem + "]",
		`kind "skill" (ktw-x:/contribution-type/skill)`, "system:/contribution-type/skill",
		"[" + contribapi.KindRuleDuplicate + "]",
		"ktw-x:/contribution-type/widget-again", "identical bodies",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("findings lack %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "\n")+1 != 2 {
		t.Errorf("want exactly two findings (one per offending declaration), got:\n%s", got)
	}

	// Declared once each: nothing to report.
	clean := []contribapi.Contribution{system, twiceA, entryOf("ktw-x", "skill", "s"), entryOf("ktw-x", "widget", "w")}
	if got := kindFindings(contribapi.KindsFrom(clean), clean); len(got) != 0 {
		t.Errorf("clean declarations reported: %v", got)
	}
}

// H3: a foreign kind resolves against the declared universe the census was
// given, and one it cannot resolve fails with the kind and the entry URI.
func TestCensusResolvesForeignKindsAgainstNamedDeclarationsOnly(t *testing.T) {
	m := &mbapi.Manifest{Provider: "go-producer", ForeignKinds: []string{"recipe"},
		Contributions: []contribapi.Contribution{
			declOf(contribapi.SystemProvider, "skill", "system:/data-type/skill"),
			entryOf("go-producer", "recipe", "crepes"),
			entryOf("go-producer", "skill", "s"),
		}}

	// Nothing named: the foreign kind cannot resolve.
	got := strings.Join(UnresolvedForeignKinds(m, nil), "\n")
	if !strings.Contains(got, `go-producer:/recipe/crepes: kind "recipe" is declared by neither go-producer nor system`) {
		t.Errorf("an unresolved foreign kind must name the kind and the entry, got:\n%s", got)
	}
	if strings.Contains(got, "skill") {
		t.Errorf("a kind the binary resolves itself (system skill) is not foreign:\n%s", got)
	}

	// The declaring pack named: resolved, nothing to report.
	pack := []contribapi.Contribution{declOf("files-only-pack", "recipe", "files-only-pack:/data-type/recipe")}
	if got := UnresolvedForeignKinds(m, pack); len(got) != 0 {
		t.Errorf("foreign kind declared by the named pack still reported: %v", got)
	}

	// The builder is the one writer of ForeignKinds: a manifest whose record
	// disagrees with its entries is reported, even when the kind resolves.
	stale := *m
	stale.ForeignKinds = nil
	got = strings.Join(UnresolvedForeignKinds(&stale, pack), "\n")
	if !strings.Contains(got, "records foreign kinds [] but its entries use undeclared kinds [recipe]") {
		t.Errorf("a stale ForeignKinds record must be reported, got:\n%s", got)
	}
}

// The real fixture, through the real producers: files-only-pack declares
// `recipe` and ships its schema, so a foreign `recipe` resolves against it.
func TestFixtureDeclarationsReadTheRealPack(t *testing.T) {
	decls := FixtureDeclarations(t, "files-only-pack")
	kinds := contribapi.KindsFrom(decls)
	if got := kinds.SchemaFor("recipe"); got != "files-only-pack:/data-type/recipe" {
		t.Fatalf("files-only-pack declares recipe with schema %q; the fixture read %d contributions", got, len(decls))
	}
	var schema bool
	for _, c := range decls {
		if c.ResolvedURI() == "files-only-pack:/data-type/recipe" && len(c.Source()) > 0 {
			schema = true
		}
	}
	if !schema {
		t.Errorf("the pack's schema was not read: %v", decls)
	}
}
