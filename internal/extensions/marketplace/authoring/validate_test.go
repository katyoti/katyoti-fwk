// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// A pack's content is checked against the schema its kind declares, at
// packaging, because that is the last moment before it is distributed and the
// only moment where the author is still present.

func skill(id, body string) contribapi.Contribution {
	return contribapi.Contribution{
		Provider: "pack", Type: "skill", ID: id,
		Layers: contribapi.InlineData(json.RawMessage(body)),
	}
}

// kinds builds a registry from declarations, the way a pack's own
// contribution-type entries do. The entry no longer carries a schema or a
// severity; its kind does.
func kinds(decls ...contribapi.KindDeclaration) contribapi.Kinds {
	var cs []contribapi.Contribution
	for _, d := range decls {
		body, _ := json.Marshal(d)
		c := contribapi.Contribution{Provider: "pack", Type: contribapi.ResourceTypeContributionType, ID: d.Name}
		c.SetData(body)
		cs = append(cs, c)
	}
	return contribapi.KindsFrom(cs)
}

var skillKinds = kinds(contribapi.KindDeclaration{Name: "skill", Schema: "system:/data-type/skill", Validate: "warn"})

// rejecting matches a body containing `bad`, standing in for a real schema
// check. The point under test is what packaging DOES with a mismatch, not how
// the parser finds one.
func rejecting(_ context.Context, uri string, body json.RawMessage) error {
	if strings.Contains(string(body), "bad") {
		return errors.New("field `name`: expected string, got number")
	}
	return nil
}

func TestPackagingRefusesAMismatchAndShipsOnceFixed(t *testing.T) {
	cfg := PackagingConfig(skillKinds)

	broken := []contribapi.Contribution{skill("a", `{"name":"bad"}`)}
	findings := ValidatePack(context.Background(), broken, cfg, skillKinds, rejecting)
	if len(findings) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(findings), findings)
	}
	err := RefuseOn(findings)
	if err == nil {
		t.Fatal("packaging accepted a body that does not match its schema")
	}
	// The refusal has to say WHICH contribution and WHAT was wrong, or the
	// author is left to guess which of two hundred files to open.
	for _, want := range []string{"pack:/skill/a", "expected string"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q:\n%v", want, err)
		}
	}

	fixed := []contribapi.Contribution{skill("a", `{"name":"ok"}`)}
	if findings := ValidatePack(context.Background(), fixed, cfg, skillKinds, rejecting); len(findings) != 0 {
		t.Fatalf("fixed pack still reported: %v", findings)
	}
}

// A kind's `validate:` answers a question about a live catalog. Packaging asks
// a different one — nothing has shipped yet — so `warn` escalates.
func TestPackagingEscalatesWarnButHonoursOff(t *testing.T) {
	k := kinds(
		contribapi.KindDeclaration{Name: "skill", Schema: "system:/data-type/skill", Validate: "warn"},
		contribapi.KindDeclaration{Name: "note", Schema: "system:/data-type/note", Validate: "off"},
	)
	cfg := PackagingConfig(k)

	if got := k.SeverityFor("skill", cfg); got != contribapi.ValidateOn {
		t.Errorf("a kind declaring warn should be strict at packaging, got %q", got)
	}
	// `off` is not a judgement about loudness — it is a kind saying its bodies
	// are not describable by a schema. Escalating it would invent a check its
	// author said does not exist.
	if got := k.SeverityFor("note", cfg); got != contribapi.ValidateOff {
		t.Errorf("a kind declaring off must stay off, got %q", got)
	}
	// A kind nobody declared is still checked: an undeclared kind is the case
	// most likely to be a typo.
	if got := k.SeverityFor("unheard-of", cfg); got != contribapi.ValidateOn {
		t.Errorf("an undeclared kind should be strict at packaging, got %q", got)
	}
}

// A kind that declares no schema is reported, not silently accepted. The
// finding names the KIND, because that is where the omission is — the entry is
// blameless.
func TestAKindDeclaringNoSchemaIsReported(t *testing.T) {
	c := contribapi.Contribution{Provider: "pack", Type: "skill", ID: "a", Layers: contribapi.InlineData(json.RawMessage(`{}`))}
	// No kind declares a schema for this entry's kind, so nothing can be
	// checked — and the check has to SAY so rather than pass.
	findings := ValidatePack(context.Background(), []contribapi.Contribution{c},
		PackagingConfig(skillKinds), contribapi.KindsFrom(nil), rejecting)

	if len(findings) != 1 {
		t.Fatalf("a contribution with no declared data-type was accepted silently: %v", findings)
	}
	if !strings.Contains(findings[0].String(), "declares no data-type") {
		t.Errorf("finding does not explain the problem: %v", findings[0])
	}
}

// Nil validator is not a pass. A packaging run with no schema service has not
// checked anything, and saying "all good" would be a lie the author acts on.
func TestNoValidatorIsReportedRatherThanAssumedClean(t *testing.T) {
	findings := ValidatePack(context.Background(),
		[]contribapi.Contribution{skill("a", `{"name":"bad"}`)},
		PackagingConfig(skillKinds), skillKinds, nil)

	if len(findings) != 1 {
		t.Fatalf("a run without a validator reported nothing: %v", findings)
	}
	if !strings.Contains(findings[0].String(), "no schema service") {
		t.Errorf("finding does not say the check did not happen: %v", findings[0])
	}
}

// Warnings do not stop a build; only errors do. Kept as a separate test
// because the two halves of RefuseOn are separately easy to get wrong.
func TestRefuseOnStopsOnErrorsOnly(t *testing.T) {
	if err := RefuseOn([]Finding{{URI: "a", Severity: contribapi.ValidateWarn, Messages: []string{"m"}}}); err != nil {
		t.Errorf("a warning refused the build: %v", err)
	}
	if err := RefuseOn(nil); err != nil {
		t.Errorf("a clean pack refused: %v", err)
	}
	if err := RefuseOn([]Finding{{URI: "a", Severity: contribapi.ValidateError, Messages: []string{"m"}}}); err == nil {
		t.Error("an error-severity finding did not refuse")
	}
}

// A file that published NOTHING is the failure the schema check structurally
// cannot reach: it reasons about contributions, and a dropped file produces
// none. So packaging checks the other side — what the host met and could not
// read — and refuses on it.
func TestPackagingRefusesAFileThatPublishedNothing(t *testing.T) {
	findings := FindingsForDrops([]contribapi.Drop{
		{Path: "skills/broken.md", Kind: "skill", Reason: "parse skill frontmatter: line 3"},
		{Path: "personas/reviewer.md", Kind: "persona", Reason: "parse persona frontmatter: line 5"},
	})
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}
	// Sorted, so a refusal reads the same way twice.
	if findings[0].URI != "personas/reviewer.md" {
		t.Errorf("findings are not sorted by path: %v", findings)
	}

	err := RefuseOn(findings)
	if err == nil {
		t.Fatal("packaging shipped a pack that lost content")
	}
	// The refusal has to name the FILE and the reason — the author is looking
	// for something that is not in the pack, and the pack cannot describe it.
	for _, want := range []string{"skills/broken.md", "published nothing", "line 3"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q:\n%v", want, err)
		}
	}
}

// Always error severity, never configurable: every other finding is about
// content that IS in the pack and might be wrong, while this one is about
// content that is not there at all. There is no degraded-but-usable absent.
func TestADroppedFileIsNeverJustAWarning(t *testing.T) {
	for _, f := range FindingsForDrops([]contribapi.Drop{{Path: "x.md", Reason: "boom"}}) {
		if f.Severity != contribapi.ValidateError {
			t.Errorf("drop severity = %q, want error", f.Severity)
		}
	}
	if len(FindingsForDrops(nil)) != 0 {
		t.Error("no drops produced findings")
	}
}
