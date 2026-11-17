// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"strings"
	"testing"
)

// withSource is a complete kind with data and source layers and the given
// `source:` block.
func withSource(name string, s SourceDecl) KindDeclaration {
	d := complete(name)
	d.Layers["source"] = LayerDecl{MediaType: "text/markdown"}
	d.Source = &s
	return d
}

// A well-formed `source:` block is no finding: a skill-like kind read from its
// frontmatter and body, a translations sibling, a companion of a declared kind.
func TestKinds_AWellFormedSourceIsNoFinding(t *testing.T) {
	skill := withSource("skill", SourceDecl{Format: SourceFrontmatter, Match: []string{"*.md"},
		Layers: map[string]LayerFrom{"data": {From: FromFrontmatter}, "source": {From: FromBody, Fold: "instructions"}}})
	task := withSource("task", SourceDecl{Folder: "tasks", Format: SourceFrontmatter, ID: SourceIDBasename,
		Layers:     map[string]LayerFrom{"data": {From: FromFrontmatter}, "source": {From: FromSibling, Suffix: ".i18n.yaml"}},
		Companions: []Companion{{Kind: "skill", Suffix: ".input.dsl.yaml", ID: "task/{id}/input"}}})
	noSource := complete("descriptor")
	k := KindsFrom([]Contribution{kindEntry(t, skill), kindEntry(t, task), kindEntry(t, noSource)})
	if got := k.Check(); len(got) != 0 {
		t.Fatalf("findings on well-formed source blocks: %v", got)
	}
	if f := k.byName["skill"].SourceFolder(); f != "skill" {
		t.Errorf("SourceFolder of a kind naming none = %q, want its name", f)
	}
	if f := k.byName["task"].SourceFolder(); f != "tasks" {
		t.Errorf("SourceFolder = %q, want tasks", f)
	}
}

// Each malformed `source:` block is a kind-source-malformed error naming what
// is wrong — one case per check.
func TestKinds_AMalformedSourceIsAFinding(t *testing.T) {
	cases := map[string]struct {
		src  SourceDecl
		want string
	}{
		"unknown format":       {SourceDecl{Format: "xml"}, `format "xml"`},
		"no format":            {SourceDecl{}, `format ""`},
		"unknown id":           {SourceDecl{Format: SourceRaw, ID: "uuid"}, `id "uuid"`},
		"a nested folder":      {SourceDecl{Format: SourceRaw, Folder: "a/b"}, `folder "a/b"`},
		"a bad glob":           {SourceDecl{Format: SourceRaw, Match: []string{"[a"}}, `match "[a"`},
		"an undeclared layer":  {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"preview": {From: FromFile}}}, `layer "preview" is not declared`},
		"an unknown from":      {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"source": {From: "url"}}}, `from "url"`},
		"a sibling no suffix":  {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"source": {From: FromSibling}}}, "names no suffix"},
		"a suffix not sibling": {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"source": {From: FromFile, Suffix: ".x"}}}, "only for from sibling"},
		"body not frontmatter": {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"source": {From: FromBody}}}, "needs format frontmatter"},
		"tree from raw":        {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"data": {From: FromTree}}}, "needs format structured or dsl"},
		"fold into itself":     {SourceDecl{Format: SourceStructured, Layers: map[string]LayerFrom{"data": {From: FromTree, Fold: "x"}}}, `fold "x"`},
		"path not into data":   {SourceDecl{Format: SourceRaw, Layers: map[string]LayerFrom{"source": {From: FromPath}}}, "from path is the data layer's"},
		"an undeclared companion": {SourceDecl{Format: SourceRaw, Companions: []Companion{{Kind: "ghost", Suffix: ".x", ID: "{id}"}}},
			`companion kind "ghost"`},
		"a bad skip": {SourceDecl{Format: SourceRaw, Skip: []string{"refs/old"}}, `skip "refs/old"`},
		"a companion no {id}": {SourceDecl{Format: SourceRaw, Companions: []Companion{{Kind: "bad", Suffix: ".x", ID: "fixed"}}},
			`does not name the entry's {id}`},
	}
	for name, c := range cases {
		k := KindsFrom([]Contribution{kindEntry(t, withSource("bad", c.src))})
		var hits []string
		for _, f := range k.Check() {
			if f.Rule == KindRuleSourceMalformed && f.Severity == ValidateError && strings.Contains(f.Message, c.want) {
				hits = append(hits, f.Message)
			}
		}
		if len(hits) == 0 {
			t.Errorf("%s: no kind-source-malformed error containing %q in %v", name, c.want, k.Check())
		}
	}
}

// id: name reads the data layer: a kind with none is a finding.
func TestKinds_AnIDByNameWithNoDataLayerIsAFinding(t *testing.T) {
	d := complete("raw-only")
	d.Layers = map[string]LayerDecl{"source": {Default: true}}
	d.Source = &SourceDecl{Format: SourceRaw, ID: SourceIDName}
	for _, f := range KindsFrom([]Contribution{kindEntry(t, d)}).Check() {
		if f.Rule == KindRuleSourceMalformed && strings.Contains(f.Message, "id name") {
			return
		}
	}
	t.Fatal("no finding for id: name on a kind with no data layer")
}

// A fold needs a data layer to fold into: a kind with none is a finding.
func TestKinds_AFoldWithNoDataLayerIsAFinding(t *testing.T) {
	d := complete("raw-only")
	d.Layers = map[string]LayerDecl{"source": {Default: true}}
	d.Source = &SourceDecl{Format: SourceFrontmatter, Layers: map[string]LayerFrom{"source": {From: FromBody, Fold: "text"}}}
	k := KindsFrom([]Contribution{kindEntry(t, d)})
	for _, f := range k.Check() {
		if f.Rule == KindRuleSourceMalformed && strings.Contains(f.Message, `fold "text"`) {
			return
		}
	}
	t.Fatalf("no finding for a fold with no data layer: %v", k.Check())
}

// One folder, one kind: two kinds claiming a folder are each a finding,
// naming both — whether the folder is named or defaults to a kind's name.
func TestKinds_TwoKindsClaimingAFolderAreAFinding(t *testing.T) {
	a := withSource("notation", SourceDecl{Format: SourceStructured})
	b := withSource("viewpoint", SourceDecl{Format: SourceStructured, Folder: "notation"})
	k := KindsFrom([]Contribution{kindEntry(t, a), kindEntry(t, b)})
	claimed := map[string]bool{}
	for _, f := range k.Check() {
		if f.Rule == KindRuleSourceMalformed && strings.Contains(f.Message, `folder "notation" is claimed by notation, viewpoint`) {
			claimed[f.Kind] = true
		}
	}
	if !claimed["notation"] || !claimed["viewpoint"] {
		t.Fatalf("both kinds should be reported for the shared folder: %v", k.Check())
	}
}
