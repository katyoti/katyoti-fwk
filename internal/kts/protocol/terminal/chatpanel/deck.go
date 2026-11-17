// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	slidedeck "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// A deck's value is its markdown: patch-ui-component edits one slide by its
// number (`target: "slide-3"`, 1 the first, vertical slides in document
// order) without resending the deck - the GUI's editDeck
// (app/ktw-display-lib/src/ui-builders/deck/edit.ts), here so the bridge
// keeps the deck's markdown. replace slide-N, add slide-N (before it),
// remove slide-N, append (at the end); each op applies to the result of the
// previous one; the frontmatter and every other slide are kept byte for byte.

var slideTarget = regexp.MustCompile(`^slide-(\d+)$`)

type slideSpan struct{ start, end, sep int }

func slideSpans(lines []string, md string) []slideSpan {
	slides := slidedeck.Parse(md).Slides
	out := make([]slideSpan, len(slides))
	for i, s := range slides {
		start := s.Line - 1
		end := len(lines)
		if i+1 < len(slides) {
			end = slides[i+1].Line - 2
		}
		sep := start - 1
		if i == 0 {
			sep = -1
		}
		out[i] = slideSpan{start: start, end: end, sep: sep}
	}
	return out
}

func slideBody(v any) ([]string, bool) {
	s, ok := v.(string)
	if !ok {
		return nil, false
	}
	s = strings.Trim(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	return append(append([]string{""}, strings.Split(s, "\n")...), ""), true
}

// editDeck applies the ops to the deck's markdown; an op that cannot apply
// is said in errs and the others still are.
func editDeck(markdown string, ops []terminal.UIPatchOp) (string, []error) {
	md := strings.ReplaceAll(markdown, "\r\n", "\n")
	var errs []error
	for k, o := range ops {
		name := o.Kind
		if o.Kind == "rfc6902" {
			name = o.Op // the agent's add travels as {kind: rfc6902, op: add}
		}
		where := fmt.Sprintf("op[%d] %s %s", k, name, o.Target)
		val, err := decodeRaw(o.Value)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		lines := strings.Split(md, "\n")
		all := slideSpans(lines, md)
		if name == "append" {
			body, ok := slideBody(val)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: value must be the new slide's markdown", where))
				continue
			}
			for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
				lines = lines[:len(lines)-1]
			}
			md = strings.Join(append(append(lines, "", "---"), body...), "\n")
			continue
		}
		m := slideTarget.FindStringSubmatch(o.Target)
		if m == nil {
			errs = append(errs, fmt.Errorf("%s: target must be slide-<n> (1 = the first slide)", where))
			continue
		}
		n, _ := strconv.Atoi(m[1])
		if n < 1 || n > len(all) {
			errs = append(errs, fmt.Errorf("%s: the deck has %d slides", where, len(all)))
			continue
		}
		s := all[n-1]
		switch name {
		case "replace":
			body, ok := slideBody(val)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: value must be the slide's markdown", where))
				continue
			}
			lines = splice(lines, s.start, s.end-s.start, body...)
		case "add":
			body, ok := slideBody(val)
			if !ok {
				errs = append(errs, fmt.Errorf("%s: value must be the new slide's markdown", where))
				continue
			}
			if s.sep >= 0 {
				lines = splice(lines, s.sep, 0, append([]string{"---"}, body...)...)
			} else {
				lines = splice(lines, s.start, 0, append(body, "---")...)
			}
		case "remove":
			switch {
			case len(all) == 1:
				lines = splice(lines, s.start, s.end-s.start, "")
			case s.sep >= 0:
				lines = splice(lines, s.sep, s.end-s.sep)
			default:
				lines = splice(lines, s.start, all[1].sep+1-s.start)
			}
		default:
			errs = append(errs, fmt.Errorf("%s: a deck takes replace, add, remove or append", where))
			continue
		}
		md = strings.Join(lines, "\n")
	}
	return md, errs
}

// splice is JavaScript's Array.prototype.splice on a copy: count elements
// from start replaced by items.
func splice(s []string, start, count int, items ...string) []string {
	if start > len(s) {
		start = len(s)
	}
	if start+count > len(s) {
		count = len(s) - start
	}
	out := make([]string, 0, len(s)-count+len(items))
	out = append(out, s[:start]...)
	out = append(out, items...)
	return append(out, s[start+count:]...)
}
