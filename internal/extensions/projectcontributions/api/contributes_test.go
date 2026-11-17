// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// moduleFiles is a module's files by path, read as CWM reads them.
type moduleFiles map[string][]byte

func (m moduleFiles) ObjectFileRead(_ context.Context, in cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error) {
	b, ok := m[in.Path]
	if !ok {
		return cwmapi.ObjectFileReadOutput{}, errors.New("no file " + in.Path)
	}
	return cwmapi.ObjectFileReadOutput{Content: b}, nil
}

func deckTheme() cwmapi.Object {
	return cwmapi.Object{ID: "acme-blue", Type: "deck-theme", Name: "Acme blue",
		Capabilities: map[string]map[string]any{
			"deck-theme": {"name": "acme-blue", "primary": "#003366"},
			CapContributes: {"kind": "deck-theme", "layers": map[string]any{
				"data": "capabilities.deck-theme", "logo": "files/themes/acme-blue/logo.svg"}},
		}}
}

// A deck theme authored as an object is served as an entry of its project's
// token: its URI from the kind and the object's id, its data layer the
// capability's field-group, a further layer read from a file of its module.
func TestAnObjectIsPublishedAsAnEntryOfItsKind(t *testing.T) {
	files := moduleFiles{"themes/acme-blue/logo.svg": []byte("<svg/>")}
	e, err := EntryOfObject(context.Background(), deckTheme(), "archi-dossiers", files)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.URI(); got != "archi-dossiers:/deck-theme/acme-blue" {
		t.Fatalf("URI = %s, want archi-dossiers:/deck-theme/acme-blue", got)
	}
	var data map[string]any
	if err := json.Unmarshal(e.Entry.Data(), &data); err != nil || data["primary"] != "#003366" {
		t.Fatalf("data layer = %s (%v), want the deck-theme field-group", e.Entry.Data(), err)
	}
	if l := e.Entry.Layers["logo"]; string(l.Inline) != "<svg/>" || l.MediaType != "image/svg+xml" {
		t.Fatalf("logo layer = %+v, want the module file as image/svg+xml", l)
	}
	if e.Object != "acme-blue" || e.Overrides != "" {
		t.Fatalf("entry = %+v, want object acme-blue overriding nothing", e)
	}
}

// The id defaults to the object's slug, else its id; body and fields are
// sources too.
func TestTheEntryIdAndSources(t *testing.T) {
	o := cwmapi.Object{ID: "docs.T-7", Slug: "intro", Body: "# Hello", Fields: map[string]any{"title": "Hi"},
		Capabilities: map[string]map[string]any{CapContributes: {"kind": "reference",
			"layers": map[string]any{"source": "body", "data": "fields", "title": "fields.title"}}}}
	e, err := EntryOfObject(context.Background(), o, "acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.URI() != "acme:/reference/intro" {
		t.Fatalf("URI = %s, want the slug as id", e.URI())
	}
	if string(e.Entry.Source()) != "# Hello" || string(e.Entry.Layers["title"].Inline) != `"Hi"` {
		t.Fatalf("layers = %+v", e.Entry.Layers)
	}
	o.Capabilities[CapContributes]["id"] = "custom"
	if e, _ := EntryOfObject(context.Background(), o, "acme", nil); e.URI() != "acme:/reference/custom" {
		t.Fatalf("URI = %s, want the declared id", e.URI())
	}
}

// What the capability cannot publish is refused, saying why.
func TestAnUnservableDeclarationIsRefused(t *testing.T) {
	cases := map[string]map[string]any{
		"no layer":           {"kind": "persona"},
		"unknown source":     {"kind": "persona", "layers": map[string]any{"data": "elsewhere"}},
		"file leaves module": {"kind": "persona", "layers": map[string]any{"data": "files/../x.yaml"}},
		"folder":             {"kind": "persona", "layers": map[string]any{"data": "files/themes/"}},
		"missing field":      {"kind": "persona", "layers": map[string]any{"data": "fields.nope"}},
		"itself":             {"kind": "persona", "layers": map[string]any{"data": "capabilities.contributes"}},
		"bad kind":           {"kind": "Persona", "layers": map[string]any{"source": "body"}},
		"override of a kind": {"kind": "persona", "overrides": "ktw-agents:/task/reviewer", "layers": map[string]any{"source": "body"}},
		"override of itself": {"kind": "persona", "overrides": "acme:/persona/reviewer", "layers": map[string]any{"source": "body"}},
		"empty layer":        {"kind": "persona", "layers": map[string]any{"source": "body"}},
	}
	for name, group := range cases {
		o := cwmapi.Object{ID: "x", Fields: map[string]any{"a": 1}, Capabilities: map[string]map[string]any{CapContributes: group}}
		if name != "empty layer" {
			o.Body = "text"
		}
		if _, err := EntryOfObject(context.Background(), o, "acme", moduleFiles{}); err == nil {
			t.Errorf("%s: accepted, want refused", name)
		}
	}
	if _, err := EntryOfObject(context.Background(), cwmapi.Object{ID: "plain"}, "acme", nil); err == nil {
		t.Error("an object without the capability was published")
	}
}

// An override carries only the layers it replaces: the French text of one
// persona overrides that layer, and the prompt is inherited.
func TestAnOverrideReplacesOnlyItsLayers(t *testing.T) {
	base := contribapi.Contribution{Provider: "ktw-agents", Type: "persona", ID: "reviewer"}
	base.SetSource([]byte("You review."), "text/markdown")
	base.SetLayer("i18n.fr", []byte("Tu relis."), "text/markdown")
	o := cwmapi.Object{ID: "reviewer-fr", Body: "Vous relisez.", Capabilities: map[string]map[string]any{CapContributes: {
		"kind": "persona", "overrides": "ktw-agents:/persona/reviewer", "layers": map[string]any{"i18n.fr": "body"}}}}
	ov, err := EntryOfObject(context.Background(), o, "acme", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, record, err := contribapi.ApplyOverride(base, ov.AsOverride(true))
	if err != nil {
		t.Fatal(err)
	}
	if got.ResolvedURI() != "ktw-agents:/persona/reviewer" {
		t.Fatalf("served as %s, want the overridden entry's URI", got.ResolvedURI())
	}
	if string(got.Source()) != "You review." || string(got.Layers["i18n.fr"].Inline) != "Vous relisez." {
		t.Fatalf("layers = %+v, want the prompt inherited and the French layer replaced", got.Layers)
	}
	if string(base.Layers["i18n.fr"].Inline) != "Tu relis." {
		t.Fatal("the overridden entry was changed in place")
	}
	if len(record) != 1 || record[0] != (contribapi.Override{URI: "ktw-agents:/persona/reviewer", Layer: "i18n.fr", From: "project:acme"}) {
		t.Fatalf("override record = %+v", record)
	}
	other := contribapi.Contribution{Provider: "ktw-agents", Type: "persona", ID: "coder"}
	if _, _, err := contribapi.ApplyOverride(other, ov.AsOverride(true)); err == nil {
		t.Fatal("an override was applied to an entry it does not name")
	}
}

// A file and an object, or two objects, serving one entry are both named.
func TestAnEntryServedTwiceIsAClash(t *testing.T) {
	file := contribapi.Contribution{Provider: "acme", Type: "skill", ID: "deploy"}
	obj := ObjectEntry{Entry: contribapi.Contribution{Provider: "acme", Type: "skill", ID: "deploy"}, Object: "ops.deploy"}
	alone := ObjectEntry{Entry: contribapi.Contribution{Provider: "acme", Type: "skill", ID: "other"}, Object: "ops.other"}
	got := Clashes([]contribapi.Contribution{file}, []ObjectEntry{obj, alone})
	if len(got) != 1 || got[0].URI != "acme:/skill/deploy" {
		t.Fatalf("clashes = %+v, want acme:/skill/deploy only", got)
	}
	f := got[0].Finding()
	if !strings.Contains(f, "file acme:/skill/deploy") || !strings.Contains(f, "object ops.deploy") {
		t.Fatalf("finding %q does not name both sources", f)
	}
}
