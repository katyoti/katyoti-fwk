// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"encoding/json"
	deckcore "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	oclocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/local"
	paneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	panelocal "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/local"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

const slides = `<!-- slide: layout=cover -->
# A deck in the workspace
Kept in a module, shown on its pane

---

<!-- slide: steps=list -->
## Why an object
- Listed and linked like any object
- Its settings are fields
- Its slides are its body

Note:
The body holds only the slides.`

// A slide-deck object is a .cwm.md in decks/, and its pane shows the Deck panel:
// a deck composed from the object's fields and its body, which lints clean.
func TestADeckObjectShowsItsDeck(t *testing.T) {
	ctx, cwm := cwmapp.New(t,
		// `content` is ktw-methods'; the test carries its name only.
		cwmapp.Pack(fstest.MapFS{"capabilities/content.yaml": {Data: []byte("name: content\n")}}),
		contrib.Pack("ktw-test"),
		oclocal.Extension(),
		panelocal.Extension(),
	)
	cwmapp.Module(t, ctx, cwm, "talks")

	body, _ := json.Marshal(slides)
	obj := cwmapi.NewRecordObject("talks", "deck-objects", "slide-deck", "talks", "Decks: what's in a name?", body)
	obj.Capabilities["deck"] = map[string]any{"aspect": "4:3", "transition": "fade"}
	created, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "talks", Object: obj})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	pane, err := application.UseService[paneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pane.Detail(ctx, paneapi.DetailInput{ID: created.Object.ID})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	var deck *paneapi.ObjectPanel
	for i, p := range out.Panels {
		if p.Name == "Deck" && p.Kind == "source" {
			deck = &out.Panels[i]
		}
	}
	if deck == nil || deck.Source != "ktw-docs:deck/view" {
		t.Fatalf("no Deck panel sourced from deck/view in %+v", out.Panels)
	}
	for _, p := range out.Panels {
		if p.Kind == "fields" && p.Name == "Deck" {
			t.Errorf("the settings tab must not share the deck panel's name")
		}
	}
	def, err := deckcore.Service{}.View(ctx, deckapi.ViewInput{ID: created.Object.ID})
	if err != nil || def.Kind != "deck" {
		t.Fatalf("deck/view: %+v, %v", def, err)
	}
	md, _ := def.Data.(string)
	for _, want := range []string{`title: "Decks: what's in a name?"`, `aspect: "4:3"`, `transition: "fade"`, "production_mark: ", "# A deck in the workspace", "Note:"} {
		if !strings.Contains(md, want) {
			t.Errorf("composed deck lacks %q:\n%s", want, md)
		}
	}
	if fs := deckapi.Lint(md, deckapi.Grammar{Layouts: layouts(t)}); len(fs) != 0 {
		t.Errorf("the composed deck has findings: %+v\n%s", fs, md)
	}
	if n := len(deckapi.Parse(md).Slides); n != 2 {
		t.Errorf("composed deck has %d slides, want 2", n)
	}
	if p := filepath.ToSlash(created.Path); !strings.Contains(p, "/decks/") || !strings.HasSuffix(p, ".cwm.md") {
		t.Errorf("a deck object is a .cwm.md in decks/, got %q", p)
	}

	// The drawings are a named file of the object, beside it in decks/.
	drawings := []byte(`[{"width":1280,"height":720,"data":[]}]`)
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: created.Object.ID, Name: "drawings", Content: drawings}); err != nil {
		t.Fatalf("write drawings: %v", err)
	}
	back, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: created.Object.ID, Name: "drawings"})
	if err != nil || string(back.Content) != string(drawings) {
		t.Fatalf("read drawings: %v %q", err, back.Content)
	}
	// The review too (OD-017): a named file beside it, the body untouched.
	review := []byte(`{"threads":[{"id":"t1","slide":{"index":1,"title":"Two"},"comments":[{"author":"Ann","text":"x","at":"2026-10-04T10:00:00Z"}]}]}`)
	wrote, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: created.Object.ID, Name: "review", Content: review})
	if err != nil {
		t.Fatalf("write review: %v", err)
	}
	if p := filepath.ToSlash(wrote.File.Path); !strings.Contains(p, "decks/") || !strings.HasSuffix(p, ".review.json") {
		t.Errorf("the review is a .review.json beside the deck, got %q", p)
	}
	if back, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: created.Object.ID, Name: "review"}); err != nil || string(back.Content) != string(review) {
		t.Fatalf("read review: %v %q", err, back.Content)
	}

	// A chat on the deck object is told the object IS the deck — write the
	// slides into its body — and gets the deck-author skill.
	oc, err := application.UseService[ocapi.ObjectContextService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	ac, err := oc.AgentContext(ctx, ocapi.AgentContextInput{ID: created.Object.ID})
	if err != nil {
		t.Fatalf("agent context: %v", err)
	}
	var framed bool
	for _, sec := range ac.System {
		if sec.ID == "slide-deck-object" {
			framed = strings.Contains(sec.Text, "cwm-object-update") &&
				strings.Contains(sec.Text, "Decks: what's in a name?") &&
				strings.Contains(sec.Text, "do NOT write a .deck.md")
		}
	}
	if !framed {
		t.Errorf("a chat on a deck object lacks the slide-deck framing: %+v", ac.System)
	}
	var skill bool
	for _, sk := range ac.Skills {
		skill = skill || strings.Contains(sk.Selector, "deck-author")
	}
	if !skill {
		t.Errorf("a chat on a deck object is not offered deck-author: %+v", ac.Skills)
	}
}

// layouts reads the built-in layouts the way the catalog decodes them.
func layouts(t *testing.T) []deckapi.LayoutBody {
	t.Helper()
	files, _ := filepath.Glob("layouts/*.yaml")
	var out []deckapi.LayoutBody
	for _, f := range files {
		raw, _ := os.ReadFile(f)
		var l deckapi.LayoutBody
		if err := yamlToStruct(raw, &l); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, l)
	}
	return out
}
