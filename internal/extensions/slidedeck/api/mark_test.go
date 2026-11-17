// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"strings"
	"testing"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
)

// MarkDeck writes the one production mark (docsapi) into a deck's
// frontmatter, replacing a stale one, and none when the deck's mark is off.
func TestMarkDeck(t *testing.T) {
	ctx := context.Background()
	mark := func(md string) any { return Parse(MarkDeck(ctx, md)).Settings["production_mark"] }

	got, _ := mark("---\ntitle: A talk\n---\n\n# A talk\n").(string)
	if !strings.HasPrefix(got, docsapi.DefaultMarkText+" · ") {
		t.Fatalf("production_mark = %q, want the docs mark", got)
	}
	got, _ = mark("---\ntitle: A talk\nproduction_mark: \"stale\"\nclassification: Interne\n---\n\n# A talk\n").(string)
	if strings.Contains(got, "stale") || !strings.HasSuffix(got, " · Interne") {
		t.Fatalf("a stale line is replaced and the deck's classification read: %q", got)
	}
	if got, _ = mark("# A talk\n").(string); !strings.HasPrefix(got, docsapi.DefaultMarkText) {
		t.Fatalf("a deck without frontmatter gets one: %q", got)
	}
	for _, off := range []string{"---\nmark: off\n---\n\n# A talk\n", "---\nmark: off\nproduction_mark: \"old\"\n---\n\n# A talk\n"} {
		if v, ok := Parse(MarkDeck(ctx, off)).Settings["production_mark"]; ok {
			t.Fatalf("a deck whose mark is off carries %v", v)
		}
	}
	if d := Parse(MarkDeck(ctx, "---\ntitle: A talk\n---\n\n# One\n\n---\n\n## Two\n")); len(d.Slides) != 2 {
		t.Fatalf("the slides are kept: %d", len(d.Slides))
	}
}
