// SPDX-License-Identifier: Apache-2.0

// Package deckbody is the shape of the slide-deck contributions a sidecar
// ships: a deck theme, one file of a theme and a slide group. Standard library
// only, so the deck kinds' leaf (slidedeck/contrib/deckkind) can describe them
// without importing slidedeck/api, which the built-in kind reader could not
// import without a cycle. slidedeck/api names them under their usual names.
package deckbody

import (
	"regexp"
	"sort"
	"strings"
)

// ThemeBody is one theme declaration — the body of a deck-theme contribution.
// A theme styles the layouts and extends them with variants; it never changes
// what a deck may write, so a deck renders under any theme. Every asset path
// is relative to the theme's own folder.
type ThemeBody struct {
	Name        string                 `json:"name"                  jsonschema:"description=Theme name as written in a deck's theme: setting"`
	Description string                 `json:"description"           jsonschema:"description=What the theme is for"`
	Aspect      string                 `json:"aspect,omitempty"      jsonschema:"description=Aspect ratio the theme is drawn for; boxes are in its slide pixels (16:9 is 1280x720),enum=16:9|4:3"`
	Tokens      map[string]string      `json:"tokens,omitempty"      jsonschema:"description=Design tokens overriding the defaults: a name without its --deck- prefix mapped to a CSS value"`
	Dark        map[string]string      `json:"dark,omitempty"        jsonschema:"description=Token overrides for the dark colour scheme"`
	Fonts       []ThemeFont            `json:"fonts,omitempty"       jsonschema:"description=Font files the theme ships; a family without a file falls back to the token's font stack"`
	Chrome      *ThemeChrome           `json:"chrome,omitempty"      jsonschema:"description=What every slide carries (logo and rules and footer and slide number) unless its layout hides it"`
	Watermark   *ThemeWatermark        `json:"watermark,omitempty"   jsonschema:"description=The production mark the theme puts on its slides and a default watermark text; a deck's watermark and mark settings change them"`
	Layouts     map[string]ThemeLayout `json:"layouts,omitempty"     jsonschema:"description=Per layout name: whether it shows the chrome and its looks (the default and named variants)"`
	Icons       map[string]ThemeIcon   `json:"icons,omitempty"       jsonschema:"description=Pictograms a slide writes as icon:<name>; names are kebab-case ASCII"`
	Guidance    string                 `json:"guidance,omitempty"    jsonschema:"description=House rules an author follows when writing a deck in this theme (markdown)"`
	Notices     []string               `json:"notices,omitempty"     jsonschema:"description=Files that travel with the theme's other files and that no slide shows: a font's licence (its OFL.txt)"`
	Sample      string                 `json:"sample,omitempty"      jsonschema:"description=A deck beside the theme (a .deck.md) that shows its layouts and variants: what the theme preview displays"`
	Palette     []ThemeColour          `json:"palette,omitempty"     jsonschema:"description=The only colours a deck in this theme may set (a slide's background=): a brand charter's palette; empty when any colour may be used"`
}

// ThemeColour is one colour of a theme's palette.
type ThemeColour struct {
	Name  string `json:"name"  jsonschema:"description=The colour's name in the brand charter"`
	Value string `json:"value" jsonschema:"description=The colour as #rrggbb"`
}

// ThemeFont is one font file of a theme.
type ThemeFont struct {
	Family string `json:"family"           jsonschema:"description=CSS font family the tokens name"`
	Src    string `json:"src"              jsonschema:"description=The font file (woff2 or woff)"`
	Weight string `json:"weight,omitempty" jsonschema:"description=CSS font-weight of this file (400) or the range a variable font covers (400 700); 400 when omitted"`
	Style  string `json:"style,omitempty"  jsonschema:"description=CSS font-style of this file,enum=normal|italic"`
}

// ThemeChrome is what every slide of a themed deck carries.
type ThemeChrome struct {
	Logo        *ThemeImage `json:"logo,omitempty"        jsonschema:"description=A logo at a fixed place on the slide"`
	Rules       []ThemeRule `json:"rules,omitempty"       jsonschema:"description=Horizontal rules across the slide"`
	Footer      *ThemeText  `json:"footer,omitempty"      jsonschema:"description=A footer line; {title} is the deck title and {author} its author"`
	SlideNumber *ThemeText  `json:"slideNumber,omitempty" jsonschema:"description=Where the slide number goes; {n} is the number and {total} the count"`
}

// ThemeWatermark is what marks a theme's slides as produced, and as what.
type ThemeWatermark struct {
	Mark *ThemeText  `json:"mark,omitempty" jsonschema:"description=The production mark: a small text such as Une production ArchiFactory at its place; shown where the chrome shows (on every slide for a theme without chrome)"`
	Logo *ThemeImage `json:"logo,omitempty" jsonschema:"description=A logo with the production mark"`
	Text string      `json:"text,omitempty" jsonschema:"description=A watermark text across every slide by default (a deck's watermark setting replaces it)"`
}

// ThemeImage is an image placed on the slide.
type ThemeImage struct {
	Asset string `json:"asset" jsonschema:"description=The image file"`
	Box   Box    `json:"box"   jsonschema:"description=Where it is placed"`
}

// ThemeRule is a horizontal rule.
type ThemeRule struct {
	Y     float64 `json:"y"               jsonschema:"description=Vertical position in slide pixels"`
	X1    float64 `json:"x1"              jsonschema:"description=Left end in slide pixels"`
	X2    float64 `json:"x2"              jsonschema:"description=Right end in slide pixels"`
	Width float64 `json:"width,omitempty" jsonschema:"description=Thickness in slide pixels; 1 when omitted"`
	Token string  `json:"token,omitempty" jsonschema:"description=Colour token of the rule; fg when omitted"`
}

// ThemeText is a line of text placed on the slide.
type ThemeText struct {
	Template string `json:"template"        jsonschema:"description=The text; {title} and {author} and {n} and {total} are replaced"`
	Box      Box    `json:"box"             jsonschema:"description=Where it is placed"`
	Size     string `json:"size,omitempty"  jsonschema:"description=CSS font size"`
	Align    string `json:"align,omitempty" jsonschema:"description=Alignment in its box; left when omitted,enum=left|center|right"`
	Token    string `json:"token,omitempty" jsonschema:"description=Colour token of the text; muted when omitted"`
}

// Box is a rectangle in the slide pixels of the theme's aspect.
type Box struct {
	X float64 `json:"x" jsonschema:"description=Left edge"`
	Y float64 `json:"y" jsonschema:"description=Top edge"`
	W float64 `json:"w" jsonschema:"description=Width"`
	H float64 `json:"h" jsonschema:"description=Height"`
}

// ThemeLayout is how a theme treats one layout.
type ThemeLayout struct {
	Chrome   *bool                   `json:"chrome,omitempty"   jsonschema:"description=false hides the theme's chrome on this layout"`
	Default  *ThemeVariant           `json:"default,omitempty"  jsonschema:"description=The layout's look when a slide names no variant"`
	Variants map[string]ThemeVariant `json:"variants,omitempty" jsonschema:"description=Named looks a slide picks with variant=<name>"`
}

// ThemeVariant is one look of a layout.
type ThemeVariant struct {
	Background string            `json:"background,omitempty" jsonschema:"description=A colour or an image file behind the slide"`
	Tokens     map[string]string `json:"tokens,omitempty"     jsonschema:"description=Token overrides on slides with this look"`
	Chrome     *bool             `json:"chrome,omitempty"     jsonschema:"description=Overrides the layout's chrome setting for this look"`
	Logo       *ThemeImage       `json:"logo,omitempty"       jsonschema:"description=A logo on slides with this look; shown even where the chrome is hidden and in place of the chrome logo"`
	Slots      map[string]Box    `json:"slots,omitempty"      jsonschema:"description=Slot boxes overriding the layout's placement (slot name to box)"`
}

// ThemeIcon is one pictogram of a theme.
type ThemeIcon struct {
	Asset string `json:"asset"           jsonschema:"description=The image file; its alpha is the shape and the theme colours it"`
	Label string `json:"label,omitempty" jsonschema:"description=Human name of the pictogram"`
}

// AssetBody is the body of a deck-asset entry read from a pack: where the
// file sits under the pack's deck-assets folder, `<theme>/<path>` (the theme
// is its first segment). The file itself is the entry's source layer.
type AssetBody struct {
	Path string `json:"path" jsonschema:"description=The file's path under the deck-assets folder: the theme then the path the theme's declaration names"`
}

// Theme is the theme that ships the file: the first segment of its path.
func (a AssetBody) Theme() string {
	t, _, _ := strings.Cut(a.Path, "/")
	return t
}

// File is the file's path in its theme, as the theme's declaration names it.
func (a AssetBody) File() string {
	_, f, _ := strings.Cut(a.Path, "/")
	return f
}

// SlideGroupBody is a slide group's declaration.
type SlideGroupBody struct {
	Name        string `json:"name"        jsonschema:"description=The group's name in kebab-case - what include=<name> writes"`
	Description string `json:"description" jsonschema:"description=What the slides show and when to include them"`
	Slides      string `json:"slides"      jsonschema:"description=The slides in the deck grammar without a frontmatter; no slide of a group includes another group"`
}

// colour matches a CSS colour value as opposed to an image file.
var colour = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\(.*\)|[a-zA-Z]+)$`)

// IsColour reports whether a background is a colour rather than a file.
func IsColour(background string) bool {
	b := strings.TrimSpace(background)
	return colour.MatchString(b) && !strings.ContainsAny(b, "./")
}

// AllowsColour reports whether a deck in this theme may set colour c: any
// colour when the theme declares no palette, else one of the palette's (the
// same #rrggbb, case aside).
func (t ThemeBody) AllowsColour(c string) bool {
	if len(t.Palette) == 0 {
		return true
	}
	c = strings.ToLower(strings.TrimSpace(c))
	for _, p := range t.Palette {
		if strings.ToLower(strings.TrimSpace(p.Value)) == c {
			return true
		}
	}
	return false
}

// HasVariant reports whether the theme declares variant v for layout l.
func (t ThemeBody) HasVariant(l, v string) bool {
	_, ok := t.Layouts[l].Variants[v]
	return ok
}

// VariantNames are the variants the theme declares for layout l, sorted.
func (t ThemeBody) VariantNames(l string) []string {
	return sortedKeys(t.Layouts[l].Variants)
}

// Assets are the files the declaration names, sorted and without duplicates:
// what the theme must ship next to it.
func (t ThemeBody) Assets() []string {
	set := map[string]bool{}
	add := func(p string) {
		if p = strings.TrimSpace(p); p != "" {
			set[p] = true
		}
	}
	image := func(i *ThemeImage) {
		if i != nil {
			add(i.Asset)
		}
	}
	for _, f := range t.Fonts {
		add(f.Src)
	}
	for _, n := range t.Notices {
		add(n)
	}
	add(t.Sample)
	if t.Chrome != nil {
		image(t.Chrome.Logo)
	}
	if t.Watermark != nil {
		image(t.Watermark.Logo)
	}
	variant := func(v ThemeVariant) {
		if v.Background != "" && !IsColour(v.Background) {
			add(v.Background)
		}
		image(v.Logo)
	}
	for _, l := range t.Layouts {
		if l.Default != nil {
			variant(*l.Default)
		}
		for _, v := range l.Variants {
			variant(v)
		}
	}
	for _, i := range t.Icons {
		add(i.Asset)
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
