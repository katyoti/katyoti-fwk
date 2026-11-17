// SPDX-License-Identifier: Apache-2.0

// Package api is the slide-deck grammar: what a deck may declare (its
// settings, a slide's settings, a layout), the parsed model of a deck, the
// splitter that produces it and the lint that checks a deck against the
// declared grammar. Parse and Lint need nothing but the source (and the
// layouts to check against); Layouts reads those from the catalog. The display
// tool, the CLI and the object lint hook all import this package.
//
// The declared types are the one source of the grammar: datatypegen renders
// each as a DSL data-type (published by ktw-docs) and as the TypeScript
// interface the deck component imports. See modules/slidedeck/.
package api

// ContributionTypeDeckLayout is the contribution kind of a layout declaration.
const ContributionTypeDeckLayout = "deck-layout"

// DefaultLayout is the layout of a slide that names none.
const DefaultLayout = "content"

// DefaultSlot is the slot holding a slide's markdown before its first
// `::name::` marker.
const DefaultSlot = "default"

// DeckSettings is a deck's frontmatter: the settings of the whole deck.
type DeckSettings struct {
	Title string `json:"title" jsonschema:"description=The deck title shown on the cover and in lists"`
	// The settings a deck object keeps as its fields, declared once
	// (DeckObjectFields): a deck file writes the same keys in its frontmatter.
	DeckObjectFields `yaml:",inline"`
	// ProductionMark is written by ArchiFactory, never by an author.
	ProductionMark string `json:"production_mark,omitempty" yaml:"production_mark,omitempty" jsonschema:"description=The production mark's line ArchiFactory writes when it shows or exports the deck (MarkDeck); the theme places it; an author writes mark on or off instead"`
}

// DeckObjectFields is the `deck` field-group of a slide-deck OBJECT: the deck
// settings as object fields. The title is the object's name and the slides
// are its body (the object core), so neither is here; the deck panel composes the
// deck's frontmatter from these fields and the body.
type DeckObjectFields struct {
	Aspect     string `json:"aspect,omitempty"     yaml:"aspect,omitempty"     jsonschema:"description=Slide aspect ratio; 16:9 when omitted,enum=16:9|4:3"`
	Theme      string `json:"theme,omitempty"      yaml:"theme,omitempty"      jsonschema:"description=Theme name; one of the deck-theme contributions; katyoti when omitted"`
	Transition string `json:"transition,omitempty" yaml:"transition,omitempty" jsonschema:"description=Default transition between slides; slide when omitted,enum=none|fade|slide|convex|concave|zoom"`
	Author     string `json:"author,omitempty"     yaml:"author,omitempty"     jsonschema:"description=Who wrote the deck"`
	Watermark  string `json:"watermark,omitempty"  yaml:"watermark,omitempty"  jsonschema:"description=A watermark across every slide such as BROUILLON or INTERNE or CONFIDENTIEL; the theme's when omitted; none removes it"`
	Mark       string `json:"mark,omitempty"       yaml:"mark,omitempty"       jsonschema:"description=The production mark (docs ProductionMark: ArchiFactory and its version and the date) at the theme's place; on when omitted; off removes it,enum=on|off"`
}

// SlideSettings is what a slide may declare on its first line, as
// `<!-- slide: key=value … -->`.
type SlideSettings struct {
	Layout     string `json:"layout,omitempty"     jsonschema:"description=Layout of the slide; one of the deck-layout contributions; content when omitted"`
	Variant    string `json:"variant,omitempty"    jsonschema:"description=One look of the layout declared by the deck's theme; the layout's default look when omitted"`
	Background string `json:"background,omitempty" jsonschema:"description=Background: a colour or an image path"`
	Transition string `json:"transition,omitempty" jsonschema:"description=Transition into this slide,enum=none|fade|slide|convex|concave|zoom"`
	Steps      string `json:"steps,omitempty"      jsonschema:"description=Reveal steps: list shows the slide's list items one at a time,enum=list|none"`
	Class      string `json:"class,omitempty"      jsonschema:"description=Extra CSS classes for the slide"`
	Include    string `json:"include,omitempty"    jsonschema:"description=Shows here the slides of the slide group of this name; the slide holds nothing else"`
}

// LayoutBody is one layout declaration — the body of a deck-layout
// contribution. The layout's styling ships with the deck component; the
// declaration is what an agent lists and what the lint checks.
type LayoutBody struct {
	Name        string       `json:"name"              jsonschema:"description=Layout name as written in layout=<name>"`
	Description string       `json:"description"       jsonschema:"description=When to use this layout"`
	Slots       []LayoutSlot `json:"slots"             jsonschema:"description=The named regions of the layout; default is the markdown before the first slot marker"`
	Example     string       `json:"example,omitempty" jsonschema:"description=A complete example slide in markdown"`
}

// LayoutSlot is one named region of a layout.
type LayoutSlot struct {
	Name        string `json:"name"                  jsonschema:"description=Slot name as written in ::name:: (default for the unmarked region)"`
	Description string `json:"description,omitempty" jsonschema:"description=What goes in this slot"`
	Required    bool   `json:"required,omitempty"    jsonschema:"description=The slot must have content"`
	// A slot holding a list (figures, steps, cards) declares how many items
	// its layout arranges well; the lint names a list outside the range.
	MinItems int `json:"minItems,omitempty" jsonschema:"description=Fewest list items the slot takes"`
	MaxItems int `json:"maxItems,omitempty" jsonschema:"description=Most list items the slot takes; more do not fit the slide"`
}

// Deck is a parsed deck: the model the component renders and the lint checks.
// Parse always returns one, whatever the source — a stream cut anywhere
// parses to the slides it holds so far.
type Deck struct {
	Settings map[string]any `json:"settings"       jsonschema:"description=The frontmatter as written; DeckSettings declares its keys"`
	Slides   []Slide        `json:"slides"         jsonschema:"description=The slides in order"`
	Open     string         `json:"open,omitempty"     jsonschema:"description=The construct the source ended inside: frontmatter or code; empty when complete,enum=frontmatter|code"`
	OpenLine int            `json:"openLine,omitempty" jsonschema:"description=Source line where the open construct starts"`
}

// Slide is one slide of a parsed deck.
type Slide struct {
	H            int               `json:"h"                      jsonschema:"description=Horizontal position from 0"`
	V            int               `json:"v"                      jsonschema:"description=Vertical position under its horizontal slide from 0"`
	Line         int               `json:"line"                   jsonschema:"description=Source line of the slide's first line from 1"`
	Settings     map[string]string `json:"settings,omitempty"     jsonschema:"description=The key=value pairs of the settings line as written"`
	SettingsLine int               `json:"settingsLine,omitempty" jsonschema:"description=Source line of the settings line; 0 when there is none"`
	Slots        []SlotContent     `json:"slots"                  jsonschema:"description=The slide's markdown by slot in order"`
	Notes        string            `json:"notes,omitempty"        jsonschema:"description=Speaker notes: the markdown after Note:"`
	Group        string            `json:"group,omitempty"        jsonschema:"description=The slide group the slide comes from - set when a deck is shown with its groups"`
}

// SlotContent is the markdown of one slot of a slide.
type SlotContent struct {
	Name     string `json:"name"     jsonschema:"description=Slot name; default for the unmarked region"`
	Line     int    `json:"line"     jsonschema:"description=Source line of the slot marker or of the default slot's first line"`
	Markdown string `json:"markdown" jsonschema:"description=The slot's markdown with surrounding blank lines trimmed"`
}
