// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"errors"
	"strings"
	"testing"
)

// TestParseURI_HappyPath covers every documented valid URI form.
func TestParseURI_HappyPath(t *testing.T) {
	cases := []struct {
		in   string
		want ResourceURI
	}{
		// Simple kebab id
		{"ktw-docs:/skill/doc-create",
			ResourceURI{"ktw-docs", "skill", "doc-create"}},

		// Tree path with extension (the "file-like" use)
		{"ktw-docs:/reference/diagrams/c4-context.md",
			ResourceURI{"ktw-docs", "reference", "diagrams/c4-context.md"}},

		// PascalCase id (data type Go-source name)
		{"ktw-xxx:/data-type/EmailMessage",
			ResourceURI{"ktw-xxx", "data-type", "EmailMessage"}},

		// Deeply nested file path with extension
		{"ktw-example:/file/config/prod/cluster.yaml",
			ResourceURI{"ktw-example", "file", "config/prod/cluster.yaml"}},

		// Hyphenated provider + type (already kebab)
		{"ktw-sample:/data-type/GmailMessage",
			ResourceURI{"ktw-sample", "data-type", "GmailMessage"}},
	}
	for _, c := range cases {
		got, err := ParseResourceURI(c.in)
		if err != nil {
			t.Errorf("ParseResourceURI(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseResourceURI(%q) = %+v, want %+v", c.in, got, c.want)
		}
		// Round-trip: String() should produce the original.
		if got.String() != c.in {
			t.Errorf("Round-trip: ParseResourceURI(%q).String() = %q", c.in, got.String())
		}
	}
}

// TestParseURI_ErrorCases covers each documented rejection.
func TestParseURI_ErrorCases(t *testing.T) {
	cases := []struct {
		in      string
		wantSub string
	}{
		{"no-separator-here", "missing ':/'"},
		{"ktw-docs:noslash", "missing ':/'"},
		{"ktw-docs:/", "missing '/<id>'"},
		{"ktw-docs:/skill", "missing '/<id>'"},
		// :/skill/x is now VALID (empty provider = current binary per the command-surface decision)
		// Removed expected-error case.
		{"Ktw-Docs:/skill/x", "invalid char"}, // uppercase in provider
		{"ktw-docs:/Skill/x", "invalid char"}, // uppercase in type
		{"ktw-docs:/skill/", "id: empty"},
		{"ktw-docs:/sk_ill/x", "invalid char"}, // underscore not kebab
	}
	for _, c := range cases {
		_, err := ParseResourceURI(c.in)
		if err == nil {
			t.Errorf("ParseResourceURI(%q): expected error containing %q, got nil", c.in, c.wantSub)
			continue
		}
		if !strings.Contains(err.Error(), c.wantSub) {
			t.Errorf("ParseResourceURI(%q): error = %q, want substring %q", c.in, err.Error(), c.wantSub)
		}
	}
}

// TestParseURI_RejectsWildcards confirms wildcards in concrete URI are errors.
func TestParseURI_RejectsWildcards(t *testing.T) {
	cases := []string{
		"*:/skill/x",
		"ktw-docs:/*/x",
		// IDs are loose — '*' allowed in concrete URI's ID field since it
		// could in theory be a real filename. The pattern parser is the
		// right tool for matching "any ID" — concrete parsing of "*" in
		// the ID is preserved verbatim (literal). Documented behaviour.
	}
	for _, c := range cases {
		_, err := ParseResourceURI(c)
		if !errors.Is(err, ErrInvalidURI) && err == nil {
			t.Errorf("ParseResourceURI(%q): expected wildcard rejection, got nil", c)
		}
	}
}

// TestListFilter_Matches confirms server-side filter logic.
func TestListFilter_Matches(t *testing.T) {
	uri := ResourceURI{"ktw-docs", "skill", "doc-create"}

	cases := []struct {
		filter ListFilter
		want   bool
		note   string
	}{
		{ListFilter{}, true, "empty matches everything"},
		{ListFilter{Providers: []string{"ktw-docs"}}, true, "matching provider"},
		{ListFilter{Providers: []string{"ktw-example"}}, false, "non-matching provider"},
		{ListFilter{Providers: []string{"ktw-example", "ktw-docs"}}, true, "any-of providers"},
		{ListFilter{Types: []string{"task"}}, false, "non-matching type"},
		{ListFilter{IDGlob: "doc-*"}, true, "glob"},
		{ListFilter{IDGlob: "xyz-*"}, false, "non-matching glob"},
		// Combined: AND semantics
		{ListFilter{Providers: []string{"ktw-docs"}, Types: []string{"skill"}}, true, "AND match"},
		{ListFilter{Providers: []string{"ktw-docs"}, Types: []string{"task"}}, false, "AND fail on type"},
	}
	for _, c := range cases {
		got := c.filter.Matches(uri)
		if got != c.want {
			t.Errorf("[%s] %+v.Matches(%s) = %v, want %v",
				c.note, c.filter, uri.String(), got, c.want)
		}
	}
}

// TestGlobMatch covers the local glob — crucial because we deviate from
// stdlib path.Match by allowing '*' to cross '/' (tree-path IDs).
func TestGlobMatch(t *testing.T) {
	cases := []struct {
		glob, s string
		want    bool
	}{
		{"*", "anything", true},
		{"doc-*", "doc-create", true},
		{"doc-*", "task-create", false},
		// '*' crosses '/' — this is the deliberate stdlib deviation
		{"diagrams/*", "diagrams/c4-context.md", true},
		{"diagrams/*.md", "diagrams/c4-context.md", true},
		{"diagrams/*/foo", "diagrams/sub/foo", true},
		// '?' matches one char
		{"doc-?", "doc-a", true},
		{"doc-?", "doc-ab", false},
		// Exact (no wildcards)
		{"doc-create", "doc-create", true},
		{"doc-create", "doc-other", false},
	}
	for _, c := range cases {
		got := globMatch(c.glob, c.s)
		if got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.glob, c.s, got, c.want)
		}
	}
}
