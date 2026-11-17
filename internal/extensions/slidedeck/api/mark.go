// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"strings"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
)

// MarkDeck writes the production mark into a deck's frontmatter as
// production_mark: the one definition (docsapi.MarkOf, modules/doc-engine/),
// read with the deck's own mark (on or off) and classification. The theme
// only places the line; a deck whose mark is off, or a universe that
// switched it off, gets none. What shows or exports a deck runs it first.
func MarkDeck(ctx context.Context, md string) string {
	d := Parse(md)
	str := func(k string) string {
		if v, ok := d.Settings[k]; ok && v != nil {
			if b, isBool := v.(bool); isBool {
				if b {
					return "on"
				}
				return "off"
			}
			if s, isStr := v.(string); isStr {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	line := ""
	if m, ok := docsapi.MarkOf(ctx, docsapi.DocMark{Mark: str("mark"), Classification: str("classification")}); ok {
		line = m.Line()
	}
	return withSetting(md, "production_mark", line)
}

// withSetting sets one key of a deck's frontmatter (removes it when value is
// empty), adding the frontmatter when the deck has none.
func withSetting(md, key, value string) string {
	lines := splitLines(md)
	var entry string
	if value != "" {
		q, _ := json.Marshal(value) // a JSON string is a YAML double-quoted scalar
		entry = key + ": " + string(q)
	}
	has, end := frontmatter(lines)
	if has && end < 0 {
		return md // an unclosed frontmatter: the lint says so
	}
	if !has {
		if entry == "" {
			return md
		}
		return "---\n" + entry + "\n---\n\n" + md
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[0])
	for _, l := range lines[1:end] {
		if strings.HasPrefix(l, key+":") {
			continue
		}
		out = append(out, l)
	}
	if entry != "" {
		out = append(out, entry)
	}
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}
