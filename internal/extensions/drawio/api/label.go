// SPDX-License-Identifier: Apache-2.0

package api

import (
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Label geometry is estimated, not measured: draw.io lays text out in the
// browser. The estimate is the one the layout reserves and the quality
// report measures, so both speak of the same boxes.
const (
	labelLineHeight = 12.0 // an edge label's line (font 9 to 10)
	labelCharWidth  = 5.6
	labelPadding    = 6.0
)

var (
	labelBreak = regexp.MustCompile(`(?i)<br\s*/?>|</div>|</p>|<hr\s*/?>`)
	labelTag   = regexp.MustCompile(`<[^>]*>`)
)

// LabelSize estimates the box a draw.io HTML label takes: one line per
// break, as wide as its longest line. An empty label takes no room.
func LabelSize(label string) (w, h float64) {
	text := labelBreak.ReplaceAllString(label, "\n")
	text = html.UnescapeString(labelTag.ReplaceAllString(text, ""))
	lines, longest := 0, 0
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		lines++
		longest = max(longest, utf8.RuneCountInString(l))
	}
	if lines == 0 {
		return 0, 0
	}
	return float64(longest)*labelCharWidth + labelPadding, float64(lines)*labelLineHeight + labelPadding
}

// PlainLabel is a label's text on one line: tags removed (a line break
// becomes a space), entities decoded, spaces collapsed.
func PlainLabel(label string) string {
	text := labelBreak.ReplaceAllString(label, " ")
	text = html.UnescapeString(labelTag.ReplaceAllString(text, ""))
	return strings.Join(strings.Fields(text), " ")
}

// PolylineMidpoint is the point halfway along a polyline — where draw.io
// puts an edge's label before its offset.
func PolylineMidpoint(pts []Point) Point {
	if len(pts) == 0 {
		return Point{}
	}
	var total float64
	for i := 0; i+1 < len(pts); i++ {
		total += math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
	}
	half := total / 2
	for i := 0; i+1 < len(pts); i++ {
		seg := math.Hypot(pts[i+1].X-pts[i].X, pts[i+1].Y-pts[i].Y)
		if seg > 0 && half <= seg {
			t := half / seg
			return Point{X: pts[i].X + t*(pts[i+1].X-pts[i].X), Y: pts[i].Y + t*(pts[i+1].Y-pts[i].Y)}
		}
		half -= seg
	}
	return pts[len(pts)-1]
}

// ParseStyle splits a draw.io style into its keys: "a=1;text;b=2" gives
// {a:1, text:"", b:2}. A key without a value (the base style name) maps to
// the empty string.
func ParseStyle(style string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(style, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		m[k] = v
	}
	return m
}

// Text geometry. draw.io lays a label out in the browser; TextBox
// reproduces that layout from the style and the glyph widths of the
// Helvetica metric (textmetrics_gen.go), so the boxes it gives are the ones
// draw.io paints, give or take the side bearings.
const (
	// textLineHeight is draw.io's line height, in font sizes.
	textLineHeight = 1.2
	// textDefaultSize and textDefaultSpacing are draw.io's defaults.
	textDefaultSize    = 12.0
	textDefaultSpacing = 2.0
	// textTopShift is how much lower than its spacing says draw.io paints
	// a top-aligned label (measured: render/testdata/textmetrics.json).
	textTopShift = 5.0
	// OverflowSlack is the pessimism a text-overflow check applies to the
	// widths: a label is reported out of its shape only when it would be
	// even 8 % narrower than estimated.
	OverflowSlack = 1.08
)

// textRun is a stretch of label text in one font size and weight.
type textRun struct {
	text string
	size float64
	bold bool
}

// textFont is the font state an HTML tag pushes.
type textFont struct {
	size float64
	bold bool
}

var (
	textTag      = regexp.MustCompile(`<[^>]*>`)
	tagName      = regexp.MustCompile(`^</?\s*([a-zA-Z0-9]+)`)
	styleAttr    = regexp.MustCompile(`(?i)style\s*=\s*["']([^"']*)["']`)
	sizeAttr     = regexp.MustCompile(`(?i)\ssize\s*=\s*["']?(\d)`)
	cssFontSize  = regexp.MustCompile(`(?i)font-size\s*:\s*([0-9.]+)\s*(px|pt|em)?`)
	cssWeight    = regexp.MustCompile(`(?i)font-weight\s*:\s*([a-z0-9]+)`)
	spaceCollect = regexp.MustCompile(`[ \t\r\f\v]+`)
)

// fontSizeAttr maps <font size="N"> to pixels, as browsers do.
var fontSizeAttr = map[byte]float64{'1': 10, '2': 13, '3': 16, '4': 18, '5': 24, '6': 32, '7': 48}

var blockTags = map[string]bool{"div": true, "p": true, "li": true, "ul": true, "ol": true, "tr": true, "table": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "blockquote": true, "pre": true}

var voidTags = map[string]bool{"br": true, "hr": true, "img": true, "input": true, "meta": true, "wbr": true}

// labelLines splits a label into its hard lines, each a list of runs: an
// HTML label breaks on <br>, block boundaries and line feeds, and its tags
// set a run's size and weight; a plain label breaks on line feeds only.
func labelLines(label string, isHTML bool, base textFont) [][]textRun {
	if !isHTML {
		var lines [][]textRun
		for _, l := range strings.Split(label, "\n") {
			lines = append(lines, []textRun{{text: l, size: base.size, bold: base.bold}})
		}
		return lines
	}
	var lines [][]textRun
	var cur []textRun
	stack := []textFont{base}
	hasText := func() bool {
		for _, r := range cur {
			if strings.TrimSpace(r.text) != "" {
				return true
			}
		}
		return false
	}
	breakLine := func() {
		if len(cur) == 0 {
			// An empty line keeps the height of the font it is in.
			f := stack[len(stack)-1]
			cur = []textRun{{size: f.size, bold: f.bold}}
		}
		lines = append(lines, cur)
		cur = nil
	}
	addText := func(s string) {
		s = html.UnescapeString(spaceCollect.ReplaceAllString(s, " "))
		if s == "" {
			return
		}
		f := stack[len(stack)-1]
		cur = append(cur, textRun{text: s, size: f.size, bold: f.bold})
	}
	label = strings.ReplaceAll(label, "\n", "<br>")
	pos := 0
	for _, loc := range textTag.FindAllStringIndex(label, -1) {
		addText(label[pos:loc[0]])
		pos = loc[1]
		tag := label[loc[0]:loc[1]]
		m := tagName.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		name := strings.ToLower(m[1])
		closing := strings.HasPrefix(tag, "</")
		switch {
		case name == "br":
			breakLine()
		case name == "hr":
			if hasText() {
				breakLine()
			}
		case closing:
			if blockTags[name] && hasText() {
				breakLine()
			}
			if len(stack) > 1 && !voidTags[name] {
				stack = stack[:len(stack)-1]
			}
		case voidTags[name] || strings.HasSuffix(tag, "/>"):
		default:
			if blockTags[name] && hasText() {
				breakLine()
			}
			stack = append(stack, tagFont(name, tag, stack[len(stack)-1]))
		}
	}
	addText(label[pos:])
	if hasText() {
		lines = append(lines, cur)
	}
	return lines
}

// tagFont is the font an opening tag sets for its content.
func tagFont(name, tag string, f textFont) textFont {
	switch name {
	case "b", "strong", "th", "h1", "h2", "h3", "h4", "h5", "h6":
		f.bold = true
	}
	if sm := sizeAttr.FindStringSubmatch(tag); name == "font" && sm != nil {
		if px, ok := fontSizeAttr[sm[1][0]]; ok {
			f.size = px
		}
	}
	sm := styleAttr.FindStringSubmatch(tag)
	if sm == nil {
		return f
	}
	css := html.UnescapeString(sm[1])
	if fm := cssFontSize.FindStringSubmatch(css); fm != nil {
		if v, err := strconv.ParseFloat(fm[1], 64); err == nil {
			switch strings.ToLower(fm[2]) {
			case "pt":
				v = v * 4 / 3
			case "em":
				v *= f.size
			}
			f.size = v
		}
	}
	if wm := cssWeight.FindStringSubmatch(css); wm != nil {
		switch strings.ToLower(wm[1]) {
		case "bold", "bolder", "600", "700", "800", "900":
			f.bold = true
		case "normal", "lighter", "100", "200", "300", "400", "500":
			f.bold = false
		}
	}
	return f
}

// textWord is a word of a line: its width, the width of the space before
// it and its largest font size. A word may span runs ("41%" in two sizes
// is one word).
type textWord struct {
	width, space, size float64
}

// splitWords cuts a hard line into words; it also returns the line's
// largest font size, spaces included.
func splitWords(line []textRun, metric bool) ([]textWord, float64) {
	var words []textWord
	var w textWord
	open := false
	pending, lineSize := 0.0, 0.0
	for _, r := range line {
		lineSize = max(lineSize, r.size)
		for _, ch := range r.text {
			if ch == ' ' {
				if open {
					words = append(words, w)
					w, open = textWord{}, false
				}
				pending = textWidth(" ", r.size, r.bold, metric)
				continue
			}
			if !open {
				w, open = textWord{space: pending}, true
			}
			w.width += glyphAdvance(ch, r.bold, metric) * r.size / 1000
			w.size = max(w.size, r.size)
		}
	}
	if open {
		words = append(words, w)
	}
	if len(words) > 0 {
		words[0].space = 0
	}
	return words, lineSize
}

// TextBox estimates where draw.io paints a vertex's label: the box of its
// lines, laid out in cell (page coordinates) the way its style says —
// fontSize, bold, HTML runs (<b>, font-size, <br>, blocks), whiteSpace=wrap
// (greedy, on spaces, no hyphenation: a word wider than the cell
// overflows), spacing, align and verticalAlign, label position, swimlane
// header, horizontal=0 and rotation, overflow=hidden. slack widens every
// line (1 = the nominal width, OverflowSlack for an overflow check). A
// label not drawn (empty, noLabel, fontColor=none, textOpacity=0) has no
// box and no lines.
func TextBox(label, style string, cell Box, slack float64) (Box, int) {
	st := ParseStyle(style)
	if st["noLabel"] == "1" || st["fontColor"] == "none" || st["textOpacity"] == "0" {
		return Box{}, 0
	}
	num := func(k string, def float64) float64 {
		if v, err := strconv.ParseFloat(st[k], 64); err == nil {
			return v
		}
		return def
	}
	size := num("fontSize", textDefaultSize)
	fontStyle, _ := strconv.Atoi(st["fontStyle"])
	metric := metricFamily(st["fontFamily"])
	hard := labelLines(label, st["html"] == "1", textFont{size: size, bold: fontStyle&1 != 0})

	// The frame the label is laid out in.
	frame := cell
	_, swimlane := st["swimlane"]
	horizontal := st["horizontal"] != "0"
	if swimlane {
		start := num("startSize", 23)
		if horizontal {
			frame.Height = start
		} else {
			frame.Width = start
		}
	}
	switch st["labelPosition"] {
	case "left":
		frame.X -= cell.Width
	case "right":
		frame.X += cell.Width
	}
	switch st["verticalLabelPosition"] {
	case "top":
		frame.Y -= cell.Height
	case "bottom":
		frame.Y += cell.Height
	}
	cx, cy := frame.X+frame.Width/2, frame.Y+frame.Height/2
	if !horizontal {
		// Laid out across the cell, then turned a quarter left.
		frame = Box{X: cx - frame.Height/2, Y: cy - frame.Width/2, Width: frame.Height, Height: frame.Width}
	}
	sp := num("spacing", textDefaultSpacing)
	spL, spR := sp+num("spacingLeft", 0), sp+num("spacingRight", 0)
	spT, spB := sp+num("spacingTop", 0), sp+num("spacingBottom", 0)
	avail := frame.Width - spL - spR

	// Lay the lines out: greedy wrap at the frame's width.
	wrap := st["whiteSpace"] == "wrap"
	var tw, th float64
	lines, anyText := 0, false
	for _, hl := range hard {
		words, lineSize := splitWords(hl, metric)
		if len(words) == 0 {
			th += lineSize * textLineHeight
			lines++
			continue
		}
		anyText = true
		var w, top float64
		started := false
		for _, wd := range words {
			if started && wrap && w+wd.space+wd.width > avail+0.01 {
				tw, th, lines = max(tw, w), th+top*textLineHeight, lines+1
				w, top, started = 0, 0, false
			}
			if started {
				w += wd.space
			}
			w += wd.width
			top = max(top, wd.size)
			started = true
		}
		tw, th, lines = max(tw, w), th+top*textLineHeight, lines+1
	}
	if !anyText {
		return Box{}, 0
	}
	tw *= slack
	if slack > 1 {
		tw++
	}

	var x, y float64
	switch st["align"] {
	case "left":
		x = frame.X + spL
	case "right":
		x = frame.X + frame.Width - spR - tw
	default:
		x = frame.X + spL + (avail-tw)/2
	}
	switch st["verticalAlign"] {
	case "top":
		y = frame.Y + spT + textTopShift
	case "bottom":
		y = frame.Y + frame.Height - spB - th
	default:
		y = frame.Y + spT + (frame.Height-spT-spB-th)/2
	}
	b := Box{X: x, Y: y, Width: tw, Height: th}
	if !horizontal {
		dx, dy := b.X+b.Width/2-cx, b.Y+b.Height/2-cy
		// A quarter turn left: (dx, dy) → (dy, -dx).
		b = Box{X: cx + dy - b.Height/2, Y: cy - dx - b.Width/2, Width: b.Height, Height: b.Width}
	}
	if rot := math.Mod(num("rotation", 0), 360); rot != 0 {
		b = rotateBox(b, Point{X: cell.X + cell.Width/2, Y: cell.Y + cell.Height/2}, rot)
	}
	switch st["overflow"] {
	case "hidden", "fill":
		b = clipBox(b, cell)
	}
	return b, lines
}

// rotateBox is the upright box around b turned deg degrees about c.
func rotateBox(b Box, c Point, deg float64) Box {
	rad := deg * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, p := range []Point{{X: b.X, Y: b.Y}, {X: b.X + b.Width, Y: b.Y}, {X: b.X, Y: b.Y + b.Height}, {X: b.X + b.Width, Y: b.Y + b.Height}} {
		dx, dy := p.X-c.X, p.Y-c.Y
		x, y := c.X+dx*cos-dy*sin, c.Y+dx*sin+dy*cos
		minX, minY, maxX, maxY = math.Min(minX, x), math.Min(minY, y), math.Max(maxX, x), math.Max(maxY, y)
	}
	return Box{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

// clipBox is the part of b inside to.
func clipBox(b, to Box) Box {
	x0, y0 := math.Max(b.X, to.X), math.Max(b.Y, to.Y)
	x1, y1 := math.Min(b.X+b.Width, to.X+to.Width), math.Min(b.Y+b.Height, to.Y+to.Height)
	if x1 <= x0 || y1 <= y0 {
		return Box{}
	}
	return Box{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}
