// SPDX-License-Identifier: Apache-2.0

package anchor

import (
	"strings"
	"testing"
)

func TestExtractMarkdownSectionMatchesPath(t *testing.T) {
	md := `# Authentication

Intro paragraph.

## Rate Limits

Limit 100 rps per key.
Burst: 200.

## OAuth Scopes

scope.read, scope.write.
`
	got, ok := extractMarkdownSection(md, []string{"Authentication", "Rate Limits"})
	if !ok {
		t.Fatalf("expected match")
	}
	want := []string{"Limit 100 rps per key.", "Burst: 200."}
	for _, line := range want {
		if !strings.Contains(got, line) {
			t.Errorf("section missing %q:\n%s", line, got)
		}
	}
	if strings.Contains(got, "scope.read") {
		t.Errorf("section leaked into next subsection: %q", got)
	}
}

func TestExtractMarkdownSectionMissingPath(t *testing.T) {
	md := "# Heading\n\nBody.\n"
	if _, ok := extractMarkdownSection(md, []string{"Other"}); ok {
		t.Errorf("missing path should return ok=false")
	}
}

func TestExtractMarkdownSectionNestedDepths(t *testing.T) {
	md := `# A

## B

### C

c content
c more
`
	got, ok := extractMarkdownSection(md, []string{"A", "B", "C"})
	if !ok {
		t.Fatalf("expected match")
	}
	if !strings.Contains(got, "c content") || !strings.Contains(got, "c more") {
		t.Errorf("nested extraction missed lines: %q", got)
	}
}

func TestExtractMarkdownSectionEmptyPathReturnsAll(t *testing.T) {
	md := "anything goes\nmore content"
	got, ok := extractMarkdownSection(md, nil)
	if !ok {
		t.Fatalf("expected match")
	}
	if got != md {
		t.Errorf("empty path: got %q want %q", got, md)
	}
}

func TestExtractMarkdownSectionStopsAtSameDepthHeading(t *testing.T) {
	md := `## First

first content

## Second

second content
`
	got, ok := extractMarkdownSection(md, []string{"First"})
	if !ok {
		t.Fatalf("expected match")
	}
	if !strings.Contains(got, "first content") {
		t.Errorf("missing first content: %q", got)
	}
	if strings.Contains(got, "second content") {
		t.Errorf("leaked into Second: %q", got)
	}
}

func TestParseHeading(t *testing.T) {
	cases := map[string]struct {
		depth int
		text  string
		ok    bool
	}{
		"# H1":             {1, "H1", true},
		"## H2":            {2, "H2", true},
		"### H3":           {3, "H3", true},
		"###### H6":        {6, "H6", true},
		"####### too-deep": {0, "", false},
		"#nospace":         {0, "", false},
		"":                 {0, "", false},
		"not a heading":    {0, "", false},
		"## title ##":      {2, "title", true},
		"  ## indented":    {2, "indented", true},
		"    too indented": {0, "", false},
	}
	for in, want := range cases {
		d, txt, ok := parseHeading(in)
		if d != want.depth || txt != want.text || ok != want.ok {
			t.Errorf("parseHeading(%q) = (%d, %q, %v) want (%d, %q, %v)",
				in, d, txt, ok, want.depth, want.text, want.ok)
		}
	}
}
