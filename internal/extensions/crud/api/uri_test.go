// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"testing"
)

// TestParseURI_HappyPaths exercises the canonical relative-form
// shapes from cwm-v3 §2.4 / §5.1.
func TestParseURI_HappyPaths(t *testing.T) {
	cases := []struct {
		name       string
		uri        URI
		wantPath   string
		wantSegs   []string
		wantType   string
		wantLast   string
		wantQueryK string
		wantQueryV string
	}{
		{
			name:     "top-level collection",
			uri:      "book",
			wantPath: "book", wantSegs: []string{"book"},
			wantType: "book", wantLast: "book",
		},
		{
			name:     "top-level record",
			uri:      "book/123",
			wantPath: "book/123", wantSegs: []string{"book", "123"},
			wantType: "book", wantLast: "123",
		},
		{
			name:     "scoped collection",
			uri:      "node/X/item",
			wantPath: "node/X/item", wantSegs: []string{"node", "X", "item"},
			wantType: "node", wantLast: "item",
		},
		{
			name:     "scoped record",
			uri:      "node/X/item/Y",
			wantPath: "node/X/item/Y", wantSegs: []string{"node", "X", "item", "Y"},
			wantType: "node", wantLast: "Y",
		},
		{
			name:     "discriminated collection with query",
			uri:      "node/X/item?type=todo",
			wantPath: "node/X/item", wantSegs: []string{"node", "X", "item"},
			wantType: "node", wantLast: "item",
			wantQueryK: "type", wantQueryV: "todo",
		},
		{
			name:     "trailing slash stripped",
			uri:      "book/",
			wantPath: "book", wantSegs: []string{"book"},
			wantType: "book", wantLast: "book",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParseURI(tc.uri)
			if err != nil {
				t.Fatalf("ParseURI(%q): %v", tc.uri, err)
			}
			if p.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", p.Path, tc.wantPath)
			}
			if len(p.Segments) != len(tc.wantSegs) {
				t.Errorf("Segments length = %d, want %d", len(p.Segments), len(tc.wantSegs))
			}
			for i, s := range tc.wantSegs {
				if i < len(p.Segments) && p.Segments[i] != s {
					t.Errorf("Segments[%d] = %q, want %q", i, p.Segments[i], s)
				}
			}
			if got := p.Type(); got != tc.wantType {
				t.Errorf("Type() = %q, want %q", got, tc.wantType)
			}
			if got := p.LastSegment(); got != tc.wantLast {
				t.Errorf("LastSegment() = %q, want %q", got, tc.wantLast)
			}
			if tc.wantQueryK != "" {
				if got := p.Query.Get(tc.wantQueryK); got != tc.wantQueryV {
					t.Errorf("Query.Get(%q) = %q, want %q", tc.wantQueryK, got, tc.wantQueryV)
				}
			}
		})
	}
}

// TestParseURI_ErrorCases exercises the rejection grammar.
func TestParseURI_ErrorCases(t *testing.T) {
	cases := []struct {
		name string
		uri  URI
	}{
		{"empty", ""},
		{"whitespace-only", "   "},
		{"absolute-scheme", "cwm:/node/X/item"},
		{"leading-slash", "/book/123"},
		{"empty-segment-double-slash", "book//123"},
		{"malformed-query", "book?%ZZ=bad"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseURI(tc.uri)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("ParseURI(%q): err = %v, want ErrInvalidInput chain", tc.uri, err)
			}
		})
	}
}

// TestParsedURI_ScopedID exercises the flat-record-reference helper.
func TestParsedURI_ScopedID(t *testing.T) {
	t.Run("collection-only", func(t *testing.T) {
		p, _ := ParseURI("book")
		typ, id, err := p.ScopedID()
		if err != nil {
			t.Fatalf("ScopedID: %v", err)
		}
		if typ != "book" || id != "" {
			t.Errorf("got (%q, %q), want (book, \"\")", typ, id)
		}
	})
	t.Run("type-and-id", func(t *testing.T) {
		p, _ := ParseURI("book/123")
		typ, id, err := p.ScopedID()
		if err != nil {
			t.Fatalf("ScopedID: %v", err)
		}
		if typ != "book" || id != "123" {
			t.Errorf("got (%q, %q), want (book, 123)", typ, id)
		}
	})
	t.Run("nested-rejected", func(t *testing.T) {
		p, _ := ParseURI("node/X/item/Y")
		_, _, err := p.ScopedID()
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("ScopedID on nested: err = %v, want ErrInvalidInput", err)
		}
	})
}
