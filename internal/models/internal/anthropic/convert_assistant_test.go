// SPDX-License-Identifier: Apache-2.0

package anthropic

import (
	"encoding/json"
	"strings"
	"testing"

	api "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// TestConvertAssistantParts_EmptyToolArgsMarshals is the regression test
// for the no-arg-tool crash: a tool call with empty Arguments ("") used to
// become json.RawMessage("") — invalid JSON — so the Anthropic SDK failed
// to marshal the ToolUseBlockParam ("unexpected end of JSON input"),
// crashing the entire next request. Any agent calling a no-arg tool
// (scripts-list, check-messages, …) hit this. The fix normalizes empty
// args to "{}". This test marshals the converted block to prove it.
func TestConvertAssistantParts_EmptyToolArgsMarshals(t *testing.T) {
	for _, args := range []string{"", "   ", "\n"} {
		parts := []api.ContentPart{
			api.ToolCall{
				ID:           "toolu_x",
				Type:         "function",
				FunctionCall: &api.FunctionCall{Name: "scripts-list", Arguments: args},
			},
		}
		blocks, err := convertAssistantParts(parts)
		if err != nil {
			t.Fatalf("convertAssistantParts(args=%q): %v", args, err)
		}
		if len(blocks) != 1 {
			t.Fatalf("args=%q: want 1 block, got %d", args, len(blocks))
		}
		// The crash was at MARSHAL time — prove the block serializes.
		raw, err := json.Marshal(blocks[0])
		if err != nil {
			t.Fatalf("args=%q: marshal block: %v", args, err)
		}
		if !strings.Contains(string(raw), `"input":{}`) {
			t.Errorf("args=%q: expected normalized empty input {}; got %s", args, raw)
		}
	}
}

// TestConvertAssistantParts_NonEmptyToolArgsPreserved confirms the
// normalization does not clobber real arguments.
func TestConvertAssistantParts_NonEmptyToolArgsPreserved(t *testing.T) {
	parts := []api.ContentPart{
		api.ToolCall{
			ID:           "toolu_y",
			Type:         "function",
			FunctionCall: &api.FunctionCall{Name: "cwm_item_get", Arguments: `{"id":"abc"}`},
		},
	}
	blocks, err := convertAssistantParts(parts)
	if err != nil {
		t.Fatalf("convertAssistantParts: %v", err)
	}
	raw, err := json.Marshal(blocks[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"id":"abc"`) {
		t.Errorf("real args lost: %s", raw)
	}
}
