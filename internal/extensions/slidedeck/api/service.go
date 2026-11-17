// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// DeckService is the slide-deck engine's service: what the platform runs on a
// slide-deck object. Hosted by ktw-docs.
type DeckService interface {
	// LintObject is the lint hook of a slide-deck object: the deck composed
	// from the object (ComposeDeck), linted against the grammar and the
	// layouts in reach. `object-lint` / lint() run it.
	LintObject(ctx context.Context, in DeckObjectInput) (ObjectLintOutput, error)
	// ImportTheme turns a PowerPoint template into a deck theme: the
	// importer's proposal with the decisions applied, as a report (a dry
	// run), or the theme folder written to Out. The theme import task runs it.
	ImportTheme(ctx context.Context, in ImportThemeInput) (ImportThemeOutput, error)
	// View is the Deck panel's source: a slide-deck object's deck, composed
	// (ComposeDeck) and marked (MarkDeck), to display.
	View(ctx context.Context, in ViewInput) (render.Def, error)
}

// ViewInput names the slide-deck object the Deck panel shows.
type ViewInput struct {
	ID string `json:"id" jsonschema:"description=The slide-deck object; required"`
}

// DeckServiceDef describes the service for KTSP/CLI auto-generation — the
// hook URI is `ktw-docs:deck/lint-object`, the importer
// `ktw-docs:deck/import-theme`.
var DeckServiceDef = meta.ServiceDef{
	Name:    "deck",
	Short:   "The slide-deck engine: checks a slide-deck object's deck and imports a PowerPoint template as a theme",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"LintObject":  meta.Method("The lint hook of a slide-deck object: its composed deck against the grammar and the layouts, each finding naming its slide"),
		"View":        meta.Method("The Deck panel's source: a slide-deck object's deck composed from its fields and its slides with the production mark written in"),
		"ImportTheme": meta.Method("Turns a PowerPoint template into a deck theme: the proposal with the decisions applied as a report, or the theme written into the pack rooted at folder"),
	},
}

// DeckObjectInput is a slide-deck object as its lint hook passes it — the
// same three parts the Deck panel composes (hooks/deck-lint.yaml binds them
// with `${current…}`: the object a hook receives by default carries no prose
// body).
type DeckObjectInput struct {
	Title string           `json:"title,omitempty" jsonschema:"The object's name"`
	Deck  DeckObjectFields `json:"deck,omitempty"  jsonschema:"The object's Deck settings"`
	Body  string           `json:"body,omitempty"  jsonschema:"The object's body: the slides"`
}

// ObjectLintFinding is one finding of the lint hook.
type ObjectLintFinding struct {
	Rule     string `json:"rule"`
	Message  string `json:"message"`
	Severity string `json:"severity" jsonschema:"error or warn or info"`
}

// ObjectLintOutput is a lint hook's answer: {status, findings}.
type ObjectLintOutput struct {
	Status   string              `json:"status" jsonschema:"ok or findings"`
	Findings []ObjectLintFinding `json:"findings"`
}

// ComposeDeck is a slide-deck object's deck: a frontmatter from its title and
// its Deck settings, then its body (the slides). The Deck panel composes the
// same deck from the same parts for display (object-panels/deck.yaml); a test
// holds the two together.
func ComposeDeck(in DeckObjectInput) string {
	var b strings.Builder
	b.WriteString("---\n")
	for _, kv := range [][2]string{
		{"title", in.Title}, {"aspect", in.Deck.Aspect}, {"theme", in.Deck.Theme},
		{"transition", in.Deck.Transition}, {"author", in.Deck.Author},
		{"watermark", in.Deck.Watermark}, {"mark", in.Deck.Mark},
	} {
		if kv[1] != "" {
			q, _ := json.Marshal(kv[1]) // a JSON string is a YAML double-quoted scalar
			b.WriteString(kv[0])
			b.WriteString(": ")
			b.Write(q)
			b.WriteByte('\n')
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(in.Body)
	return b.String()
}

// DeckObjectOf is ComposeDeck's inverse: a deck file split into what a
// slide-deck object holds — its title, its Deck settings, its body (the
// slides, without the frontmatter). How a deck shown in a chat becomes an
// object of the workspace.
func DeckObjectOf(md string) DeckObjectInput {
	d := Parse(md)
	str := func(k string) string {
		if v, ok := d.Settings[k]; ok && v != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return ""
	}
	lines := splitLines(md)
	body := md
	if has, end := frontmatter(lines); has && end > 0 {
		body = strings.Join(lines[end+1:], "\n")
	}
	return DeckObjectInput{
		Title: str("title"),
		Deck: DeckObjectFields{
			Aspect: str("aspect"), Theme: str("theme"), Transition: str("transition"),
			Author: str("author"), Watermark: str("watermark"), Mark: str("mark"),
		},
		Body: strings.TrimLeft(body, "\n"),
	}
}

// LintObjectDeck lints a slide-deck object's composed deck against g. Each message names
// the slide so the author can find it in the body.
func LintObjectDeck(in DeckObjectInput, g Grammar) ObjectLintOutput {
	md := ComposeDeck(in)
	deck := Parse(md)
	out := ObjectLintOutput{Status: "ok", Findings: []ObjectLintFinding{}}
	if bodyHasFrontmatter(in.Body) {
		out.Findings = append(out.Findings, ObjectLintFinding{Rule: "body-frontmatter", Severity: SeverityError,
			Message: "the body holds only the slides: remove its leading `---` block — its settings (title, theme, aspect…) are the object's name and Deck fields"})
	}
	for _, f := range Lint(md, g) {
		msg := f.Message
		if n := slideAt(deck, f.Line); n > 0 {
			msg = "slide " + strconv.Itoa(n) + ": " + msg
		}
		out.Findings = append(out.Findings, ObjectLintFinding{Rule: f.Rule, Message: msg, Severity: f.Severity})
	}
	if len(out.Findings) > 0 {
		out.Status = "findings"
	}
	return out
}

// bodyHasFrontmatter reports a body that opens with a frontmatter block — a
// `---` line, then only `key: value` lines, then `---`: what a deck file
// starts with, copied into an object whose settings are its fields. Composed,
// it would show as a slide of settings.
func bodyHasFrontmatter(body string) bool {
	lines := splitLines(strings.TrimLeft(body, " \t\r\n"))
	has, end := frontmatter(lines)
	if !has || end < 2 {
		return false
	}
	for _, l := range lines[1:end] {
		if t := strings.TrimSpace(l); t != "" && !frontmatterKeyRe.MatchString(t) {
			return false
		}
	}
	return true
}

var frontmatterKeyRe = regexp.MustCompile(`^[A-Za-z][\w-]*:(\s|$)`)

// slideAt is the number (from 1) of the slide holding source line n, or 0.
func slideAt(d Deck, n int) int {
	at := 0
	for i, s := range d.Slides {
		if s.Line <= n {
			at = i + 1
		}
	}
	return at
}
