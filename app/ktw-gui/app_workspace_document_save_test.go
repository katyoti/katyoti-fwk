// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// A deck shown in a chat, saved without a place: the drafts module is made,
// the deck lands in it as a slide-deck — its title the name, its frontmatter
// the Deck fields, its body only the slides.
func TestDocumentSave_ADeckBecomesASlideDeckInTheDrafts(t *testing.T) {
	s := &multiStub{t: t,
		errs:    map[string]error{"ktw-agents:cwm/object-get": errors.New("not found")},
		replies: map[string]any{"ktw-agents:cwm/object-create": cwmapi.ObjectCreateOutput{Object: objWithTitle("drafts.q4-review", "slide-deck", "Q4 review")}},
	}
	md := "---\ntitle: Q4 review\ntheme: acme\n---\n\n# Q4 review\n\n---\n\n## Costs\n"
	got, err := documentSaveVia(context.Background(), s, "", render.MediaTypeDeck, "", md)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.calls) != 3 {
		t.Fatalf("calls: %v (get drafts, create drafts, create deck)", s.paths())
	}
	mod := s.calls[1].input.(cwmapi.ObjectCreateInput)
	if mod.Module != "drafts" || mod.Object.ID != "drafts" || mod.Object.Type != "documentation" {
		t.Errorf("drafts module: %+v", mod)
	}
	in := s.calls[2].input.(cwmapi.ObjectCreateInput)
	o := in.Object
	if in.Module != "drafts" || o.Parent != "drafts" || o.Type != "slide-deck" || o.Name != "Q4 review" {
		t.Errorf("deck: module %q %+v", in.Module, o)
	}
	if o.Capabilities["deck"]["theme"] != "acme" || o.Body != "# Q4 review\n\n---\n\n## Costs\n" {
		t.Errorf("fields %v body %q", o.Capabilities, o.Body)
	}
	if got.Object.ID != "drafts.q4-review" {
		t.Errorf("result %+v", got)
	}
}

// Saved under an object, markdown becomes a document in that object's module;
// another kind of document is refused with what to do instead.
func TestDocumentSave_MarkdownUnderAnObject(t *testing.T) {
	s := &multiStub{t: t, replies: map[string]any{"ktw-agents:cwm/object-create": cwmapi.ObjectCreateOutput{Object: objWithTitle("live.notes", "document", "Notes")}}}
	if _, err := documentSaveVia(context.Background(), s, "live.dossier-1", render.MediaTypeMarkdown, "", "intro\n\n# Notes\n- a\n"); err != nil {
		t.Fatal(err)
	}
	in := s.inputFor("ktw-agents:cwm/object-create").(cwmapi.ObjectCreateInput)
	if in.Module != "live" || in.Object.Parent != "live.dossier-1" || in.Object.Type != "document" || in.Object.Name != "Notes" {
		t.Errorf("document: %+v", in)
	}
	if _, err := documentSaveVia(context.Background(), &multiStub{t: t}, "live.x", "image/png", "p", "x"); err == nil {
		t.Error("a picture was taken as an object")
	}
}
