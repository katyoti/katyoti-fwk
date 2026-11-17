// SPDX-License-Identifier: Apache-2.0

package api

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The splitting rules. They are the one part of the grammar the TypeScript
// parser (app/ktw-display-lib/src/ui-builders/deck/parse.ts) also holds; the
// corpus under testdata/corpus holds the two together.
var (
	settingsRe = regexp.MustCompile(`^<!--\s*slide:(.*?)-->$`)
	pairRe     = regexp.MustCompile(`([A-Za-z][A-Za-z0-9_-]*)=("([^"]*)"|[^\s"]+)`)
	slotRe     = regexp.MustCompile(`^::([a-z][a-z0-9-]*)::$`)
	noteRe     = regexp.MustCompile(`^Note:`)
)

// lineKind is what one source line is to the grammar.
type lineKind int

const (
	kText lineKind = iota
	kBlank
	kFence    // opens or closes a fenced code block
	kCode     // inside a fenced code block
	kSepH     // `---`: a new horizontal slide
	kSepV     // `--`: a new vertical slide
	kSettings // `<!-- slide: … -->`
	kSlot     // `::name::`
	kNote     // `Note:` — the rest of the slide is speaker notes
)

// classify tags each line of body, fence-aware: nothing inside a fenced code
// block is a separator, a settings line, a slot marker or a note. It returns
// the source line (from 1) of a fence left open at the end, or 0.
func classify(lines []string) (kinds []lineKind, openFence int) {
	kinds = make([]lineKind, len(lines))
	var fenceChar byte
	var fenceLen, fenceLine int
	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if fenceLen > 0 {
			if indent <= 3 && fenceRun(trimmed, fenceChar) >= fenceLen && strings.TrimLeft(trimmed, string(fenceChar)) == "" {
				kinds[i] = kFence
				fenceLen = 0
			} else {
				kinds[i] = kCode
			}
			continue
		}
		if indent <= 3 && len(trimmed) > 0 && (trimmed[0] == '`' || trimmed[0] == '~') {
			if n := fenceRun(trimmed, trimmed[0]); n >= 3 {
				kinds[i] = kFence
				fenceChar, fenceLen, fenceLine = trimmed[0], n, i+1
				continue
			}
		}
		switch {
		case line == "":
			kinds[i] = kBlank
		case line == "---":
			kinds[i] = kSepH
		case line == "--":
			kinds[i] = kSepV
		case settingsRe.MatchString(strings.TrimSpace(line)):
			kinds[i] = kSettings
		case slotRe.MatchString(line):
			kinds[i] = kSlot
		case noteRe.MatchString(line):
			kinds[i] = kNote
		default:
			kinds[i] = kText
		}
	}
	if fenceLen > 0 {
		return kinds, fenceLine
	}
	return kinds, 0
}

func fenceRun(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// splitLines normalises line endings and splits.
func splitLines(src string) []string {
	return strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
}

// frontmatter locates the frontmatter block: the YAML between a first line
// `---` and the next `---`. has reports a block was opened; end is the index
// of its closing line, or -1 when the source ends inside it.
func frontmatter(lines []string) (has bool, end int) {
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return false, -1
	}
	for j := 1; j < len(lines); j++ {
		if strings.TrimRight(lines[j], " \t") == "---" {
			return true, j
		}
	}
	return true, -1
}

// ParseSettings reads the key=value pairs of a settings line body (the text
// between `slide:` and `-->`). rest holds what is not a pair — empty when the
// line is well formed.
func ParseSettings(body string) (pairs map[string]string, rest string) {
	pairs = map[string]string{}
	for _, m := range pairRe.FindAllStringSubmatch(body, -1) {
		v := m[2]
		if strings.HasPrefix(v, `"`) {
			v = m[3]
		}
		pairs[m[1]] = v
	}
	return pairs, strings.TrimSpace(pairRe.ReplaceAllString(body, ""))
}

// Parse splits a deck. It never fails: a source cut anywhere — mid-stream —
// parses to the slides it holds so far, and Open names the construct it ended
// inside. What is wrong with a deck is the lint's to say.
func Parse(src string) Deck {
	d := Deck{Settings: map[string]any{}, Slides: []Slide{}}
	lines := splitLines(src)
	start := 0
	if has, end := frontmatter(lines); has {
		if end < 0 {
			d.Open, d.OpenLine = "frontmatter", 1
			return d
		}
		var m map[string]any
		if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &m); err == nil && m != nil {
			d.Settings = m
		}
		start = end + 1
	}
	body := lines[start:]
	kinds, open := classify(body)
	if open > 0 {
		d.Open, d.OpenLine = "code", start+open
	}

	type chunk struct {
		vertical bool
		from, to int // body indexes, to exclusive
	}
	var chunks []chunk
	cur := chunk{from: 0}
	for i, k := range kinds {
		if k == kSepH || k == kSepV {
			cur.to = i
			chunks = append(chunks, cur)
			cur = chunk{vertical: k == kSepV, from: i + 1}
		}
	}
	cur.to = len(body)
	chunks = append(chunks, cur)

	// A blank slide is dropped. A vertical slide whose horizontal parent was
	// dropped starts a new horizontal slide: the `---` still opened a section.
	h, v := -1, 0
	parentDropped := false
	for _, c := range chunks {
		s, ok := buildSlide(body[c.from:c.to], kinds[c.from:c.to], start+c.from+1)
		if !ok {
			if !c.vertical {
				parentDropped = true
			}
			continue
		}
		if c.vertical && h >= 0 && !parentDropped {
			v++
		} else {
			h, v = h+1, 0
		}
		parentDropped = false
		s.H, s.V = h, v
		d.Slides = append(d.Slides, s)
	}
	return d
}

// buildSlide reads one slide from its lines; first is the source line of
// lines[0]. A slide with no settings, no content and no notes is no slide.
func buildSlide(lines []string, kinds []lineKind, first int) (Slide, bool) {
	s := Slide{Line: first, Slots: []SlotContent{}}
	i := 0
	for i < len(lines) && kinds[i] == kBlank {
		i++
	}
	if i < len(lines) && kinds[i] == kSettings {
		m := settingsRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
		s.Settings, _ = ParseSettings(m[1])
		s.SettingsLine = first + i
		i++
	}

	name, slotLine, from := DefaultSlot, 0, i
	flush := func(to int) {
		md, firstContent := trimBlank(lines[from:to])
		if name == DefaultSlot {
			if md == "" {
				return
			}
			slotLine = first + from + firstContent
		}
		s.Slots = append(s.Slots, SlotContent{Name: name, Line: slotLine, Markdown: md})
	}
	for ; i < len(lines); i++ {
		switch kinds[i] {
		case kSlot:
			flush(i)
			name = slotRe.FindStringSubmatch(strings.TrimRight(lines[i], " \t"))[1]
			slotLine, from = first+i, i+1
		case kNote:
			flush(i)
			rest := append([]string{strings.TrimSpace(strings.TrimPrefix(lines[i], "Note:"))}, lines[i+1:]...)
			s.Notes, _ = trimBlank(rest)
			return s, true
		}
	}
	flush(len(lines))
	return s, s.SettingsLine > 0 || len(s.Slots) > 0 || s.Notes != ""
}

// trimBlank joins lines, dropping blank lines at both ends and trailing
// whitespace on each line; first is the index of the first kept line.
func trimBlank(lines []string) (md string, first int) {
	a, b := 0, len(lines)
	for a < b && strings.TrimSpace(lines[a]) == "" {
		a++
	}
	for b > a && strings.TrimSpace(lines[b-1]) == "" {
		b--
	}
	out := make([]string, 0, b-a)
	for _, l := range lines[a:b] {
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return strings.Join(out, "\n"), a
}
