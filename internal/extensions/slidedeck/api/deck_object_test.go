// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// A deck file becomes an object's parts and back: the title and settings
// leave the frontmatter for the fields, the slides stay the body.
func TestDeckObjectOfIsComposeDeckInverse(t *testing.T) {
	md := "---\ntitle: \"Q4 review\"\ntheme: acme\naspect: \"16:9\"\nwatermark: BROUILLON\n---\n\n<!-- slide: layout=cover -->\n# Q4 review\n\n---\n\n## Costs\n- a\n"
	got := DeckObjectOf(md)
	if got.Title != "Q4 review" || got.Deck.Theme != "acme" || got.Deck.Aspect != "16:9" || got.Deck.Watermark != "BROUILLON" {
		t.Fatalf("parts = %+v", got)
	}
	if got.Body != "<!-- slide: layout=cover -->\n# Q4 review\n\n---\n\n## Costs\n- a\n" {
		t.Fatalf("body = %q", got.Body)
	}
	if bodyHasFrontmatter(got.Body) {
		t.Fatal("the body kept a frontmatter")
	}
	back := DeckObjectOf(ComposeDeck(got))
	if back.Title != got.Title || back.Deck != got.Deck || back.Body != got.Body {
		t.Fatalf("round trip: %+v != %+v", back, got)
	}
}
