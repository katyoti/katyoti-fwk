// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// TestResolveFromExecRequest_UserCommsInput_AutoDetection pins the
// auto-detection rule at the factory boundary — the resolve step that
// converts TaskExecRequest into AgentRunRequest must finalize
// UserCommsInput according to:
//
//   - Empty incoming + Mode=chat     → "direct"     (interactive chat)
//   - Empty incoming + Mode=worker   → "inbox"      (headless worker)
//   - Empty incoming + Mode=remote   → "none"       (remote preset)
//   - Empty incoming + Mode=teammate → "via_leader"
//   - Non-empty incoming (preset/frontmatter) → trust as-is
//
// This is the only place EffectiveUserCommsInput runs in the request
// resolver chain; downstream code reads UserCommsInput verbatim.
func TestResolveFromExecRequest_UserCommsInput_AutoDetection(t *testing.T) {
	f := NewAgentMachineFactory(nil, nil)

	cases := []struct {
		name      string
		config    map[string]any
		wantInput string
	}{
		{
			name:      "chat mode → preset default direct",
			config:    map[string]any{"mode": "chat", "prompt": "hi"},
			wantInput: "direct",
		},
		{
			name:      "headless worker: empty incoming → preset default inbox",
			config:    map[string]any{"mode": "worker", "prompt": "do thing"},
			wantInput: "inbox",
		},
		{
			name:      "remote: empty incoming → preset default none",
			config:    map[string]any{"mode": "remote", "prompt": "run remotely"},
			wantInput: "none",
		},
		{
			name:      "teammate: empty incoming → preset default via_leader",
			config:    map[string]any{"mode": "teammate", "prompt": "be teammate"},
			wantInput: "via_leader",
		},
		{
			name:      "explicit incoming wins over preset default",
			config:    map[string]any{"mode": "worker", "userCommsInput": "none", "prompt": "shh"},
			wantInput: "none",
		},
		{
			name:      "explicit incoming wins over chat preset (operator was deliberate)",
			config:    map[string]any{"mode": "chat", "userCommsInput": "none", "prompt": "shh"},
			wantInput: "none",
		},
		{
			name:      "no mode → safe default inbox",
			config:    map[string]any{"prompt": "headless"},
			wantInput: "inbox",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := f.resolveFromExecRequest(&taskapi.TaskExecRequest{
				Config: tc.config,
			})
			if err != nil {
				t.Fatalf("resolveFromExecRequest: %v", err)
			}
			if req.UserCommsInput != tc.wantInput {
				t.Errorf("UserCommsInput = %q, want %q", req.UserCommsInput, tc.wantInput)
			}
		})
	}
}

// TestResolveFromExecRequest_UserCommsInput_DirectAPIPath confirms
// the *AgentRunRequest direct-API path takes the request as-is (no
// resolveFromExecRequest), so empty UserCommsInput stays empty.
// This pins the contract that programmatic callers MUST pre-compute
// via presets.EffectiveUserCommsInput when calling
// CreateMachine(*AgentRunRequest{...}) directly.
func TestResolveFromExecRequest_UserCommsInput_DirectAPIPath(t *testing.T) {
	f := NewAgentMachineFactory(nil, nil)
	got, err := f.resolveRequest(&api.AgentRunRequest{
		Mode: "chat",
		// UserCommsInput intentionally empty
	})
	if err != nil {
		t.Fatalf("resolveRequest: %v", err)
	}
	// Direct-API path passes through verbatim — no auto-detection
	// applied.
	if got.UserCommsInput != "" {
		t.Errorf("direct-API path should pass through verbatim; got %q, want empty",
			got.UserCommsInput)
	}
}

var _ = context.Background // silence unused-import linter on bare-test files
