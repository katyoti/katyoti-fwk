// SPDX-License-Identifier: Apache-2.0

package local

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// A TASK.md `config:` key the agent request does not have is an error naming
// the key, not a silent default. Three embedded tasks carried `max_turns`
// (json tag `maxTurns`) and `permission_mode` (no such field) for months;
// every one of them ran on defaults.
func TestDecodeAgentConfig_RefusesUnknownKeysAndKeepsTheKnownOnes(t *testing.T) {
	var req api.AgentRunRequest
	data, _ := json.Marshal(map[string]any{"maxTurns": 3, "mode": "chat", "capability": "smart", "body": "do it", "_target_id": "x"})
	if err := decodeAgentConfig(data, &req); err != nil {
		t.Fatalf("known keys, yaml-only keys and substrate keys must decode: %v", err)
	}
	if req.MaxTurns != 3 || req.Mode != "chat" {
		t.Errorf("known keys not applied: %+v", req)
	}

	data, _ = json.Marshal(map[string]any{"max_turns": 3})
	err := decodeAgentConfig(data, &req)
	if err == nil || !strings.Contains(err.Error(), `"max_turns"`) {
		t.Fatalf("an unknown key must be refused by name, got %v", err)
	}
	data, _ = json.Marshal(map[string]any{"permission_mode": "default"})
	if err := decodeAgentConfig(data, &req); err == nil {
		t.Fatal("a key that is no field at all must be refused")
	}
}
