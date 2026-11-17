// SPDX-License-Identifier: Apache-2.0

package api

import (
	"strings"
	"testing"
)

// A deck object's body holds only the slides: a frontmatter copied into it
// is named, a body opening on a slide is not.
func TestObjectBodyWithAFrontmatterIsAnError(t *testing.T) {
	has := func(out ObjectLintOutput) bool {
		for _, f := range out.Findings {
			if f.Rule == "body-frontmatter" {
				return true
			}
		}
		return false
	}
	g := Grammar{}
	copied := DeckObjectInput{Title: "T", Body: "---\ntitle: T\ntheme: acme\naspect: \"16:9\"\n---\n\n## One\n- a\n"}
	if !has(LintObjectDeck(copied, g)) {
		t.Fatal("a body with a frontmatter is not named")
	}
	clean := DeckObjectInput{Title: "T", Body: "<!-- slide: layout=cover -->\n# T\n\n---\n\n## One\n- a\n"}
	if has(LintObjectDeck(clean, g)) {
		t.Fatal("a body of slides is named as a frontmatter")
	}
	rule := DeckObjectInput{Title: "T", Body: "---\n## One\n- a\n---\n## Two\n"}
	if has(LintObjectDeck(rule, g)) {
		t.Fatal("slides separated by --- are named as a frontmatter")
	}
}

// The object lint checks every Deck setting the panel shows: a mark that is
// neither on nor off is named (the lint once left mark and watermark out).
func TestObjectLintChecksTheMark(t *testing.T) {
	in := DeckObjectInput{Title: "T", Deck: DeckObjectFields{Mark: "maybe"}, Body: "## One\n- a\n"}
	for _, f := range LintObjectDeck(in, Grammar{}).Findings {
		if strings.Contains(f.Message, "mark") {
			return
		}
	}
	t.Fatalf("mark: maybe passed the object lint: %+v", LintObjectDeck(in, Grammar{}).Findings)
}
