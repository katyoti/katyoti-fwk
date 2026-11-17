// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// The deck editor's settings come from the grammar's types, in declaration
// order, with the values they allow: nothing is written in the panel.
func TestDeckEditorSettingFields_FromTheGrammarTypes(t *testing.T) {
	deck := settingFields[deckapi.DeckSettings]()
	byName := map[string]DeckEditorField{}
	for _, f := range deck {
		byName[f.Name] = f
	}
	for _, k := range []string{"title", "aspect", "theme", "transition", "author"} {
		if _, ok := byName[k]; !ok {
			t.Errorf("deck setting %q missing: %+v", k, deck)
		}
	}
	if got := byName["aspect"].Enum; len(got) != 2 || got[0] != "16:9" {
		t.Errorf("aspect values: %v", got)
	}
	if byName["theme"].Description == "" {
		t.Error("theme has no description")
	}
	object := settingFields[deckapi.DeckObjectFields]()
	if len(object) != 6 {
		t.Errorf("a deck object keeps aspect, theme, transition, author, watermark, mark as fields: %+v", object)
	}
	slide := settingFields[deckapi.SlideSettings]()
	names := map[string]bool{}
	for _, f := range slide {
		names[f.Name] = true
	}
	for _, k := range []string{"layout", "variant", "background", "transition", "steps"} {
		if !names[k] {
			t.Errorf("slide setting %q missing: %+v", k, slide)
		}
	}
}

// A theme offers its variants by layout; a layout without variants is left out.
func TestDeckEditorThemes_VariantsByLayout(t *testing.T) {
	th := deckapi.ThemeBody{Name: "acme", Description: "ACME", Palette: []deckapi.ThemeColour{{Name: "Rouge cavalier", Value: "#82072A"}}, Layouts: map[string]deckapi.ThemeLayout{
		"cover":   {Variants: map[string]deckapi.ThemeVariant{"sand": {}, "cream": {}}},
		"section": {},
	}}
	got := editorThemes([]deckapi.ThemeBody{th})
	if len(got) != 1 || got[0].Name != "acme" {
		t.Fatalf("themes: %+v", got)
	}
	if v := got[0].Variants["cover"]; len(v) != 2 {
		t.Errorf("cover variants: %v", v)
	}
	if len(got[0].Palette) != 1 || got[0].Palette[0].Value != "#82072A" {
		t.Errorf("palette: %+v", got[0].Palette)
	}
	if _, ok := got[0].Variants["section"]; ok {
		t.Errorf("a layout without variants is listed: %+v", got[0].Variants)
	}
}
