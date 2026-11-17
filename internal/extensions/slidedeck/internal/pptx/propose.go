// SPDX-License-Identifier: Apache-2.0

package pptx

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
)

// Options name the theme a template becomes.
type Options struct {
	// Name is the theme's name (kebab-case), Description its one line.
	Name        string
	Description string
	// Layouts are the deck layouts in reach: the sample deck is written from
	// their examples, and a decision names one of them.
	Layouts []deckapi.LayoutBody
	// Decisions are applied over the importer's own proposal.
	Decisions deckapi.ImportDecisions
}

// Proposal is the theme proposed from a template: its declaration, the files
// it ships (by path in its folder), how each template layout was mapped, and
// what a person reviews (guesses and what was not imported).
type Proposal struct {
	Theme   deckapi.ThemeBody
	Files   map[string][]byte
	Mapping []Mapping
	Notes   []string
}

// Mapping is what one template layout became.
type Mapping = deckapi.ImportMapping

// keywords map a template layout's own name (French or English, lower case)
// to a deck layout; the first match wins, so the specific comes first
// (« Titre impactant et texte 2 colonnes » is impact, not columns).
var keywords = []struct {
	deck  string
	words []string
}{
	{"cover", []string{"couverture", "diapositive de titre", "title slide", "cover"}},
	{"section", []string{"chapitre", "intercalaire", "en-tête de section", "section"}},
	{"agenda", []string{"sommaire", "agenda", "table des matières", "contents"}},
	{"quote", []string{"citation", "quote"}},
	{"statement", []string{"exergue", "statement", "message clé"}},
	{"key-figures", []string{"chiffre", "kpi", "figure", "number"}},
	{"timeline", []string{"time line", "timeline", "frise", "chronolog", "planning", "roadmap"}},
	{"impact", []string{"impactant", "impact"}},
	{"icons", []string{"picto", "icon"}},
	{"visuals", []string{"visuel", "visual", "galerie", "gallery"}},
	{"comparison", []string{"comparaison", "comparison", "deux contenus", "two content", "2 colonnes", "deux colonnes"}},
	{"blank", []string{"vide", "blank"}},
	{"content", []string{"titre et contenu", "title and content", "titre et texte", "title only", "titre seul", "contenu", "content"}},
}

// numbered strips PowerPoint's copy prefix: « 1_Couverture titre ».
var numbered = regexp.MustCompile(`^\d+_`)

// mapLayout decides which deck layout a template layout is: by its name, then
// by its structure; "" when neither decides.
func mapLayout(l Layout) (deck, by string) {
	name := strings.ToLower(numbered.ReplaceAllString(l.Name, ""))
	for _, k := range keywords {
		for _, w := range k.words {
			if strings.Contains(name, w) {
				return k.deck, "name"
			}
		}
	}
	switch l.Type {
	case "title":
		return "cover", "structure"
	case "secHead":
		return "section", "structure"
	case "blank":
		return "blank", "structure"
	case "obj", "tx", "titleOnly":
		return "content", "structure"
	}
	var text int
	for _, p := range l.Placeholders {
		if !chromeHolder(p.Type) {
			text++
		}
	}
	if text == 0 {
		return "blank", "structure"
	}
	return "", "none"
}

// chromeHolder: the date, footer and number placeholders are the chrome's.
func chromeHolder(t string) bool { return t == "dt" || t == "ftr" || t == "sldNum" }

// order is the sample deck's order of layouts.
var order = []string{"cover", "agenda", "section", "content", "statement", "key-figures", "icons", "visuals", "impact", "timeline", "comparison", "quote", "blank"}

// Propose turns an inventory into a theme folder.
func Propose(inv *Inventory, o Options) (*Proposal, error) {
	if !kebabName.MatchString(o.Name) {
		return nil, fmt.Errorf("theme name %q is not kebab-case ASCII", o.Name)
	}
	pr := &proposer{inv: inv, o: o, brand: inv.Brand(), p: &Proposal{Files: map[string][]byte{}}, assets: map[string]string{}}
	if err := pr.checkDecisions(); err != nil {
		return nil, err
	}
	return pr.run()
}

// checkDecisions refuses a decision the importer cannot apply: a template
// layout it does not have, a deck layout not in reach, a variant name that
// is not kebab-case.
func (pr *proposer) checkDecisions() error {
	known := map[string]bool{}
	for _, l := range pr.o.Layouts {
		known[l.Name] = true
	}
	if len(known) == 0 {
		for _, d := range order {
			known[d] = true
		}
	}
	for k, d := range pr.o.Decisions.Layouts {
		found := false
		for _, l := range pr.inv.Layouts {
			found = found || l.Name == k || path.Base(l.Part) == k
		}
		if !found {
			return fmt.Errorf("decision for layout %q: the template has no layout of that name or part", k)
		}
		if d != "drop" && !known[d] {
			return fmt.Errorf("decision for layout %q: %q is not a deck layout (known: %s, or drop)", k, d, strings.Join(sortedKeys(known), ", "))
		}
	}
	for k, v := range pr.o.Decisions.Variants {
		if l, old, ok := strings.Cut(k, "/"); !ok || l == "" || old == "" {
			return fmt.Errorf("variant decision %q: write it <layout>/<variant>", k)
		}
		if !kebabName.MatchString(v) {
			return fmt.Errorf("variant decision %q: new name %q is not kebab-case ASCII", k, v)
		}
	}
	return nil
}

// decided is the decision for a template layout: by its part first (two
// layouts may share a name), then by its name.
func (pr *proposer) decided(l Layout) (string, bool) {
	if d, ok := pr.o.Decisions.Layouts[path.Base(l.Part)]; ok {
		return d, true
	}
	d, ok := pr.o.Decisions.Layouts[l.Name]
	return d, ok
}

// renameVariants applies the variant decisions.
func (pr *proposer) renameVariants() error {
	for _, k := range sortedKeys(pr.o.Decisions.Variants) {
		name := pr.o.Decisions.Variants[k]
		layout, old, _ := strings.Cut(k, "/")
		tl, ok := pr.t.Layouts[layout]
		if _, has := tl.Variants[old]; !ok || !has {
			return fmt.Errorf("variant decision %q: the proposal has no variant %q on layout %q", k, old, layout)
		}
		if _, taken := tl.Variants[name]; taken && name != old {
			return fmt.Errorf("variant decision %q: layout %q already has a variant %q", k, layout, name)
		}
		v := tl.Variants[old]
		delete(tl.Variants, old)
		tl.Variants[name] = v
		for i := range pr.p.Mapping {
			if m := &pr.p.Mapping[i]; m.Deck == layout && m.Variant == old {
				m.Variant = name
			}
		}
	}
	return nil
}

var kebabName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type proposer struct {
	inv    *Inventory
	o      Options
	brand  *Master
	p      *Proposal
	t      deckapi.ThemeBody
	tokens map[string]string
	assets map[string]string // media part → asset path
	logos  int
	body   float64 // the body size in pt, the em of every size token
	byDeck map[string][]Layout
}

func (pr *proposer) run() (*Proposal, error) {
	inv := pr.inv
	pr.tokens = map[string]string{}
	pr.t = deckapi.ThemeBody{Name: pr.o.Name, Description: pr.o.Description, Aspect: inv.Aspect, Tokens: pr.tokens}
	if pr.t.Description == "" {
		pr.t.Description = fmt.Sprintf("Thème importé du modèle PowerPoint « %s ».", inv.Theme.Name)
	}
	pr.byDeck = map[string][]Layout{}
	if g := strings.TrimSpace(pr.o.Decisions.Guidance); g != "" {
		pr.t.Guidance = g + "\n"
	}
	rank := map[string]int{}
	for _, l := range inv.Layouts {
		deck, by := mapLayout(l)
		if d, ok := pr.decided(l); ok {
			deck, by = d, "decision"
		}
		m := Mapping{Layout: l.Name, Part: path.Base(l.Part), Deck: deck, By: by}
		switch {
		case deck == "drop":
			m.Deck, m.Note = "", "dropped by decision"
		case deck == "":
			m.Note = "no deck layout: its slides are written with content"
		default:
			pr.byDeck[deck] = append(pr.byDeck[deck], l)
		}
		pr.p.Mapping = append(pr.p.Mapping, m)
		rank[l.Part] = map[string]int{"name": 0, "structure": 1, "decision": 2}[by]
	}
	// The first layout of a deck layout is the one it keeps (a cover's
	// default look): a match by name before one by structure, before one a
	// decision added — template order otherwise.
	for _, ls := range pr.byDeck {
		sort.SliceStable(ls, func(i, j int) bool { return rank[ls[i].Part] < rank[ls[j].Part] })
	}
	pr.body = pr.brand.BodySize
	if pr.body <= 0 {
		pr.body = 18
	}
	pr.colours()
	pr.type_()
	if err := pr.chrome(); err != nil {
		return nil, err
	}
	if err := pr.layouts(); err != nil {
		return nil, err
	}
	if err := pr.renameVariants(); err != nil {
		return nil, err
	}
	pr.icons()
	pr.sample()
	pr.notes()
	pr.p.Theme = pr.t
	return pr.p, nil
}

// colours derive the colour tokens from the master: the background and text
// from its colour map, the heading from its title style, the accents from the
// colours its layouts' text uses most, the rest from its scheme by lightness.
func (pr *proposer) colours() {
	scheme := pr.inv.Theme.Colours
	role := func(r string) string {
		if m, ok := pr.brand.clrMap[r]; ok {
			r = m
		}
		return scheme[r]
	}
	bg := or(role("bg1"), "#FFFFFF")
	fg := or(pr.brand.BodyColour, role("tx1"), "#000000")
	heading := or(pr.brand.TitleColour, fg)
	pr.tokens["bg"], pr.tokens["fg"], pr.tokens["heading"] = bg, fg, heading

	count := map[string]int{}
	for _, l := range pr.inv.Layouts {
		for _, p := range l.Placeholders {
			if c := p.Colour; c != "" && !chromeHolder(p.Type) && p.Type != "pic" && c != fg && c != bg && c != heading && luminance(c) < 0.9 && luminance(c) > 0.02 {
				count[c]++
			}
		}
	}
	used := ranked(count)
	accent := first(used, 0, or(scheme["accent1"], fg))
	accent2 := first(used, 1, accent)
	pr.tokens["accent"], pr.tokens["accent-2"] = accent, accent2

	taken := map[string]bool{bg: true, fg: true, accent: true, accent2: true}
	var dark, light []string
	for _, r := range []string{"accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "dk2", "lt2"} {
		c := scheme[r]
		if c == "" || taken[c] {
			continue
		}
		taken[c] = true
		if luminance(c) < 0.5 {
			dark = append(dark, c)
		} else {
			light = append(light, c)
		}
	}
	sort.SliceStable(dark, func(i, j int) bool { return luminance(dark[i]) < luminance(dark[j]) })
	sort.SliceStable(light, func(i, j int) bool { return luminance(light[i]) > luminance(light[j]) })
	pr.tokens["muted"] = first(dark, 0, fg)
	surface := first(light, 0, bg)
	pr.tokens["surface"], pr.tokens["accent-soft"] = surface, surface
	pr.tokens["border"] = first(light, 1, surface)
	pr.tokens["icon"] = accent2
}

// type_ derives the fonts and the type scale: the body size in px, every
// other size in em of it.
func (pr *proposer) type_() {
	th := pr.inv.Theme
	stack := func(f string) string {
		if f == "" {
			return "Arial, sans-serif"
		}
		return quoteFont(f) + ", Arial, sans-serif"
	}
	pr.tokens["font"] = stack(th.Minor)
	pr.tokens["heading-font"] = stack(th.Major)
	if th.Major != "" {
		pr.tokens["office-heading-font"] = th.Major
	}
	if th.Minor != "" {
		pr.tokens["office-font"] = th.Minor
	}
	if !pr.brand.TitleBold {
		pr.tokens["heading-weight"] = "400"
	}
	pr.tokens["size"] = px(pr.body)
	pr.tokens["h2"] = pr.em(pr.brand.TitleSize)
	if t := pr.brand.placeholderOf("title"); t != nil {
		pr.tokens["pad-x"] = px0(t.Box.X)
		pr.tokens["pad-y"] = px0(t.Box.Y)
	}
	size := func(deck, token string, pick func(Layout) *Placeholder) {
		if ls := pr.byDeck[deck]; len(ls) > 0 {
			if p := pick(ls[0]); p != nil && p.Size > 0 {
				pr.tokens[token] = pr.em(p.Size)
			}
		}
	}
	size("cover", "h1", titleOf)
	size("cover", "cover-size", titleOf)
	size("section", "number-size", numberOf)
	size("agenda", "agenda-title-size", titleOf)
	size("statement", "statement-size", titleOf)
	size("impact", "impact-size", titleOf)
	size("key-figures", "figure-size", largestBody)
	size("timeline", "step-size", largestBody)
	// What the deck layouts draw and a template's do not: the cover's rule
	// under the title, the chapter's bar and tinted background.
	if c := pr.byDeck["cover"]; len(c) > 0 && len(c[0].Rules) == 0 {
		pr.tokens["cover-rule-width"] = "0px"
	}
	if s := pr.byDeck["section"]; len(s) > 0 {
		pr.tokens["section-bg"] = or(pr.solidBackground(s[0]), pr.tokens["bg"])
		if len(s[0].Rules) == 0 && len(s[0].Fills) == 0 {
			pr.tokens["section-bar"] = "0px"
			pr.tokens["section-indent"] = "0px"
		}
	}
	if c := pr.byDeck["cover"]; len(c) > 0 {
		t, s := titleOf(c[0]), placeholderTyped(c[0], "subTitle")
		if t != nil && t.Align != "" {
			pr.tokens["cover-align"] = t.Align
		}
		if t != nil && s != nil && s.Box.Y+s.Box.H/2 < t.Box.Y+t.Box.H/2 {
			pr.tokens["cover-direction"] = "column-reverse"
		}
		if (t != nil && (t.Caps || upper(t.Prompt))) || anyUpperTitle(c) {
			pr.tokens["cover-transform"] = "uppercase"
		}
	}
}

// chrome is what the brand master draws on every slide: its rules, its logo
// (a picture away from the slide's edges), its footer and slide number when
// the master's example slides keep them.
func (pr *proposer) chrome() error {
	m := pr.brand
	c := &deckapi.ThemeChrome{}
	for _, r := range m.Rules {
		c.Rules = append(c.Rules, deckapi.ThemeRule{Y: r.Y, X1: r.X1, X2: r.X2, Width: r.Width, Token: pr.tokenFor(r.Colour, "rule")})
	}
	for _, p := range m.Pictures {
		if pr.fullSlide(p.Box) {
			continue
		}
		a, err := pr.logo(p.Media)
		if err != nil {
			return err
		}
		c.Logo = &deckapi.ThemeImage{Asset: a, Box: rbox(p.Box)}
		break
	}
	kept := pr.keptOnSlides()
	if f := m.placeholderOf("ftr"); f != nil && f.Size > 1 && kept["ftr"] {
		c.Footer = &deckapi.ThemeText{Template: "{title}", Box: deckBox(f.Box), Size: px0(f.Size * 4 / 3), Align: alignOf(f, pr.inv.Width), Token: pr.tokenFor(f.Colour, "chrome")}
		pr.tokens["chrome-size"] = px0(f.Size * 4 / 3)
	}
	if n := m.placeholderOf("sldNum"); n != nil && n.Size > 1 && kept["sldNum"] {
		c.SlideNumber = &deckapi.ThemeText{Template: "{n}", Box: deckBox(n.Box), Size: px0(n.Size * 4 / 3), Align: alignOf(n, pr.inv.Width), Token: pr.tokenFor(n.Colour, "chrome")}
	}
	if c.Logo != nil || len(c.Rules) > 0 || c.Footer != nil || c.SlideNumber != nil {
		pr.t.Chrome = c
	}
	return nil
}

// keptOnSlides are the chrome placeholders most example slides of the brand
// master keep: what a deck in this template shows.
func (pr *proposer) keptOnSlides() map[string]bool {
	own := map[string]bool{}
	for _, l := range pr.brand.Layouts {
		own[l] = true
	}
	var n int
	count := map[string]int{}
	for _, s := range pr.inv.Slides {
		if !own[s.Layout] {
			continue
		}
		n++
		seen := map[string]bool{}
		for _, p := range s.Placeholders {
			if !seen[p] {
				seen[p] = true
				count[p]++
			}
		}
	}
	out := map[string]bool{}
	for p, c := range count {
		out[p] = n > 0 && c*2 > n
	}
	if n == 0 { // no example slide: the master's own placeholders decide
		out["ftr"], out["sldNum"] = true, true
	}
	return out
}

// layouts are the per-layout looks: the cover and its variants (one per
// cover layout, named by its background colour), the chrome a layout hides,
// a layout's own background and text colours, a photo variant where a
// picture fills the slide.
func (pr *proposer) layouts() error {
	out := map[string]deckapi.ThemeLayout{}
	if covers := pr.byDeck["cover"]; len(covers) > 0 {
		tl, err := pr.covers(covers)
		if err != nil {
			return err
		}
		out["cover"] = tl
	}
	for _, deck := range order {
		ls := pr.byDeck[deck]
		if deck == "cover" || len(ls) == 0 {
			continue
		}
		l := ls[0]
		var tl deckapi.ThemeLayout
		if pr.hidesChrome(l) {
			tl.Chrome = boolp(false)
		}
		photo := pr.photoLayout(l)
		v := deckapi.ThemeVariant{Tokens: map[string]string{}}
		if bg := pr.solidBackground(l); bg != "" {
			v.Background = bg
			v.Tokens["bg"] = bg
		}
		if h := headingOf(l); h != nil && h.Colour != "" && h.Colour != pr.tokens["heading"] {
			v.Tokens["heading"] = h.Colour
		}
		if c := bodyColour(l, pr.body); c != "" && c != pr.tokens["fg"] && !photo {
			v.Tokens["fg"] = c
		}
		if photo {
			delete(v.Tokens, "heading") // the photo's text colour, not the layout's
		}
		if v.Background != "" || len(v.Tokens) > 0 {
			if len(v.Tokens) == 0 {
				v.Tokens = nil
			}
			tl.Default = &v
		}
		if photo {
			pv := deckapi.ThemeVariant{Chrome: boolp(false)}
			if c := or(bodyColour(l, pr.body), colourOf(headingOf(l))); c != "" {
				pv.Tokens = map[string]string{"fg": c, "heading": c, "muted": c}
			}
			tl.Variants = map[string]deckapi.ThemeVariant{"photo": pv}
			pr.mapped(l, deck, "photo", "a picture fills the slide: the photo variant (the slide's background= is the photo)")
		}
		if tl.Chrome != nil || tl.Default != nil || tl.Variants != nil {
			out[deck] = tl
		}
		for _, other := range ls[1:] {
			pr.mapped(other, deck, "", fmt.Sprintf("a second %s layout: not kept (the deck layout arranges its own items)", deck))
		}
	}
	if len(out) > 0 {
		pr.t.Layouts = out
	}
	return nil
}

// covers: the first cover layout is the default look, every other one a
// variant named after its background colour (and -logo when it carries a
// logo of its own); covers identical in background and logo are one.
func (pr *proposer) covers(covers []Layout) (deckapi.ThemeLayout, error) {
	tl := deckapi.ThemeLayout{}
	if pr.hidesChrome(covers[0]) {
		tl.Chrome = boolp(false)
	}
	type look struct{ bg, logo string }
	seen := map[look]string{}
	var defaultLogo string
	for i, l := range covers {
		v := deckapi.ThemeVariant{Tokens: map[string]string{}}
		var bgColour, bgMedia, logoMedia string
		var logoBox Box
		for _, p := range l.Pictures {
			if pr.fullSlide(p.Box) {
				bgMedia = p.Media
			} else if logoMedia == "" {
				logoMedia, logoBox = p.Media, p.Box
			}
		}
		if bgMedia == "" && l.Background.Media != "" {
			bgMedia = l.Background.Media
		}
		name := ""
		switch {
		case bgMedia != "":
			img, err := decode(pr.inv.Media(bgMedia))
			if err != nil {
				pr.p.Notes = append(pr.p.Notes, fmt.Sprintf("%s: background %s is not a PNG or JPEG: not imported", l.Name, path.Base(bgMedia)))
				break
			}
			bgColour = average(img)
			name = colourName(bgColour)
			a := "bg/" + name + ".jpg"
			if prev, ok := pr.assets[bgMedia]; ok {
				a = prev
			} else {
				a = pr.unique(a)
				b, err := backgroundJPEG(img, int(pr.inv.Width))
				if err != nil {
					return tl, err
				}
				pr.p.Files[a] = b
				pr.assets[bgMedia] = a
			}
			v.Background = a
		default:
			bgColour = pr.solidBackground(l)
			if bgColour != "" {
				v.Background = bgColour
				name = colourName(bgColour)
			}
		}
		if logoMedia != "" {
			a, err := pr.logo(logoMedia)
			if err != nil {
				return tl, err
			}
			v.Logo = &deckapi.ThemeImage{Asset: a, Box: rbox(logoBox)}
		}
		if t := titleOf(l); t != nil && t.Colour != "" && t.Colour != pr.tokens["heading"] {
			v.Tokens["heading"] = t.Colour
		}
		if s := placeholderTyped(l, "subTitle"); s != nil && s.Colour != "" {
			if s.Colour != pr.tokens["fg"] {
				v.Tokens["fg"] = s.Colour
			}
			if s.Colour != pr.tokens["muted"] {
				v.Tokens["muted"] = s.Colour
			}
		}
		if len(v.Tokens) == 0 {
			v.Tokens = nil
		}
		k := look{v.Background, logoMedia}
		if prev, dup := seen[k]; dup {
			pr.mapped(l, "cover", prev, "the same look as "+or(prev, "the default cover"))
			continue
		}
		if i == 0 {
			if t, s := titleOf(l), placeholderTyped(l, "subTitle"); t != nil {
				b := t.Box
				if s != nil {
					b = union(b, s.Box)
				}
				v.Slots = map[string]deckapi.Box{"default": deckBox(b)}
			}
			defaultLogo = logoMedia
			tl.Default = &v
			seen[k] = ""
			continue
		}
		if name == "" {
			name = "plain"
		}
		if logoMedia != "" && logoMedia != defaultLogo {
			name += "-logo"
		}
		name = uniqueKey(tl.Variants, name)
		if tl.Variants == nil {
			tl.Variants = map[string]deckapi.ThemeVariant{}
		}
		tl.Variants[name] = v
		seen[k] = name
		pr.mapped(l, "cover", name, fmt.Sprintf("variant named after its background (%s, a guess)", or(bgColour, "none")))
	}
	return tl, nil
}

// icons are the example slides' freeform pictograms, as SVG files.
func (pr *proposer) icons() {
	if len(pr.inv.Icons) == 0 {
		return
	}
	pr.t.Icons = map[string]deckapi.ThemeIcon{}
	for _, ic := range pr.inv.Icons {
		name := "icon-" + strings.ToLower(ic.Name)
		a := "icons/" + name + ".svg"
		pr.p.Files[a] = []byte(ic.SVG)
		pr.t.Icons[name] = deckapi.ThemeIcon{Asset: a, Label: "pictogramme de " + strings.TrimSuffix(ic.Slide, ".xml")}
	}
}

// sample writes the theme preview: one slide per deck layout the template
// maps, then one per cover variant, from the layouts' own examples.
func (pr *proposer) sample() {
	examples := map[string]string{}
	for _, l := range pr.o.Layouts {
		examples[l.Name] = strings.TrimSpace(l.Example)
	}
	icons := sortedNames(pr.t.Icons)
	var slides []string
	add := func(deck, variant string) {
		ex := examples[deck]
		if ex == "" {
			return
		}
		lines := strings.Split(ex, "\n")
		if strings.HasPrefix(lines[0], "<!-- slide:") {
			head := "<!-- slide: layout=" + deck
			if variant != "" {
				head += " variant=" + variant
			}
			if variant == "photo" { // a dark colour stands in for the photo
				head += fmt.Sprintf(" background=%q", pr.tokens["muted"])
			}
			lines[0] = head + " -->"
		}
		slides = append(slides, withIcons(strings.Join(lines, "\n"), icons))
	}
	for _, deck := range order {
		if len(pr.byDeck[deck]) > 0 {
			add(deck, "")
		}
	}
	if c, ok := pr.t.Layouts["cover"]; ok {
		for _, v := range sortedKeys(c.Variants) {
			add("cover", v)
		}
	}
	for _, v := range sortedKeys(pr.t.Layouts) {
		if v == "cover" {
			continue
		}
		for _, name := range sortedKeys(pr.t.Layouts[v].Variants) {
			add(v, name)
		}
	}
	if len(slides) == 0 {
		return
	}
	pr.t.Sample = "sample.deck.md"
	src := fmt.Sprintf("---\ntitle: %s\ntheme: %s\n---\n\n%s\n", yamlString("Aperçu du thème "+pr.o.Name), pr.o.Name, strings.Join(slides, "\n\n---\n\n"))
	pr.p.Files[pr.t.Sample] = []byte(src)
}

// notes are what a person reviews: what the importer could not take.
func (pr *proposer) notes() {
	th := pr.inv.Theme
	for _, f := range uniq([]string{th.Major, th.Minor}) {
		pr.p.Notes = append(pr.p.Notes, fmt.Sprintf("font %s is named, not shipped: a template does not carry its fonts; ship a web font with its licence (fonts:, notices:) or keep the fallback", f))
	}
	if d := pr.brand.placeholderOf("dt"); d != nil {
		pr.p.Notes = append(pr.p.Notes, "the date placeholder is not imported (a deck has no date in its chrome)")
	}
	if pr.t.Chrome == nil || pr.t.Chrome.Logo == nil {
		pr.p.Notes = append(pr.p.Notes, "no logo found on the master: add chrome.logo by hand")
	}
	pr.p.Notes = append(pr.p.Notes, pr.inv.Dropped...)
	pr.p.Notes = append(pr.p.Notes, "a brand charter (palette, colour rules, screen fonts) is not in a template: add palette: and guidance: from the charter")
}

// mapped records a layout's variant or note on its mapping.
func (pr *proposer) mapped(l Layout, deck, variant, note string) {
	for i := range pr.p.Mapping {
		if pr.p.Mapping[i].Part == path.Base(l.Part) {
			pr.p.Mapping[i].Deck, pr.p.Mapping[i].Variant, pr.p.Mapping[i].Note = deck, variant, note
		}
	}
}

// logo ships a logo picture once, the brand master's first as logo.<ext>.
func (pr *proposer) logo(media string) (string, error) {
	if a, ok := pr.assets[media]; ok {
		return a, nil
	}
	pr.logos++
	ext := strings.ToLower(path.Ext(media))
	a := "logo" + ext
	if pr.logos > 1 {
		a = fmt.Sprintf("logo-%d%s", pr.logos, ext)
	}
	b, err := fitAsset(a, pr.inv.Media(media))
	if err != nil {
		return "", err
	}
	pr.p.Files[a] = b
	pr.assets[media] = a
	return a, nil
}

// unique is a file path not yet shipped.
func (pr *proposer) unique(a string) string {
	if _, ok := pr.p.Files[a]; !ok {
		return a
	}
	ext := path.Ext(a)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d%s", strings.TrimSuffix(a, ext), i, ext)
		if _, ok := pr.p.Files[c]; !ok {
			return c
		}
	}
}

// hidesChrome: a layout that hides the master's shapes and does not draw the
// master's logo again hides the chrome.
func (pr *proposer) hidesChrome(l Layout) bool {
	if l.ShowMasterShapes {
		return false
	}
	for _, mp := range pr.brand.Pictures {
		for _, p := range l.Pictures {
			if p.Media == mp.Media {
				return false
			}
		}
	}
	return true
}

// solidBackground is a layout's own background colour: its p:bg, or a filled
// shape covering the slide.
func (pr *proposer) solidBackground(l Layout) string {
	if l.Background.Colour != "" {
		return l.Background.Colour
	}
	for _, f := range l.Fills {
		if pr.fullSlide(f.Box) {
			return f.Colour
		}
	}
	return ""
}

// photoLayout: a picture placeholder fills the slide.
func (pr *proposer) photoLayout(l Layout) bool {
	for _, p := range l.Placeholders {
		if p.Type == "pic" && pr.fullSlide(p.Box) {
			return true
		}
	}
	return false
}

func colourOf(p *Placeholder) string {
	if p == nil {
		return ""
	}
	return p.Colour
}

func (pr *proposer) fullSlide(b Box) bool {
	return b.W >= pr.inv.Width*0.95 && b.H >= pr.inv.Height*0.95
}

// tokenFor names the colour token holding c, adding <fallback> when none does.
func (pr *proposer) tokenFor(c, fallback string) string {
	if c == "" {
		return "fg"
	}
	for _, k := range []string{"fg", "heading", "muted", "accent", "accent-2", "border"} {
		if strings.EqualFold(pr.tokens[k], c) {
			return k
		}
	}
	pr.tokens[fallback] = c
	return fallback
}

func (pr *proposer) em(pt float64) string {
	return strconv.FormatFloat(round2(pt/pr.body), 'f', -1, 64) + "em"
}

func titleOf(l Layout) *Placeholder {
	for i := range l.Placeholders {
		if isTitle(l.Placeholders[i].Type) {
			return &l.Placeholders[i]
		}
	}
	return nil
}

func placeholderTyped(l Layout, t string) *Placeholder {
	for i := range l.Placeholders {
		if l.Placeholders[i].Type == t {
			return &l.Placeholders[i]
		}
	}
	return nil
}

var digits = regexp.MustCompile(`^\d+$`)

// numberOf is a layout's number placeholder: one whose prompt is digits.
func numberOf(l Layout) *Placeholder {
	for i := range l.Placeholders {
		if digits.MatchString(strings.TrimSpace(l.Placeholders[i].Prompt)) {
			return &l.Placeholders[i]
		}
	}
	return nil
}

// headingOf is the placeholder the slide's heading takes: the title, or the
// largest text when the title holds a number (a chapter's « 00 »).
func headingOf(l Layout) *Placeholder {
	t := titleOf(l)
	if t != nil && !digits.MatchString(strings.TrimSpace(t.Prompt)) {
		return t
	}
	return largestBody(l)
}

func largestBody(l Layout) *Placeholder {
	var best *Placeholder
	for i := range l.Placeholders {
		p := &l.Placeholders[i]
		if p.Type != "body" || p.Size <= 1 || strings.TrimSpace(p.Prompt) == "" {
			continue
		}
		if best == nil || p.Size > best.Size {
			best = p
		}
	}
	return best
}

// bodyColour is the colour most of a layout's running text uses: display
// text (a figure, a chapter title, at least 1.5 times the body size) is not
// running text.
func bodyColour(l Layout, body float64) string {
	count := map[string]int{}
	for _, p := range l.Placeholders {
		if p.Type == "body" && p.Size > 1 && p.Size < body*1.5 && p.Colour != "" && strings.TrimSpace(p.Prompt) != "" {
			count[p.Colour]++
		}
	}
	return first(ranked(count), 0, "")
}

func anyUpperTitle(ls []Layout) bool {
	for _, l := range ls {
		if t := titleOf(l); t != nil && upper(t.Prompt) {
			return true
		}
	}
	return false
}

// upper: a prompt written in capitals.
func upper(s string) bool {
	return strings.ToUpper(s) == s && strings.ToLower(s) != s
}

func alignOf(p *Placeholder, width float64) string {
	if p.Align != "" {
		return p.Align
	}
	if p.Box.X+p.Box.W/2 > width*2/3 {
		return "right"
	}
	return "left"
}

// withIcons points an example's pictograms at the theme's icons, or drops the
// pictures when the theme has none.
var exampleImage = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*)\)\s*`)

func withIcons(s string, icons []string) string {
	i := 0
	return exampleImage.ReplaceAllStringFunc(s, func(m string) string {
		if len(icons) == 0 {
			return ""
		}
		label := exampleImage.FindStringSubmatch(m)[1]
		name := icons[i%len(icons)]
		i++
		return fmt.Sprintf("![%s](icon:%s) ", label, name)
	})
}

func quoteFont(f string) string {
	if strings.ContainsAny(f, " ") {
		return "'" + f + "'"
	}
	return f
}

func yamlString(s string) string { return strconv.Quote(s) }

func px(pt float64) string { return strconv.Itoa(int(pt*4/3+0.5)) + "px" }

func px0(v float64) string { return strconv.Itoa(int(v+0.5)) + "px" }

func round2(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

func rbox(b Box) deckapi.Box {
	return deckapi.Box{X: round(b.X), Y: round(b.Y), W: round(b.W), H: round(b.H)}
}

func deckBox(b Box) deckapi.Box { return rbox(b) }

func union(a, b Box) Box {
	x1, y1 := min(a.X, b.X), min(a.Y, b.Y)
	x2, y2 := max(a.X+a.W, b.X+b.W), max(a.Y+a.H, b.Y+b.H)
	return Box{x1, y1, x2 - x1, y2 - y1}
}

func boolp(b bool) *bool { return &b }

func or(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

func first(vs []string, i int, def string) string {
	if i < len(vs) {
		return vs[i]
	}
	return def
}

// ranked are the keys by count, then by name.
func ranked(count map[string]int) []string {
	out := make([]string, 0, len(count))
	for k := range count {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if count[out[i]] != count[out[j]] {
			return count[out[i]] > count[out[j]]
		}
		return out[i] < out[j]
	})
	return out
}

func uniq(vs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range vs {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func uniqueKey[V any](m map[string]V, k string) string {
	if _, ok := m[k]; !ok {
		return k
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d", k, i)
		if _, ok := m[c]; !ok {
			return c
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedNames(m map[string]deckapi.ThemeIcon) []string { return sortedKeys(m) }
