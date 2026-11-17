// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"
)

func lookupIn(ts ...ContentTemplate) func(string) (ContentTemplate, bool) {
	return func(n string) (ContentTemplate, bool) {
		for _, t := range ts {
			if t.Name == n {
				return t, true
			}
		}
		return ContentTemplate{}, false
	}
}

var risks = ContentTemplate{Name: "risks", Description: "Risks", Partial: true, Sections: []TemplateSection{
	{ID: "risks", Title: "Risks", Intent: "What could go wrong and how it is handled", Required: true},
}}

var board = ContentTemplate{Name: "architecture-board", Description: "Have an architecture approved", Sections: []TemplateSection{
	{ID: "ask", Title: "The decision asked", Intent: "What the board decides today", Required: true},
	{ID: "options", Title: "Options", Intent: "The candidates compared", Repeat: "dossier-solution",
		Sources: []TemplateSource{{From: "children", Value: "dossier-solution"}}, Deck: &DeckHint{Layout: "comparison"}},
	{ID: "r", Include: "risks"},
}}

// A full template includes partial ones: their sections take the include's
// place, ids prefixed so they stay unique.
func TestExpandInlinesPartialTemplates(t *testing.T) {
	got, err := Expand(board, lookupIn(board, risks))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range got.Sections {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "ask,options,r.risks" {
		t.Fatalf("sections = %v", ids)
	}
	if len(Check(board, lookupIn(board, risks))) != 0 {
		t.Fatalf("a sound template has findings: %v", Check(board, lookupIn(board, risks)))
	}
}

// Check names what an author gets wrong: an unknown include, a full template
// included, a cycle, an include with content, a section without intent, a
// source of an unknown kind.
func TestCheckNamesWhatIsWrong(t *testing.T) {
	missing := board
	missing.Sections = append([]TemplateSection{}, board.Sections...)
	missing.Sections[2] = TemplateSection{ID: "r", Include: "nowhere"}
	if fs := Check(missing, lookupIn(missing)); len(fs) != 1 || !strings.Contains(fs[0], "no template in reach") {
		t.Errorf("unknown include: %v", fs)
	}
	full := ContentTemplate{Name: "x", Description: "x", Sections: []TemplateSection{{ID: "b", Include: "architecture-board"}}}
	if fs := Check(full, lookupIn(full, board, risks)); len(fs) != 1 || !strings.Contains(fs[0], "not a partial") {
		t.Errorf("full included: %v", fs)
	}
	a := ContentTemplate{Name: "a", Description: "a", Partial: true, Sections: []TemplateSection{{ID: "b", Include: "b"}}}
	b := ContentTemplate{Name: "b", Description: "b", Partial: true, Sections: []TemplateSection{{ID: "a", Include: "a"}}}
	if fs := Check(a, lookupIn(a, b)); len(fs) != 1 || !strings.Contains(fs[0], "includes itself") {
		t.Errorf("cycle: %v", fs)
	}
	bad := ContentTemplate{Name: "Bad Name", Sections: []TemplateSection{
		{ID: "x", Include: "risks", Title: "T"},
		{ID: "y", Title: "No intent", Sources: []TemplateSource{{From: "web"}}},
	}}
	fs := strings.Join(Check(bad, nil), "\n")
	for _, want := range []string{"kebab-case", "description", "nothing but its id", "title and an intent", `from "web"`} {
		if !strings.Contains(fs, want) {
			t.Errorf("missing %q in:\n%s", want, fs)
		}
	}
}

// Parts marks each section with the groups it came through, nested includes
// outermost first; the template's own sections carry none.
func TestPartsMarkTheIncludedGroups(t *testing.T) {
	inner := ContentTemplate{Name: "costs", Description: "c", Partial: true, Sections: []TemplateSection{{ID: "cost", Title: "Cost", Intent: "i"}}}
	outer := ContentTemplate{Name: "money", Description: "m", Partial: true, Sections: []TemplateSection{{ID: "budget", Title: "Budget", Intent: "i"}, {ID: "c", Include: "costs"}}}
	top := ContentTemplate{Name: "top", Description: "t", Sections: []TemplateSection{{ID: "ask", Title: "Ask", Intent: "i"}, {ID: "m", Include: "money"}}}
	parts, err := Parts(top, lookupIn(top, outer, inner))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range parts {
		got = append(got, p.Section.ID+"="+strings.Join(p.Groups, "/"))
	}
	if strings.Join(got, ",") != "ask=,m.budget=money,m.c.cost=money/costs" {
		t.Fatalf("parts = %v", got)
	}
}
