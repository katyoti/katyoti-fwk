// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

// An inline definition is read whole: its body is the agent's prompt, its
// top-level keys (as a delegated worker's or a fork's are written) and its
// config block (as a persona's is) are its config.
func TestAnInlineDefinitionKeepsItsBodyAndConfig(t *testing.T) {
	flat := inlineTask(map[string]any{
		"name": "prompt-worker", "type": "agent", "description": "d",
		"capability": "smart-best", "plugins": []any{"augmented"}, "body": "do it",
	})
	if flat.Body != "do it" || flat.Config["capability"] != "smart-best" || flat.Config["plugins"] == nil {
		t.Fatalf("flat = body %q config %v; want the body and its keys as config", flat.Body, flat.Config)
	}
	for _, meta := range []string{"name", "type", "description", "body"} {
		if _, ok := flat.Config[meta]; ok {
			t.Errorf("config carries the meta key %q", meta)
		}
	}
	block := inlineTask(map[string]any{
		"name": "specifier", "type": "agent-loop", "body": "You are the specifier.",
		"config": map[string]any{"plugins": []any{"augmented", "cwm"}, "profileName": "smart-cost"},
	})
	if block.Body != "You are the specifier." || block.Config["profileName"] != "smart-cost" || len(block.Config["plugins"].([]any)) != 2 {
		t.Fatalf("block = body %q config %v; want the body and the config block", block.Body, block.Config)
	}
	if _, ok := block.Config["config"]; ok {
		t.Error("the config block is nested in config")
	}
}
