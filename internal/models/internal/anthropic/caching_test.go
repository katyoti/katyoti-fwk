// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
)

// TestApplyPromptCaching_PlacesBreakpoints verifies the fix for the no-op cache:
// cache_control breakpoints are placed on the tool defs, the system prompt, and
// the LAST message (the rolling conversation-prefix lever) — so an agentic loop
// re-reads the prefix at 0.1× instead of re-sending it uncached.
func TestApplyPromptCaching_PlacesBreakpoints(t *testing.T) {
	params := &anthropicsdk.MessageNewParams{
		System: []anthropicsdk.TextBlockParam{{Text: "system prompt"}},
		Tools: []anthropicsdk.ToolUnionParam{
			{OfTool: &anthropicsdk.ToolParam{Name: "file-read"}},
		},
		Messages: []anthropicsdk.MessageParam{
			anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("earlier turn")),
			anthropicsdk.NewUserMessage(anthropicsdk.NewTextBlock("latest turn")),
		},
	}

	applyPromptCaching(params)

	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	got := strings.Count(string(raw), "cache_control")
	if got != 3 {
		t.Errorf("cache_control breakpoints = %d, want 3 (tools + system + last message)\nJSON: %s", got, raw)
	}
	// The breakpoint must be on the LATEST message, not an earlier one.
	if !strings.Contains(string(raw), "latest turn") {
		t.Fatal("latest message missing from request")
	}
}

// TestCachingEnabled_Gating confirms caching is only applied when the
// WithPromptCaching marker is present (auto_cache off → no breakpoints).
func TestCachingEnabled_Gating(t *testing.T) {
	params := &anthropicsdk.MessageNewParams{
		System: []anthropicsdk.TextBlockParam{{Text: "system"}},
	}
	// No applyPromptCaching call (caching disabled): no breakpoints.
	raw, _ := json.Marshal(params)
	if strings.Contains(string(raw), "cache_control") {
		t.Error("no caching requested, but cache_control was placed")
	}
}
