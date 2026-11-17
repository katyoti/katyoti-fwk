// SPDX-License-Identifier: Apache-2.0

package create_document

import (
	"strings"
	"testing"
)

// TestStreamParser_Basic verifies that feed() correctly extracts content
// deltas from partial JSON as it arrives chunk by chunk.
func TestStreamParser_Basic(t *testing.T) {
	state := newDocStreamState("tool-1")

	// Simulate LLM streaming a tool-input JSON in chunks.
	chunks := []string{
		`{"title":"My `,
		`Report","mediaType":`,
		`"text/markdown","content":"# Hello\n\n`,
		`This is a test.\n\n`,
		`End of report."}`,
	}

	var allDeltas []string
	for _, chunk := range chunks {
		deltas := state.feed(chunk)
		allDeltas = append(allDeltas, deltas...)
	}

	// Title + mediaType should be extracted
	if state.title != "My Report" {
		t.Errorf("title = %q, want %q", state.title, "My Report")
	}
	if state.mediaType != "text/markdown" {
		t.Errorf("mediaType = %q, want %q", state.mediaType, "text/markdown")
	}

	// All deltas concatenated should equal the full content (decoded).
	full := strings.Join(allDeltas, "")
	want := "# Hello\n\nThis is a test.\n\nEnd of report."
	if full != want {
		t.Errorf("reconstructed content = %q, want %q", full, want)
	}
}

// TestStreamParser_ProgressiveDeltas verifies that content arrives as
// multiple deltas (not all at once at the end).
func TestStreamParser_ProgressiveDeltas(t *testing.T) {
	state := newDocStreamState("tool-1")

	state.feed(`{"title":"T","mediaType":"text/markdown","content":"Hello`)
	deltas1 := state.feed(` wor`)
	deltas2 := state.feed(`ld"}`)

	if len(deltas1) == 0 {
		t.Error("expected deltas from second chunk")
	}
	if len(deltas2) == 0 {
		t.Error("expected deltas from final chunk")
	}

	all := ""
	for _, d := range deltas1 {
		all += d
	}
	for _, d := range deltas2 {
		all += d
	}
	if !strings.Contains(all, "world") {
		t.Errorf("expected 'world' in deltas, got %q", all)
	}
}

// TestStreamParser_EscapedQuote verifies that escaped quotes inside
// "content" don't falsely terminate the string.
func TestStreamParser_EscapedQuote(t *testing.T) {
	state := newDocStreamState("tool-1")

	chunks := []string{
		`{"title":"T","mediaType":"text/markdown","content":"She said \"hello\" to me."}`,
	}
	var all []string
	for _, c := range chunks {
		all = append(all, state.feed(c)...)
	}

	joined := strings.Join(all, "")
	want := `She said "hello" to me.`
	if joined != want {
		t.Errorf("content = %q, want %q", joined, want)
	}
}

// TestStreamParser_SplitEscape verifies that an escape sequence split
// across chunks doesn't break the parser.
func TestStreamParser_SplitEscape(t *testing.T) {
	state := newDocStreamState("tool-1")

	// Split the \n escape across two chunks
	state.feed(`{"title":"T","mediaType":"text/markdown","content":"Line1\`)
	deltas2 := state.feed(`nLine2"}`)

	all := ""
	for _, d := range deltas2 {
		all += d
	}

	// Should have the newline and Line2
	if !strings.Contains(all, "\n") {
		t.Errorf("expected newline in deltas, got %q", all)
	}
	if !strings.Contains(all, "Line2") {
		t.Errorf("expected 'Line2' in deltas, got %q", all)
	}
}

// TestStreamParser_TitleMediaTypeExtraction verifies that title and mediaType
// are extracted even when "content" comes first in the JSON.
func TestStreamParser_TitleMediaTypeExtraction(t *testing.T) {
	state := newDocStreamState("tool-1")

	// Content first, then title and mediaType
	state.feed(`{"content":"Hello","title":"MyDoc","mediaType":"text/html"}`)

	if state.title != "MyDoc" {
		t.Errorf("title = %q, want MyDoc", state.title)
	}
	if state.mediaType != "text/html" {
		t.Errorf("mediaType = %q, want text/html", state.mediaType)
	}
}
