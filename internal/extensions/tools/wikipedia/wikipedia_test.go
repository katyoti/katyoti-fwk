// SPDX-License-Identifier: Apache-2.0

package wikipedia

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmptyQuery(t *testing.T) {
	_, err := executeWikipedia(nil, WikiInput{Query: ""})
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Errorf("err = %v, want 'query is required'", err)
	}
}

func TestUnknownAction(t *testing.T) {
	_, err := executeWikipedia(nil, WikiInput{Action: "invalid", Query: "test"})
	if err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("err = %v, want 'unknown action'", err)
	}
}

func TestDescribe_Search(t *testing.T) {
	raw, _ := json.Marshal(WikiInput{Query: "Albert Einstein"})
	got := describeWikipedia(raw)
	if got != `wikipedia search query="Albert Einstein"` {
		t.Errorf("got %q", got)
	}
}

func TestDescribe_Summary(t *testing.T) {
	raw, _ := json.Marshal(WikiInput{Action: "summary", Query: "Go (programming language)"})
	got := describeWikipedia(raw)
	if got != `wikipedia summary query="Go (programming language)"` {
		t.Errorf("got %q", got)
	}
}

func TestDescribe_LongQuery(t *testing.T) {
	long := strings.Repeat("x", 100)
	raw, _ := json.Marshal(WikiInput{Query: long})
	got := describeWikipedia(raw)
	if !strings.Contains(got, "...") {
		t.Error("should truncate long queries")
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"<b>bold</b>", "bold"},
		{"<span class=\"highlight\">text</span> more", "text more"},
		{"no <em>tags</em> here", "no tags here"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripHTML(tt.in)
		if got != tt.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// ── Live integration tests (require network) ──────────────────────────

func TestSearch_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	out, err := executeWikipedia(context.Background(), WikiInput{Query: "Albert Einstein"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Results) == 0 {
		t.Error("no results")
	}
	found := false
	for _, r := range out.Results {
		if strings.Contains(r.Title, "Einstein") {
			found = true
			break
		}
	}
	if !found {
		t.Error("no Einstein-related result")
	}
}

func TestSummary_Live(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	out, err := executeWikipedia(context.Background(), WikiInput{Action: "summary", Query: "Albert Einstein"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Title == "" {
		t.Error("empty title")
	}
	if !strings.Contains(out.Summary, "physicist") {
		t.Logf("Summary = %q (expected mention of physicist)", out.Summary[:min(200, len(out.Summary))])
	}
	if out.URL == "" {
		t.Error("empty URL")
	}
}

func TestSummary_French(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	out, err := executeWikipedia(context.Background(), WikiInput{Action: "summary", Query: "Paris", Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != "Paris" {
		t.Errorf("title = %q", out.Title)
	}
	if !strings.Contains(out.URL, "fr.wikipedia.org") {
		t.Errorf("URL should be French wikipedia, got %q", out.URL)
	}
}

func TestSummary_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	_, err := executeWikipedia(context.Background(), WikiInput{Action: "summary", Query: "Xyzzy_nonexistent_page_12345"})
	if err == nil || !strings.Contains(err.Error(), "no article found") {
		t.Errorf("err = %v, want 'no article found'", err)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
