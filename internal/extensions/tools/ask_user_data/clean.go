// SPDX-License-Identifier: Apache-2.0

package ask_user_data

import (
	"strings"
	"unicode"
)

// cleanStrings returns the submission with every string value trimmed of
// surrounding whitespace and stripped of control characters other than
// newline and tab, recursively through objects and arrays.
//
// A form field submitted as "Infolog\n" went verbatim into a search query
// (dossier run of 2026-09-22): the widget hands back what the input holds,
// and a trailing newline or a stray control character is never what the
// user meant. Multi-line text keeps its newlines and tabs; nothing else
// below 0x20 survives, nor DEL.
func cleanStrings(v map[string]any) map[string]any {
	if v == nil {
		return nil
	}
	out := make(map[string]any, len(v))
	for k, val := range v {
		out[k] = cleanValue(val)
	}
	return out
}

func cleanValue(v any) any {
	switch x := v.(type) {
	case string:
		return cleanString(x)
	case map[string]any:
		return cleanStrings(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = cleanValue(e)
		}
		return out
	default:
		return v
	}
}

func cleanString(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}
