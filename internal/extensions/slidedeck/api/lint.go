// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"gopkg.in/yaml.v3"
)

// Finding severities.
const (
	SeverityError = "error"
	SeverityWarn  = "warn"
	SeverityInfo  = "info"
)

// Finding is one thing wrong with a deck, at a source line, worded so an
// agent can fix it and try again.
type Finding struct {
	Line     int    `json:"line"     jsonschema:"description=Source line from 1"`
	Rule     string `json:"rule"     jsonschema:"description=Stable rule code"`
	Severity string `json:"severity" jsonschema:"description=error blocks the deck; warn and info do not,enum=error|warn|info"`
	Message  string `json:"message"  jsonschema:"description=What is wrong and how to fix it"`
}

// HasErrors reports whether any finding is an error.
func HasErrors(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// declared holds the keys of DeckSettings and SlideSettings, read from their
// TypeAST: the lint checks exactly what the data-types publish.
var declared = sync.OnceValue(func() struct{ deck, slide map[string]*schemaapi.Type } {
	deck, err := ktwreflect.TypeAST[DeckSettings]()
	if err != nil {
		panic(err)
	}
	slide, err := ktwreflect.TypeAST[SlideSettings]()
	if err != nil {
		panic(err)
	}
	return struct{ deck, slide map[string]*schemaapi.Type }{deck.Fields, slide.Fields}
})

// iconRe is an icon written as a markdown image: `![label](icon:<name>)`.
var iconRe = regexp.MustCompile(`\]\(icon:([^)\s]*)\)`)

// inlineCode is a code span on one line: `…` (or “…“).
var inlineCode = regexp.MustCompile("(`+)[^`]*?(`+)")

// Lint checks a deck against the declared grammar and the layouts and themes
// of g. With no layouts (the catalog is unavailable) the layout and slot
// checks are skipped rather than guessed; with no themes, the theme, variant
// and icon checks are.
func Lint(src string, g Grammar) []Finding {
	layouts := g.Layouts
	var out []Finding
	add := func(line int, sev, rule, msg string, args ...any) {
		out = append(out, Finding{Line: line, Rule: rule, Severity: sev, Message: fmt.Sprintf(msg, args...)})
	}
	lines := splitLines(src)
	d := Parse(src)

	has, end := frontmatter(lines)
	switch {
	case !has:
		add(1, SeverityError, "frontmatter-missing", "a deck starts with a `---` frontmatter block holding at least `title:`")
	case end < 0:
		add(1, SeverityError, "frontmatter-unclosed", "the frontmatter opened on line 1 is never closed by a `---` line")
		return out
	default:
		lintFrontmatter(lines[1:end], add)
	}
	if d.Open == "code" {
		add(d.OpenLine, SeverityError, "code-fence-unclosed", "the code block opened here is never closed; everything after it is code")
	}
	if len(d.Slides) == 0 {
		add(max(end+2, 1), SeverityError, "deck-empty", "the deck has no slide; write slides separated by `---` lines")
		return out
	}
	theme := lintTheme(d, g, frontmatterLine(lines, end, "theme"), add)
	if theme != nil {
		lintAspect(d, *theme, frontmatterLine(lines, end, "aspect"), add)
	}

	byName := map[string]LayoutBody{}
	for _, l := range layouts {
		byName[l.Name] = l
	}
	settingsLines := map[int]bool{}
	for _, s := range d.Slides {
		if s.SettingsLine > 0 {
			settingsLines[s.SettingsLine] = true
			lintSlideSettings(lines[s.SettingsLine-1], s.SettingsLine, add)
		}
		if inc := s.Settings["include"]; inc != "" {
			lintInclude(s, inc, g, add)
			continue
		}
		if len(layouts) > 0 {
			lintSlots(s, byName, add)
		}
		if theme != nil {
			lintVariant(s, *theme, add)
			lintPalette(s, *theme, add)
		}
	}

	// A settings line anywhere but a slide's first line is text to reveal.js:
	// the slide silently loses its settings.
	start := 0
	if has {
		start = end + 1
	}
	kinds, _ := classify(lines[start:])
	// A chart block's data, checked when its fence closes (chart.go).
	chartLine, chartKind := 0, ""
	var chartData []string
	for i, k := range kinds {
		n := start + i + 1
		switch {
		case k == kFence && chartLine == 0:
			if kind, ok := IsChartFence(lines[start+i]); ok {
				chartLine, chartKind, chartData = n, kind, nil
			}
		case k == kFence:
			if _, err := ParseChart(chartKind, chartData); err != nil {
				add(chartLine, SeverityError, "chart-invalid", "%v", err)
			}
			chartLine = 0
		case k == kCode && chartLine > 0:
			chartData = append(chartData, lines[start+i])
		}
		if k == kSettings && !settingsLines[n] {
			add(n, SeverityError, "settings-not-first", "a `<!-- slide: … -->` line must be the first line of its slide; move it up or it is ignored")
		}
		if theme != nil && k == kText {
			// An icon written inside inline code is text, not an image.
			for _, m := range iconRe.FindAllStringSubmatch(inlineCode.ReplaceAllString(lines[start+i], ""), -1) {
				if _, ok := theme.Icons[m[1]]; !ok {
					add(start+i+1, SeverityError, "icon-unknown", "theme %q has no icon %q; %s", theme.Name, m[1], known("its icons", sortedKeys(theme.Icons)))
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Line < out[j].Line })
	return out
}

func lintFrontmatter(fm []string, add func(int, string, string, string, ...any)) {
	var m map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(fm, "\n")), &m); err != nil {
		add(1, SeverityError, "frontmatter-invalid", "the frontmatter is not valid YAML: %v", err)
		return
	}
	keyLine := func(key string) int {
		for i, l := range fm {
			if strings.HasPrefix(l, key+":") {
				return i + 2
			}
		}
		return 1
	}
	fields := declared().deck
	if t, _ := m["title"].(string); strings.TrimSpace(t) == "" {
		add(1, SeverityError, "deck-title-missing", "the frontmatter needs a non-empty `title:`")
	}
	for _, k := range sortedKeys(m) {
		f, ok := fields[k]
		if !ok {
			add(keyLine(k), SeverityWarn, "deck-key-unknown", "unknown deck setting %q; known: %s", k, strings.Join(sortedKeys(fields), ", "))
			continue
		}
		if m[k] == nil {
			continue // `key:` with no value: not set
		}
		if msg := enumViolation(f, fmt.Sprint(m[k])); msg != "" {
			add(keyLine(k), SeverityError, "deck-value-invalid", "%s: %s", k, msg)
		}
	}
}

func lintSlideSettings(line string, n int, add func(int, string, string, string, ...any)) {
	m := settingsRe.FindStringSubmatch(strings.TrimSpace(line))
	pairs, rest := ParseSettings(m[1])
	if rest != "" {
		add(n, SeverityError, "slide-settings-invalid", "cannot read %q; write settings as key=value, quoting values with spaces", rest)
	}
	fields := declared().slide
	for _, k := range sortedKeys(pairs) {
		f, ok := fields[k]
		if !ok {
			add(n, SeverityError, "slide-key-unknown", "unknown slide setting %q; known: %s", k, strings.Join(sortedKeys(fields), ", "))
			continue
		}
		if msg := enumViolation(f, pairs[k]); msg != "" {
			add(n, SeverityError, "slide-value-invalid", "%s: %s", k, msg)
		}
	}
}

// lintInclude checks a slide showing a slide group: it holds nothing else,
// and names a group in reach (when the reach is the universe's; else the name
// is not checked, and the lint says so).
func lintInclude(s Slide, name string, g Grammar, add func(int, string, string, string, ...any)) {
	line := s.SettingsLine
	if len(s.Settings) > 1 || len(s.Slots) > 0 || s.Notes != "" {
		add(line, SeverityError, "include-not-alone", "a slide that includes the group %q holds nothing else: no other setting, no content, no notes", name)
	}
	for _, n := range g.Groups {
		if n == name {
			return
		}
	}
	if g.GroupsComplete {
		known := "none"
		if len(g.Groups) > 0 {
			known = strings.Join(g.Groups, ", ")
		}
		add(line, SeverityError, "group-unknown", "unknown slide group %q; known: %s", name, known)
		return
	}
	add(line, SeverityInfo, "group-unchecked", "slide group %q was not checked (it is not among the groups in reach)", name)
}

func lintSlots(s Slide, layouts map[string]LayoutBody, add func(int, string, string, string, ...any)) {
	name := s.Settings["layout"]
	if name == "" {
		name = DefaultLayout
	}
	// A slide-level finding points at the settings line, else the first content.
	line := s.SettingsLine
	if line == 0 && len(s.Slots) > 0 {
		line = s.Slots[0].Line
	}
	if line == 0 {
		line = s.Line
	}
	l, ok := layouts[name]
	if !ok {
		add(line, SeverityError, "layout-unknown", "unknown layout %q; known: %s", name, strings.Join(sortedKeys(layouts), ", "))
		return
	}
	declaredSlots := map[string]LayoutSlot{}
	for _, sl := range l.Slots {
		declaredSlots[sl.Name] = sl
	}
	seen := map[string]bool{}
	for _, c := range s.Slots {
		if seen[c.Name] {
			add(c.Line, SeverityError, "slot-duplicate", "slot %q appears twice in this slide", c.Name)
		}
		seen[c.Name] = true
		if _, ok := declaredSlots[c.Name]; !ok {
			add(c.Line, SeverityError, "slot-unknown", "layout %q has no slot %q; its slots: %s", name, c.Name, strings.Join(sortedKeys(declaredSlots), ", "))
		}
	}
	for _, c := range s.Slots {
		sl, ok := declaredSlots[c.Name]
		if !ok || (sl.MinItems == 0 && sl.MaxItems == 0) {
			continue
		}
		n := listItems(c.Markdown)
		advice := ""
		switch {
		case sl.MaxItems > 0 && n > sl.MaxItems:
			advice = "split the slide or merge items"
		case sl.MinItems > 0 && n < sl.MinItems:
			advice = "add items or pick another layout"
		default:
			continue
		}
		add(c.Line, SeverityError, "slot-items", "layout %q slot %q takes %s list items; it has %d — %s",
			name, c.Name, itemRange(sl), n, advice)
	}
	for _, sl := range l.Slots {
		if !sl.Required {
			continue
		}
		filled := false
		for _, c := range s.Slots {
			filled = filled || (c.Name == sl.Name && strings.TrimSpace(c.Markdown) != "")
		}
		if !filled {
			add(line, SeverityError, "slot-missing", "layout %q needs content in slot %q", name, sl.Name)
		}
	}
}

// enumViolation says what is wrong with v for a field with a value set.
func enumViolation(f *schemaapi.Type, v string) string {
	if len(f.Enum) == 0 {
		return ""
	}
	var allowed []string
	for _, e := range f.Enum {
		if fmt.Sprint(e) == v {
			return ""
		}
		allowed = append(allowed, fmt.Sprint(e))
	}
	return fmt.Sprintf("%q is not one of %s", v, strings.Join(allowed, ", "))
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// lintTheme checks the deck's theme against g and returns it when the variant
// and icon checks can use it. A theme g does not hold is unknown (an error)
// only when g holds every theme there is; otherwise it is unchecked (info,
// once), and so are the deck's variants and icons.
func lintTheme(d Deck, g Grammar, line int, add func(int, string, string, string, ...any)) *ThemeBody {
	named, _ := d.Settings["theme"].(string)
	name := named
	if name == "" {
		name = DefaultTheme
	}
	if t, ok := g.Theme(name); ok {
		return &t
	}
	switch {
	case named == "":
		// The default theme, not in reach: nothing the author wrote to check.
	case g.ThemesComplete:
		names := make([]string, 0, len(g.Themes))
		for _, t := range g.Themes {
			names = append(names, t.Name)
		}
		add(line, SeverityError, "theme-unknown", "unknown theme %q; %s", named, known("known", names))
	case named != DefaultTheme:
		add(line, SeverityInfo, "theme-unchecked", "theme %q was not checked (it is not among the themes in reach); nor were its variants and icons", named)
	}
	return nil
}

// lintAspect warns when the deck sets an aspect other than the one its theme
// is drawn for: the theme's logo, rules, footer and mark are placed for that
// shape, and on another they are stretched. A deck that sets none takes its
// theme's.
func lintAspect(d Deck, theme ThemeBody, line int, add func(int, string, string, string, ...any)) {
	aspect, _ := d.Settings["aspect"].(string)
	if aspect == "" || theme.Aspect == "" || aspect == theme.Aspect {
		return
	}
	add(line, SeverityWarn, "theme-aspect", "theme %q is drawn for %s, not %s: its logo, rules and footer are stretched on this deck; remove `aspect:` to take the theme's", theme.Name, theme.Aspect, aspect)
}

// lintVariant checks a slide's variant against the deck's theme.
func lintVariant(s Slide, theme ThemeBody, add func(int, string, string, string, ...any)) {
	v := s.Settings["variant"]
	if v == "" {
		return
	}
	layout := s.Settings["layout"]
	if layout == "" {
		layout = DefaultLayout
	}
	if !theme.HasVariant(layout, v) {
		add(s.SettingsLine, SeverityError, "variant-unknown", "theme %q has no variant %q for layout %q; %s", theme.Name, v, layout, known("its variants", theme.VariantNames(layout)))
	}
}

// lintPalette checks a slide's background colour against the theme's palette:
// a theme that declares one (a brand charter's) allows no other colour. An
// image background is not a colour and is not checked here.
func lintPalette(s Slide, theme ThemeBody, add func(int, string, string, string, ...any)) {
	bg := s.Settings["background"]
	if bg == "" || !IsColour(bg) || theme.AllowsColour(bg) {
		return
	}
	colours := make([]string, 0, len(theme.Palette))
	for _, c := range theme.Palette {
		colours = append(colours, c.Value+" ("+c.Name+")")
	}
	add(s.SettingsLine, SeverityError, "theme-palette", "theme %q allows only the colours of its palette; %q is not one of them: %s", theme.Name, bg, strings.Join(colours, ", "))
}

// listItem is a top-level item of a markdown list: `- `, `* `, `+ `, `1. ` or
// `1) ` indented by at most three spaces.
var listItem = regexp.MustCompile(`^ {0,3}([-*+]|\d{1,9}[.)])(\s|$)`)

// listItems counts the top-level list items of a slot's markdown, outside
// code blocks.
func listItems(md string) int {
	lines := splitLines(md)
	kinds, _ := classify(lines)
	n := 0
	for i, l := range lines {
		if kinds[i] == kText && listItem.MatchString(l) {
			n++
		}
	}
	return n
}

func itemRange(sl LayoutSlot) string {
	switch {
	case sl.MinItems > 0 && sl.MaxItems > 0:
		return fmt.Sprintf("%d to %d", sl.MinItems, sl.MaxItems)
	case sl.MaxItems > 0:
		return fmt.Sprintf("at most %d", sl.MaxItems)
	default:
		return fmt.Sprintf("at least %d", sl.MinItems)
	}
}

// frontmatterLine is the source line of a frontmatter key, else line 1.
func frontmatterLine(lines []string, end int, key string) int {
	for i := 1; i < end && i < len(lines); i++ {
		if strings.HasPrefix(lines[i], key+":") {
			return i + 1
		}
	}
	return 1
}

func known(what string, names []string) string {
	if len(names) == 0 {
		return what + ": none"
	}
	return what + ": " + strings.Join(names, ", ")
}
