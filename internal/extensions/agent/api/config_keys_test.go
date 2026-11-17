// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"testing"
)

// TestUnknownAgentConfigKeys pins the agent strategy's config-key rule: json
// tags of AgentRunRequest (any case), the yaml-only keys and `_` keys pass;
// a snake_case or invented key is refused.
func TestUnknownAgentConfigKeys(t *testing.T) {
	got := UnknownConfigKeys([]string{"maxTurns", "MaxTurns", "capability", "prompt", "body", "_mode", "plugins", "permission_mode", "max_turns"})
	want := []string{"max_turns", "permission_mode"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UnknownConfigKeys = %v, want %v", got, want)
	}
	if got := UnknownConfigKeys(nil); len(got) != 0 {
		t.Fatalf("UnknownConfigKeys(nil) = %v, want none", got)
	}
}
