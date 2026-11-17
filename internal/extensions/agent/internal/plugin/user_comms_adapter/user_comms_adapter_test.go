// SPDX-License-Identifier: Apache-2.0

package user_comms_adapter

import (
	"strings"
	"testing"
)

// TestNewPlugin_NilForEmptyInput pins the auto-attach guard — the
// factory calls NewPlugin unconditionally and skips attach when nil
// comes back. Empty input must return nil so legacy programmatic
// runs (no resolver) don't get a misleading channel block.
func TestNewPlugin_NilForEmptyInput(t *testing.T) {
	if NewPlugin("") != nil {
		t.Error("empty input should produce no plugin (nil)")
	}
	if NewPlugin("nonexistent-mode") != nil {
		t.Error("unrecognized input should produce no plugin (nil)")
	}
}

// TestNewPlugin_FiveChannels pins the five recognized values + the
// prompt vocabulary the agent will see. Each channel block must
// mention the channel's defining property so the LLM gets useful
// guidance — not a generic "you have a user channel" placeholder.
func TestNewPlugin_FiveChannels(t *testing.T) {
	cases := []struct {
		input       string
		mustContain []string // every substring must appear in the prompt
		mustExclude []string // none of these may appear (anti-cross-contamination)
	}{
		{
			input:       "direct",
			mustContain: []string{"REAL TIME", "ask-user"},
			mustExclude: []string{"Inbox", "leader", "headless"},
		},
		{
			input:       "inbox",
			mustContain: []string{"HEADLESS", "Inbox", "approval-request"},
			mustExclude: []string{"REAL TIME", "leader"},
		},
		{
			input:       "bubble_to_parent",
			mustContain: []string{"CHILD", "parent"},
			mustExclude: []string{"leader", "Inbox", "REAL TIME"},
		},
		{
			input:       "via_leader",
			mustContain: []string{"TEAMMATE", "leader", "team"},
			mustExclude: []string{"parent", "Inbox", "REAL TIME"},
		},
		{
			input:       "none",
			mustContain: []string{"NO user channel", "fail"},
			mustExclude: []string{"REAL TIME", "Inbox", "leader", "parent"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			p := NewPlugin(tc.input)
			if p == nil {
				t.Fatalf("NewPlugin(%q) returned nil", tc.input)
			}
			sm := p.SystemMessage()
			if sm == "" {
				t.Fatalf("SystemMessage() is empty for %q", tc.input)
			}
			if !strings.HasPrefix(sm, "<user_channel>") || !strings.HasSuffix(sm, "</user_channel>") {
				t.Errorf("SystemMessage should be wrapped in <user_channel> tags; got %q", sm[:min(60, len(sm))])
			}
			for _, want := range tc.mustContain {
				if !strings.Contains(sm, want) {
					t.Errorf("prompt for %q missing required phrase %q", tc.input, want)
				}
			}
			for _, forbid := range tc.mustExclude {
				if strings.Contains(sm, forbid) {
					t.Errorf("prompt for %q must NOT mention %q (cross-channel contamination)", tc.input, forbid)
				}
			}
		})
	}
}

// TestPlugin_ImplementsAgentPluginContract confirms the plugin
// satisfies the AgentPlugin interface without owning tools or hooks
// in Phase B (Phase C will swap ask_user / notify_user; until then
// it's prompt-only).
func TestPlugin_ImplementsAgentPluginContract(t *testing.T) {
	p := NewPlugin("direct")
	if p.Name() != "user-comms-adapter" {
		t.Errorf("Name = %q, want user-comms-adapter", p.Name())
	}
	if len(p.ToolDefs()) != 0 {
		t.Errorf("Phase B plugin must not own tools; ToolDefs returned %d", len(p.ToolDefs()))
	}
	if !p.Stable() {
		t.Error("Plugin should be Stable (cache-friendly prefix)")
	}
	hooks := p.Hooks()
	if hooks.OnDecision != nil || hooks.OnPreToolExec != nil || hooks.OnTurnStart != nil {
		t.Error("Phase B plugin must not register any hooks; defer hook wiring to Phase C")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
