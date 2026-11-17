// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	stdreflect "reflect"
	"strings"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// DeckEditorField is one setting the deck editor shows: its key as written,
// what it means and, when the grammar restricts it, its values.
type DeckEditorField struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// DeckEditorTheme is a theme the editor offers: its name, its description,
// the variants it declares, by layout, and its palette — the only colours a
// deck in it may set, when it declares one.
type DeckEditorTheme struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Variants    map[string][]string   `json:"variants,omitempty"`
	Palette     []deckapi.ThemeColour `json:"palette,omitempty"`
}

// DeckEditorGrammarResult is what the deck settings panel needs to offer
// choices without writing any down: the deck's and a slide's settings (from
// the grammar's Go types), and the layouts and themes in the universe.
type DeckEditorGrammarResult struct {
	Deck  []DeckEditorField `json:"deck"`
	Slide []DeckEditorField `json:"slide"`
	// Object is the deck settings a slide-deck object keeps as fields (its
	// `deck` group); the others it has elsewhere (the title is its name).
	Object  []DeckEditorField    `json:"object"`
	Layouts []deckapi.LayoutBody `json:"layouts"`
	Themes  []DeckEditorTheme    `json:"themes"`
	Error   string               `json:"error,omitempty"`
}

// DeckEditorGrammar returns the choices of the deck settings panel for a
// universe (modules/slidedeck/, the Deck component): the settings a deck and
// a slide declare, the layouts and the themes in reach with their variants.
//
// Wails-bound: window.go.main.App.DeckEditorGrammar(universeName).
func (a *App) DeckEditorGrammar(universeName string) DeckEditorGrammarResult {
	out := DeckEditorGrammarResult{
		Deck:   settingFields[deckapi.DeckSettings](),
		Slide:  settingFields[deckapi.SlideSettings](),
		Object: settingFields[deckapi.DeckObjectFields](),
	}
	cat, err := a.universeCatalog(universeName)
	if err != nil {
		out.Error = fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)
		return out
	}
	g, err := deckapi.GrammarAt(a.deckThemeCtx(universeName), mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Layouts = g.Layouts
	out.Themes = editorThemes(g.Themes)
	return out
}

// editorThemes keeps of each theme what the editor offers: its name, its
// description, its variants by layout, its palette.
func editorThemes(themes []deckapi.ThemeBody) []DeckEditorTheme {
	out := make([]DeckEditorTheme, 0, len(themes))
	for _, t := range themes {
		e := DeckEditorTheme{Name: t.Name, Description: t.Description, Palette: t.Palette}
		for l := range t.Layouts {
			if v := t.VariantNames(l); len(v) > 0 {
				if e.Variants == nil {
					e.Variants = map[string][]string{}
				}
				e.Variants[l] = v
			}
		}
		out = append(out, e)
	}
	return out
}

// settingFields lists T's settings in declaration order, each with the
// description and the values its type declares (reflect.TypeAST: the json
// and jsonschema tags).
func settingFields[T any]() []DeckEditorField {
	ast, err := corereflect.TypeAST[T]()
	if err != nil || ast == nil {
		return nil
	}
	var out []DeckEditorField
	for _, f := range stdreflect.VisibleFields(stdreflect.TypeFor[T]()) {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		ft, ok := ast.Fields[name]
		if name == "" || name == "-" || !ok || ft == nil {
			continue
		}
		e := DeckEditorField{Name: name, Description: ft.Description}
		for _, v := range ft.Enum {
			e.Enum = append(e.Enum, fmt.Sprint(v))
		}
		out = append(out, e)
	}
	return out
}
