// SPDX-License-Identifier: Apache-2.0

// Package pptx reads a PowerPoint template (.pptx / .potx) into what a deck
// theme needs (modules/slidedeck/, the importer): its aspect, colours, fonts,
// type scale, chrome, backgrounds, layouts with their placeholders, the
// pictograms drawn as freeforms, and its media. Reading is deterministic and
// uses the standard library only (archive/zip, encoding/xml); Propose turns
// the inventory into a theme folder (propose.go).
package pptx

import (
	"archive/zip"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// EMUPerPx converts OOXML's English Metric Units to slide pixels: a 16:9
// PowerPoint page (12 192 000 EMU) is 1280 px wide, the deck's slide.
const EMUPerPx = 9525.0

// Box is a rectangle in slide pixels.
type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Inventory is what a template declares, read deterministically.
type Inventory struct {
	Width   float64   `json:"width"`
	Height  float64   `json:"height"`
	Aspect  string    `json:"aspect"`
	Theme   ThemeInfo `json:"theme"`
	Masters []Master  `json:"masters"`
	Layouts []Layout  `json:"layouts"`
	Slides  []Slide   `json:"slides"`
	Icons   []Icon    `json:"icons,omitempty"`
	Dropped []string  `json:"dropped,omitempty"`
	media   map[string][]byte
}

// ThemeInfo is the brand master's theme: its colour scheme by OOXML role
// (dk1, lt1, dk2, lt2, accent1…6, hlink) and its font scheme.
type ThemeInfo struct {
	Name    string            `json:"name"`
	Colours map[string]string `json:"colours"`
	Major   string            `json:"major"`
	Minor   string            `json:"minor"`
}

// Master is a slide master: its text styles and what it draws on every
// slide of its layouts.
type Master struct {
	Part         string        `json:"part"`
	Layouts      []string      `json:"layouts"`
	TitleSize    float64       `json:"titleSize"` // pt
	TitleBold    bool          `json:"titleBold"`
	TitleColour  string        `json:"titleColour"`
	BodySize     float64       `json:"bodySize"` // pt
	BodyColour   string        `json:"bodyColour"`
	Background   Background    `json:"background"`
	Pictures     []Picture     `json:"pictures,omitempty"`
	Rules        []Rule        `json:"rules,omitempty"`
	Placeholders []Placeholder `json:"placeholders,omitempty"`
	clrMap       map[string]string
	scheme       map[string]string
	vectors      map[string][]byte // a vector logo's SVG by its media name
}

// Layout is one slide layout of a master.
type Layout struct {
	Part             string        `json:"part"`
	Master           string        `json:"master"`
	Name             string        `json:"name"`
	Type             string        `json:"type,omitempty"`
	ShowMasterShapes bool          `json:"showMasterShapes"`
	Background       Background    `json:"background"`
	Pictures         []Picture     `json:"pictures,omitempty"`
	Rules            []Rule        `json:"rules,omitempty"`
	Fills            []Fill        `json:"fills,omitempty"`
	Placeholders     []Placeholder `json:"placeholders"`
}

// Fill is a filled shape without text — a full-slide one is a background.
type Fill struct {
	Colour string `json:"colour"`
	Box    Box    `json:"box"`
}

// Background is a colour (#rrggbb) or a picture (a media part).
type Background struct {
	Colour string `json:"colour,omitempty"`
	Media  string `json:"media,omitempty"`
}

// Picture is a picture placed on a master or a layout.
type Picture struct {
	Media string `json:"media"`
	Name  string `json:"name,omitempty"`
	Box   Box    `json:"box"`
}

// Rule is a horizontal line.
type Rule struct {
	Y      float64 `json:"y"`
	X1     float64 `json:"x1"`
	X2     float64 `json:"x2"`
	Width  float64 `json:"width"`
	Colour string  `json:"colour,omitempty"`
}

// Placeholder is a layout's text or picture region, with what its text looks
// like; a value the layout leaves out is its master's (same type).
type Placeholder struct {
	Type   string  `json:"type"`
	Idx    string  `json:"idx,omitempty"`
	Box    Box     `json:"box"`
	Prompt string  `json:"prompt,omitempty"`
	Size   float64 `json:"size,omitempty"` // pt
	Colour string  `json:"colour,omitempty"`
	Bold   bool    `json:"bold,omitempty"`
	Caps   bool    `json:"caps,omitempty"`
	Align  string  `json:"align,omitempty"` // left | center | right
}

// Slide is an example slide: its layout and the placeholders it keeps.
type Slide struct {
	Part         string   `json:"part"`
	Layout       string   `json:"layout"`
	Placeholders []string `json:"placeholders"`
	Text         string   `json:"text,omitempty"`
}

// Icon is a pictogram drawn as freeforms on a slide, as SVG.
type Icon struct {
	Slide string `json:"slide"`
	Name  string `json:"name"`
	SVG   string `json:"-"`
}

// Media returns a media part's bytes.
func (inv *Inventory) Media(part string) []byte { return inv.media[part] }

// pkg is an opened package: its parts by name.
type pkg struct {
	parts map[string][]byte
}

func (p *pkg) xml(part string) (*node, error) {
	b, ok := p.parts[part]
	if !ok {
		return nil, fmt.Errorf("part %s is missing", part)
	}
	return parseXML(b)
}

// rels maps a part's relationship ids to the parts they target.
func (p *pkg) rels(part string) map[string]string {
	dir, file := path.Split(part)
	r, err := p.xml(dir + "_rels/" + file + ".rels")
	out := map[string]string{}
	if err != nil {
		return out
	}
	for _, rel := range r.children("Relationship") {
		if rel.attr("TargetMode") == "External" {
			continue
		}
		out[rel.attr("Id")] = path.Clean(path.Join(dir, rel.attr("Target")))
	}
	return out
}

// A template's parts are read whole: these bound what one part and the whole
// package may unpack to, so a hostile file (a zip bomb) is refused instead of
// exhausting memory.
const (
	maxPart  = 64 << 20
	maxTotal = 512 << 20
)

// Read reads a template's inventory. A file that is not a well-formed
// template is an error, never a crash.
func Read(data []byte) (inv *Inventory, err error) {
	defer func() {
		if r := recover(); r != nil {
			inv, err = nil, fmt.Errorf("not a readable PowerPoint template: %v", r)
		}
	}()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a PowerPoint file: %w", err)
	}
	p := &pkg{parts: map[string][]byte{}}
	var total int64
	for _, f := range zr.File {
		if f.UncompressedSize64 > maxPart {
			return nil, fmt.Errorf("part %s unpacks to %d bytes: above %d", f.Name, f.UncompressedSize64, maxPart)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, maxPart+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(b) > maxPart {
			return nil, fmt.Errorf("part %s unpacks beyond %d bytes", f.Name, maxPart)
		}
		if total += int64(len(b)); total > maxTotal {
			return nil, fmt.Errorf("the template unpacks beyond %d bytes", maxTotal)
		}
		p.parts[f.Name] = b
	}
	pres, err := p.xml("ppt/presentation.xml")
	if err != nil {
		return nil, fmt.Errorf("not a PowerPoint file: %w", err)
	}
	inv = &Inventory{media: map[string][]byte{}}
	sz := pres.child("sldSz")
	inv.Width, inv.Height = sz.num("cx", 12192000)/EMUPerPx, sz.num("cy", 6858000)/EMUPerPx
	inv.Aspect = aspectOf(inv.Width, inv.Height)
	presRels := p.rels("ppt/presentation.xml")

	masters := map[string]*Master{}
	for _, id := range pres.path("sldMasterIdLst").children("sldMasterId") {
		part := presRels[id.rid()]
		if part == "" {
			continue
		}
		m, err := readMaster(p, part)
		if err != nil {
			return nil, err
		}
		inv.Masters = append(inv.Masters, *m)
		masters[part] = &inv.Masters[len(inv.Masters)-1]
	}
	if len(inv.Masters) == 0 {
		return nil, fmt.Errorf("no slide master")
	}
	// The brand master is the one with the most layouts; its theme is the
	// template's.
	brand := inv.Brand()
	inv.Theme = readTheme(p, brand)

	byPart := map[string]bool{}
	for i := range inv.Masters {
		m := &inv.Masters[i]
		for _, lp := range m.Layouts {
			if byPart[lp] {
				continue
			}
			byPart[lp] = true
			l, err := readLayout(p, lp, m)
			if err != nil {
				return nil, err
			}
			inv.Layouts = append(inv.Layouts, *l)
		}
	}
	sort.SliceStable(inv.Layouts, func(i, j int) bool { return partNumber(inv.Layouts[i].Part) < partNumber(inv.Layouts[j].Part) })

	for _, id := range pres.path("sldIdLst").children("sldId") {
		part := presRels[id.rid()]
		if part == "" {
			continue
		}
		s, icons, dropped := readSlide(p, part)
		inv.Slides = append(inv.Slides, s)
		inv.Icons = append(inv.Icons, icons...)
		inv.Dropped = append(inv.Dropped, dropped...)
	}
	for name, b := range p.parts {
		if strings.HasPrefix(name, "ppt/media/") {
			inv.media[name] = b
		}
	}
	for _, m := range inv.Masters {
		for name, b := range m.vectors {
			inv.media[name] = b
		}
	}
	return inv, nil
}

// Brand is the master that carries the most layouts.
func (inv *Inventory) Brand() *Master {
	best := &inv.Masters[0]
	for i := range inv.Masters {
		if len(inv.Masters[i].Layouts) > len(best.Layouts) {
			best = &inv.Masters[i]
		}
	}
	return best
}

func aspectOf(w, h float64) string {
	if h == 0 {
		return "16:9"
	}
	r := w / h
	if r < (16.0/9+4.0/3)/2 {
		return "4:3"
	}
	return "16:9"
}

func partNumber(part string) int {
	base := strings.TrimSuffix(path.Base(part), ".xml")
	i := len(base)
	for i > 0 && base[i-1] >= '0' && base[i-1] <= '9' {
		i--
	}
	n, _ := strconv.Atoi(base[i:])
	return n
}

func readMaster(p *pkg, part string) (*Master, error) {
	x, err := p.xml(part)
	if err != nil {
		return nil, err
	}
	rels := p.rels(part)
	m := &Master{Part: part, clrMap: map[string]string{}, vectors: map[string][]byte{}}
	if cm := x.child("clrMap"); cm != nil {
		for _, a := range cm.Attr {
			m.clrMap[a.Name.Local] = a.Value
		}
	}
	for _, id := range x.path("sldLayoutIdLst").children("sldLayoutId") {
		if lp := rels[id.rid()]; lp != "" {
			m.Layouts = append(m.Layouts, lp)
		}
	}
	for _, t := range rels {
		if strings.Contains(t, "/theme/") {
			m.scheme = schemeOf(p, t)
		}
	}
	tl := x.path("txStyles", "titleStyle", "lvl1pPr", "defRPr")
	m.TitleSize = tl.num("sz", 4400) / 100
	m.TitleBold = tl.attr("b") == "1"
	m.TitleColour = m.colour(tl)
	bl := x.path("txStyles", "bodyStyle", "lvl1pPr", "defRPr")
	m.BodySize = bl.num("sz", 2800) / 100
	m.BodyColour = m.colour(bl)
	tree := x.path("cSld", "spTree")
	m.Background = m.background(x, rels)
	m.Pictures, m.Rules, _, m.Placeholders = m.shapes(tree, rels, nil)
	return m, nil
}

func schemeOf(p *pkg, themePart string) map[string]string {
	t, err := p.xml(themePart)
	out := map[string]string{}
	if err != nil {
		return out
	}
	cs := t.find("clrScheme")
	if cs == nil {
		return out
	}
	for _, c := range cs.Kids {
		if v := c.child("srgbClr").attr("val"); v != "" {
			out[c.Name] = "#" + strings.ToUpper(v)
		} else if v := c.child("sysClr").attr("lastClr"); v != "" {
			out[c.Name] = "#" + strings.ToUpper(v)
		}
	}
	return out
}

func readTheme(p *pkg, m *Master) ThemeInfo {
	ti := ThemeInfo{Colours: m.scheme}
	for _, t := range p.rels(m.Part) {
		if !strings.Contains(t, "/theme/") {
			continue
		}
		x, err := p.xml(t)
		if err != nil {
			continue
		}
		ti.Name = x.attr("name")
		ti.Major = x.find("majorFont").child("latin").attr("typeface")
		ti.Minor = x.find("minorFont").child("latin").attr("typeface")
	}
	return ti
}

// colour resolves the solid fill below n: an RGB value, or a scheme colour
// through the master's colour map.
func (m *Master) colour(n *node) string {
	return m.clr(n.child("solidFill"))
}

// clr resolves the colour element below n: an RGB value, or a scheme colour
// through the master's colour map.
func (m *Master) clr(f *node) string {
	if f == nil {
		return ""
	}
	if v := f.child("srgbClr").attr("val"); v != "" {
		return "#" + strings.ToUpper(v)
	}
	if v := f.child("schemeClr").attr("val"); v != "" {
		if mapped, ok := m.clrMap[v]; ok {
			v = mapped
		}
		return m.scheme[v]
	}
	return ""
}

// background reads a part's `p:bg`: a colour or a picture.
func (m *Master) background(x *node, rels map[string]string) Background {
	bg := x.path("cSld", "bg", "bgPr")
	if bg == nil {
		return Background{}
	}
	if c := m.colour(bg); c != "" {
		return Background{Colour: c}
	}
	if b := bg.path("blipFill", "blip"); b != nil {
		return Background{Media: rels[b.attr("embed")]}
	}
	return Background{}
}

// xform maps a group's child coordinates to the slide's.
type xform struct{ ox, oy, sx, sy, cx, cy float64 }

func (t *xform) apply(x, y, w, h float64) Box {
	if t == nil {
		return Box{x / EMUPerPx, y / EMUPerPx, w / EMUPerPx, h / EMUPerPx}
	}
	return Box{(t.ox + (x-t.cx)*t.sx) / EMUPerPx, (t.oy + (y-t.cy)*t.sy) / EMUPerPx, w * t.sx / EMUPerPx, h * t.sy / EMUPerPx}
}

func boxOf(xf *node, t *xform) (Box, bool) {
	off, ext := xf.child("off"), xf.child("ext")
	if off == nil || ext == nil {
		return Box{}, false
	}
	return t.apply(off.num("x", 0), off.num("y", 0), ext.num("cx", 0), ext.num("cy", 0)), true
}

func groupXform(g *node, outer *xform) *xform {
	xf := g.path("grpSpPr", "xfrm")
	off, ext, choff, chext := xf.child("off"), xf.child("ext"), xf.child("chOff"), xf.child("chExt")
	if off == nil || ext == nil || choff == nil || chext == nil || chext.num("cx", 0) == 0 || chext.num("cy", 0) == 0 {
		return outer
	}
	t := &xform{
		ox: off.num("x", 0), oy: off.num("y", 0),
		sx: ext.num("cx", 0) / chext.num("cx", 1), sy: ext.num("cy", 0) / chext.num("cy", 1),
		cx: choff.num("x", 0), cy: choff.num("y", 0),
	}
	if outer != nil {
		b := outer.apply(t.ox, t.oy, ext.num("cx", 0), ext.num("cy", 0))
		t.ox, t.oy = b.X*EMUPerPx, b.Y*EMUPerPx
		t.sx *= outer.sx
		t.sy *= outer.sy
	}
	return t
}

// shapes reads a shape tree: pictures, horizontal rules, placeholders.
func (m *Master) shapes(tree *node, rels map[string]string, t *xform) (pics []Picture, rules []Rule, fills []Fill, phs []Placeholder) {
	if tree == nil {
		return
	}
	for _, k := range tree.Kids {
		switch k.Name {
		case "grpSp":
			// A group drawn only with freeforms is a picture (a vector logo).
			if b, svg, ok := m.vectorGroup(k, t); ok {
				name := fmt.Sprintf("ppt/vector/%08x.svg", crc32.ChecksumIEEE([]byte(svg)))
				m.vectors[name] = []byte(svg)
				pics = append(pics, Picture{Media: name, Name: k.path("nvGrpSpPr", "cNvPr").attr("name"), Box: b})
				continue
			}
			p, r, fl, ph := m.shapes(k, rels, groupXform(k, t))
			pics, rules, fills, phs = append(pics, p...), append(rules, r...), append(fills, fl...), append(phs, ph...)
		case "pic":
			b, ok := boxOf(k.path("spPr", "xfrm"), t)
			blip := k.path("blipFill", "blip")
			if ok && blip != nil && rels[blip.attr("embed")] != "" {
				pics = append(pics, Picture{Media: rels[blip.attr("embed")], Name: k.path("nvPicPr", "cNvPr").attr("name"), Box: b})
			}
		case "cxnSp", "sp":
			if ph := k.path("nvSpPr", "nvPr", "ph"); ph != nil && k.Name == "sp" {
				phs = append(phs, m.placeholder(k, ph, t))
				continue
			}
			b, ok := boxOf(k.path("spPr", "xfrm"), t)
			geom := k.path("spPr", "prstGeom").attr("prst")
			if ok && b.H < 0.5 && b.W > 20 && (k.Name == "cxnSp" || geom == "line") {
				ln := k.path("spPr", "ln")
				rules = append(rules, Rule{Y: round(b.Y), X1: round(b.X), X2: round(b.X + b.W), Width: maxf(1, round(ln.num("w", 9525)/EMUPerPx)), Colour: m.colour(ln)})
			} else if ok && k.Name == "sp" && strings.TrimSpace(k.child("txBody").texts()) == "" {
				if c := m.colour(k.child("spPr")); c != "" {
					fills = append(fills, Fill{Colour: c, Box: b})
				}
			}
		}
	}
	return
}

func (m *Master) placeholder(sp, ph *node, t *xform) Placeholder {
	typ := ph.attr("type")
	if typ == "" {
		typ = "body"
	}
	p := Placeholder{Type: typ, Idx: ph.attr("idx")}
	if b, ok := boxOf(sp.path("spPr", "xfrm"), t); ok {
		p.Box = Box{round(b.X), round(b.Y), round(b.W), round(b.H)}
	}
	body := sp.child("txBody")
	lvl := body.path("lstStyle", "lvl1pPr")
	rpr := lvl.child("defRPr")
	if rpr == nil {
		rpr = body.find("rPr")
	}
	p.Size = rpr.num("sz", 0) / 100
	p.Bold = rpr.attr("b") == "1"
	p.Caps = rpr.attr("cap") == "all"
	p.Colour = m.colour(rpr)
	// The prompt paragraph's own alignment overrides its list style's.
	algn := lvl.attr("algn")
	if a := body.child("p").child("pPr").attr("algn"); a != "" {
		algn = a
	}
	switch algn {
	case "ctr":
		p.Align = "center"
	case "r":
		p.Align = "right"
	case "l", "just":
		p.Align = "left"
	}
	p.Prompt = strings.TrimSpace(body.texts())
	return p
}

func readLayout(p *pkg, part string, m *Master) (*Layout, error) {
	x, err := p.xml(part)
	if err != nil {
		return nil, err
	}
	rels := p.rels(part)
	l := &Layout{Part: part, Master: m.Part, Name: x.child("cSld").attr("name"), Type: x.attr("type"), ShowMasterShapes: x.attr("showMasterSp") != "0"}
	l.Background = m.background(x, rels)
	l.Pictures, l.Rules, l.Fills, l.Placeholders = m.shapes(x.path("cSld", "spTree"), rels, nil)
	// What the layout leaves out is its master's placeholder of the same type.
	for i := range l.Placeholders {
		ph := &l.Placeholders[i]
		mp := m.placeholderOf(ph.Type)
		if ph.Box.W == 0 && mp != nil {
			ph.Box = mp.Box
		}
		if ph.Size == 0 {
			switch {
			case mp != nil && mp.Size > 0:
				ph.Size = mp.Size
			case isTitle(ph.Type):
				ph.Size = m.TitleSize
			default:
				ph.Size = m.BodySize
			}
		}
		if ph.Colour == "" {
			if mp != nil && mp.Colour != "" {
				ph.Colour = mp.Colour
			} else if isTitle(ph.Type) {
				ph.Colour = m.TitleColour
			} else {
				ph.Colour = m.BodyColour
			}
		}
		if !ph.Bold && isTitle(ph.Type) && m.TitleBold {
			ph.Bold = true
		}
	}
	return l, nil
}

func (m *Master) placeholderOf(typ string) *Placeholder {
	for i := range m.Placeholders {
		if m.Placeholders[i].Type == typ || (isTitle(typ) && isTitle(m.Placeholders[i].Type)) {
			return &m.Placeholders[i]
		}
	}
	return nil
}

func isTitle(t string) bool { return t == "title" || t == "ctrTitle" }

func readSlide(p *pkg, part string) (Slide, []Icon, []string) {
	s := Slide{Part: part}
	x, err := p.xml(part)
	if err != nil {
		return s, nil, nil
	}
	for _, t := range p.rels(part) {
		if strings.Contains(t, "/slideLayouts/") {
			s.Layout = t
		}
	}
	tree := x.path("cSld", "spTree")
	for _, ph := range tree.all("ph", nil) {
		t := ph.attr("type")
		if t == "" {
			t = "body"
		}
		s.Placeholders = append(s.Placeholders, t)
	}
	s.Text = strings.TrimSpace(tree.texts())
	var dropped []string
	for _, g := range tree.all("graphicFrame", nil) {
		uri := g.find("graphicData").attr("uri")
		kind := "a table"
		switch {
		case strings.Contains(uri, "chart"):
			kind = "a chart"
		case strings.Contains(uri, "diagram"):
			kind = "a SmartArt diagram"
		}
		dropped = append(dropped, fmt.Sprintf("%s: %s (not imported)", path.Base(part), kind))
	}
	return s, freeformIcons(tree, path.Base(part)), dropped
}

func round(v float64) float64 { return float64(int(v + 0.5)) }

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
