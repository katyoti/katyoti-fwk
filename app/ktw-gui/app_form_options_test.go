// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// A tiny universe catalog: deck themes from two sidecars.
type fakeCatalog struct{ entries []contribapi.Contribution }

func (f fakeCatalog) List(context.Context) ([]*mbapi.Manifest, error) { return nil, nil }
func (f fakeCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, c := range f.entries {
		if c.Type == t {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f fakeCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	for _, c := range f.entries {
		if c.ResolvedURI() == uri {
			return c.Data(), nil
		}
	}
	return nil, nil
}

// The launch form the GUI serves lists the themes in reach — and a theme a
// new sidecar publishes appears with no change to the form.
func TestResolveFormOptionsListsThemesInReach(t *testing.T) {
	theme := func(provider, name, desc string) contribapi.Contribution {
		return contribapi.Contribution{
			Provider: provider, Type: "deck-theme", ID: name,
			Metadata: &contribapi.ContributionMeta{Name: name, Description: desc},
			Layers:   contribapi.InlineData(json.RawMessage(`{"name":"` + name + `"}`)),
		}
	}
	form := func() *coreschemaapi.Schema {
		return &coreschemaapi.Schema{Name: "slidedeck-fast-create-input", Root: "In", Types: map[string]*coreschemaapi.Type{
			"In": {Kind: coreschemaapi.KindObject, Fields: map[string]*coreschemaapi.Type{
				"theme": {Kind: coreschemaapi.KindString, Content: "text", ContentParams: map[string]any{
					"options_from": map[string]any{"source": "catalog", "kind": "deck-theme", "value": "name", "label": "description"},
				}},
			}},
		}}
	}
	cat := fakeCatalog{entries: []contribapi.Contribution{
		theme("ktw-docs", "katyoti", "KaTyoti"),
		theme("ktw-demo", "demo", "Démo"),
	}}

	s := form()
	resolveFormOptions(context.Background(), s, cat)
	if got := s.Types["In"].Fields["theme"].Enum; !reflect.DeepEqual(got, []any{"demo", "katyoti"}) {
		t.Fatalf("enum = %v", got)
	}

	cat.entries = append(cat.entries, theme("ktw-acme", "acme", "ACME"))
	s = form()
	resolveFormOptions(context.Background(), s, cat)
	if got := s.Types["In"].Fields["theme"].Enum; !reflect.DeepEqual(got, []any{"acme", "demo", "katyoti"}) {
		t.Fatalf("a new sidecar's theme must appear: enum = %v", got)
	}
}

// fakeThemeObjects is a universe's CWM holding one project theme.
type fakeThemeObjects struct{}

func (fakeThemeObjects) ObjectList(context.Context, cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	return cwmapi.ObjectListOutput{Objects: []cwmapi.Object{{
		ID: "talks.acme", Type: deckapi.ObjectTypeDeckTheme,
		Capabilities: map[string]map[string]any{deckapi.CapabilityDeckTheme: {"name": "acme", "description": "Acme"}},
	}}}, nil
}

func (fakeThemeObjects) ObjectFileRead(context.Context, cwmapi.ObjectFileReadInput) (cwmapi.ObjectFileReadOutput, error) {
	return cwmapi.ObjectFileReadOutput{}, nil
}

// With the deck-themes source, a theme imported into the project is offered
// beside the sidecars' themes, the moment it is stored.
func TestResolveFormOptionsOffersProjectThemes(t *testing.T) {
	s := &coreschemaapi.Schema{Name: "slidedeck-fast-create-input", Root: "In", Types: map[string]*coreschemaapi.Type{
		"In": {Kind: coreschemaapi.KindObject, Fields: map[string]*coreschemaapi.Type{
			"theme": {Kind: coreschemaapi.KindString, Content: "text", ContentParams: map[string]any{
				"options_from": map[string]any{"source": "deck-themes", "value": "name", "label": "description"},
			}},
		}},
	}}
	cat := fakeCatalog{entries: []contribapi.Contribution{{
		Provider: "ktw-docs", Type: "deck-theme", ID: "katyoti",
		Metadata: &contribapi.ContributionMeta{Name: "katyoti", Description: "KaTyoti"},
		Layers:   contribapi.InlineData(json.RawMessage(`{"name":"katyoti","description":"KaTyoti"}`)),
	}}}
	resolveFormOptions(deckapi.WithThemeObjects(context.Background(), fakeThemeObjects{}), s, cat)
	got := s.Types["In"].Fields["theme"].Enum
	if !reflect.DeepEqual(got, []any{"acme", "katyoti"}) {
		t.Fatalf("enum = %v, want the project theme beside the sidecar's", got)
	}
}

// fakeTemplateObjects is a universe's CWM holding one project template.
type fakeTemplateObjects struct{}

func (fakeTemplateObjects) ObjectList(context.Context, cwmapi.ObjectListInput) (cwmapi.ObjectListOutput, error) {
	return cwmapi.ObjectListOutput{Objects: []cwmapi.Object{{
		ID: "talks.our-review", Type: docsapi.ObjectTypeContentTemplate,
		Capabilities: map[string]map[string]any{docsapi.CapabilityContentTemplate: {
			"name": "our-review", "description": "Our review", "target": "dossier",
			"sections": []any{map[string]any{"id": "a", "title": "A", "intent": "B"}},
		}},
	}}}, nil
}

// With the content-templates source, the project's templates come beside the
// sidecars', partial ones are never offered, and `target` keeps those about
// that object type (and those about any).
func TestResolveFormOptionsOffersContentTemplates(t *testing.T) {
	tpl := func(name, body string) contribapi.Contribution {
		return contribapi.Contribution{
			Provider: "ktw-docs", Type: docsapi.ContributionTypeContentTemplate, ID: name,
			Metadata: &contribapi.ContributionMeta{Name: name},
			Layers:   contribapi.InlineData(json.RawMessage(body)),
		}
	}
	cat := fakeCatalog{entries: []contribapi.Contribution{
		tpl("architecture-board", `{"name":"architecture-board","label":"Comité d'architecture","description":"x","target":"dossier","sections":[]}`),
		tpl("steering-committee", `{"name":"steering-committee","description":"x","sections":[]}`),
		tpl("risks", `{"name":"risks","description":"x","partial":true,"sections":[]}`),
		tpl("module-review", `{"name":"module-review","description":"x","target":"module","sections":[]}`),
	}}
	s := &coreschemaapi.Schema{Name: "deck-from-object-input", Root: "In", Types: map[string]*coreschemaapi.Type{
		"In": {Kind: coreschemaapi.KindObject, Fields: map[string]*coreschemaapi.Type{
			"template": {Kind: coreschemaapi.KindString, Content: "text", ContentParams: map[string]any{
				"options_from": map[string]any{"source": "content-templates", "value": "name", "label": "label", "target": "dossier"},
			}},
		}},
	}}
	resolveFormOptions(docsapi.WithTemplateObjects(context.Background(), fakeTemplateObjects{}), s, cat)
	if got := s.Types["In"].Fields["template"].Enum; !reflect.DeepEqual(got, []any{"architecture-board", "our-review", "steering-committee"}) {
		t.Fatalf("enum = %v", got)
	}
}
