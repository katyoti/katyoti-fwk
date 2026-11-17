// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"testing"
)

// A part with caching instructions reads back whole — the part it wraps and its
// cache control — as every other part does (a saved conversation keeps it).
func TestACachedPartReadsBackWhole(t *testing.T) {
	in := MessageContent{Role: ChatMessageTypeHuman, Parts: []ContentPart{
		WithCacheControl(TextContent{Text: "the long context"}, &CacheControl{Type: "ephemeral"}),
		TextContent{Text: "the question"},
	}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out MessageContent
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("read back %s: %v", b, err)
	}
	if len(out.Parts) != 2 {
		t.Fatalf("parts = %#v", out.Parts)
	}
	cached, ok := out.Parts[0].(CachedContent)
	if !ok {
		t.Fatalf("first part = %#v, want a CachedContent (saved as %s)", out.Parts[0], b)
	}
	if txt, _ := cached.ContentPart.(TextContent); txt.Text != "the long context" || cached.CacheControl == nil || cached.CacheControl.Type != "ephemeral" {
		t.Errorf("cached part = %#v, want its text and its cache control", cached)
	}
}
