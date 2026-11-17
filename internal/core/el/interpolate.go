// SPDX-License-Identifier: Apache-2.0

package el

import (
	"encoding/json"
	"strings"
)

// INTERPOLATION. A string may carry `${expr}` templates. A string that is ONE
// template keeps the expression's typed value; templates embedded in text are
// spliced in as strings. How an expression is evaluated is the caller's (an
// env and a cache, or answers already computed), so every surface resolving
// templates — actions, hooks, panels — reads them the same way.

// Interpolate resolves the templates of v when it is a string; any other value
// is returned as is. eval answers one expression (without the `${ }`, trimmed)
// and whether it resolved. An unresolved whole-string template stays the
// template; an unresolved embedded one becomes empty.
func Interpolate(v any, eval func(expr string) (any, bool)) any {
	s, ok := v.(string)
	if !ok || !strings.Contains(s, "${") {
		return v
	}
	if start, expr, end, ok := nextTemplate(s); ok && start == 0 && end == len(s) {
		if out, ok := eval(expr); ok {
			return out
		}
		return s
	}
	var b strings.Builder
	rest := s
	for {
		start, expr, end, ok := nextTemplate(rest)
		if !ok {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		if out, ok := eval(expr); ok {
			b.WriteString(AsString(out))
		}
		rest = rest[end:]
	}
	return b.String()
}

// TemplateExpressions lists the expressions of every `${...}` in s, trimmed,
// in order — what Interpolate would ask eval for.
func TemplateExpressions(s string) []string {
	var out []string
	rest := s
	for {
		_, expr, end, ok := nextTemplate(rest)
		if !ok {
			return out
		}
		out = append(out, expr)
		rest = rest[end:]
	}
}

// nextTemplate is THE template scanner, shared by Interpolate and
// TemplateExpressions. It finds the first `${` of s and the `}` that closes
// it: brackets ({…}, […], (…)) nest and quoted strings ('…', "…" with
// backslash escapes, `…` raw) are skipped, so a map literal or a quoted brace
// stays inside the expression. It returns where the template starts, its
// expression (trimmed) and the index just past its closing brace. ok is false
// when s has no template or the first one never closes — the rest of s is
// then text.
func nextTemplate(s string) (start int, expr string, end int, ok bool) {
	start = strings.Index(s, "${")
	if start < 0 {
		return 0, "", 0, false
	}
	depth := 0
	for i := start + 2; i < len(s); i++ {
		switch c := s[i]; c {
		case '\'', '"', '`':
			j := skipQuoted(s, i)
			if j < 0 {
				return 0, "", 0, false
			}
			i = j
		case '{', '[', '(':
			depth++
		case ']', ')':
			if depth > 0 {
				depth--
			}
		case '}':
			if depth == 0 {
				return start, strings.TrimSpace(s[start+2 : i]), i + 1, true
			}
			depth--
		}
	}
	return 0, "", 0, false
}

// skipQuoted returns the index of the quote closing the string opened at
// s[open], or -1 when it never closes. A backslash escapes the next byte,
// except in a raw (backquoted) string.
func skipQuoted(s string, open int) int {
	q := s[open]
	for i := open + 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if q != '`' {
				i++
			}
		case q:
			return i
		}
	}
	return -1
}

// AsString is a value as text: a string as is, anything else as its JSON.
func AsString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}
