// SPDX-License-Identifier: Apache-2.0

package anchor

import "strings"

// headingFrame is one entry on the heading stack used by
// extractMarkdownSection. Package-level so hasSuffix can reference
// the same named type as the producer.
type headingFrame struct {
	depth int
	text  string
}

// extractMarkdownSection finds the section of `content` whose
// heading breadcrumb matches `path` and returns its body (the
// text between that heading and the next heading at the same or
// shallower depth). The matched heading line itself is excluded
// from the output — only the section's content.
//
// Path semantics:
//
//   - Empty path: returns the whole document, ok=true.
//   - Path matches: the heading stack must contain each path
//     element in order. E.g. path=["Authentication", "Rate
//     Limits"] matches when we're inside a `## Rate Limits`
//     nested under a `# Authentication` (or deeper levels of
//     each — only the relative order matters).
//   - Path doesn't match: ("", false).
//
// Walk algorithm:
//
//   - Maintain a slice of headings (depth + text) we're currently
//     inside.
//   - Each ATX heading line (`# …`, `## …`, …) pops the stack to
//     its depth-1 then pushes the new heading.
//   - When the stack ENDS with `path` (as a suffix), we're inside
//     the requested section.
//   - Section content begins at the first line AFTER the matching
//     heading and continues until we leave the section (a new
//     heading at depth ≤ matched-heading-depth).
//
// The matcher is tolerant: trailing `#` on closed-ATX headings
// (`## title ##`) is stripped; setext headings (===, ---) are
// NOT supported in v1.
func extractMarkdownSection(content string, path []string) (string, bool) {
	if len(path) == 0 {
		return content, true
	}

	lines := strings.Split(content, "\n")
	var stack []headingFrame
	var (
		insideMatch  bool
		matchedDepth int
		captureStart int
		captureEnd   int
		found        bool
	)

	pop := func(toDepth int) {
		for len(stack) > 0 && stack[len(stack)-1].depth >= toDepth {
			stack = stack[:len(stack)-1]
		}
	}

	closeMatch := func(endIdx int) {
		if !insideMatch {
			return
		}
		captureEnd = endIdx
		insideMatch = false
		found = true
	}

	for i, raw := range lines {
		depth, text, ok := parseHeading(raw)
		if !ok {
			continue
		}

		// Leaving the matched region — a heading at depth ≤ the
		// matched heading's depth closes the section.
		if insideMatch && depth <= matchedDepth {
			closeMatch(i)
		}

		// Update the heading stack.
		pop(depth)
		stack = append(stack, headingFrame{depth: depth, text: text})

		// Check if the current stack ENDS with the requested path
		// (suffix match — extra ancestors are fine).
		if !insideMatch && hasSuffix(stack, path) {
			insideMatch = true
			matchedDepth = depth
			captureStart = i + 1
			// captureEnd is set when we exit the section (or EOF).
		}
	}
	if insideMatch {
		closeMatch(len(lines))
	}

	if !found {
		return "", false
	}
	return strings.Join(lines[captureStart:captureEnd], "\n"), true
}

// parseHeading recognizes ATX-style markdown headings (`#`,
// `##`, …) up to depth 6. Returns (depth, title-text, true) for
// a heading line, (0, "", false) otherwise.
//
// Stripping rules:
//   - leading `#` characters set depth (counted up to 6).
//   - exactly one space MUST follow the `#`s.
//   - trailing run of `#` characters on closed-ATX headings is
//     stripped along with any surrounding spaces.
func parseHeading(line string) (int, string, bool) {
	s := line
	// Lead can be optional spaces (≤ 3 per CommonMark).
	leading := 0
	for leading < len(s) && leading < 3 && s[leading] == ' ' {
		leading++
	}
	s = s[leading:]
	if s == "" || s[0] != '#' {
		return 0, "", false
	}
	depth := 0
	for depth < len(s) && s[depth] == '#' {
		depth++
	}
	if depth == 0 || depth > 6 {
		return 0, "", false
	}
	rest := s[depth:]
	if rest == "" {
		// `###` with no following content — treat as empty heading.
		return depth, "", true
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false // `#abc` is not a heading.
	}
	rest = strings.TrimLeft(rest, " \t")
	// Strip closed-ATX `#` suffix.
	if i := strings.LastIndexFunc(rest, func(r rune) bool {
		return r != '#'
	}); i >= 0 {
		// Trim trailing `#` only when preceded by whitespace.
		core := rest[:i+1]
		core = strings.TrimRight(core, " \t")
		return depth, core, true
	}
	return depth, strings.TrimSpace(rest), true
}

// hasSuffix reports whether stack ends with the requested path.
// Path elements compared by exact string equality. Extra
// ancestors above the path are allowed.
func hasSuffix(stack []headingFrame, path []string) bool {
	if len(path) > len(stack) {
		return false
	}
	offset := len(stack) - len(path)
	for i, want := range path {
		if stack[offset+i].text != want {
			return false
		}
	}
	return true
}
