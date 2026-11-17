// SPDX-License-Identifier: Apache-2.0

package assembly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// fakePlugin stands in for resolver output. It optionally owns a
// `complete` tool so the chat wrapper's filtering is observable.
type fakePlugin struct {
	name        string
	hasComplete bool
}

func (p *fakePlugin) Name() string          { return p.name }
func (p *fakePlugin) SystemMessage() string { return "" }
func (p *fakePlugin) Stable() bool          { return true }
func (p *fakePlugin) ToolDefs() []api.ToolDef {
	defs := []api.ToolDef{{Name: p.name + "-tool"}}
	if p.hasComplete {
		defs = append(defs, api.ToolDef{Name: "complete"})
	}
	return defs
}
func (p *fakePlugin) HandleToolCall(_ context.Context, name string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("fake: no tool %s", name)
}
func (p *fakePlugin) Hooks() api.PluginHooks { return api.PluginHooks{} }

func names(plugins []api.AgentPlugin) []string {
	out := make([]string, len(plugins))
	for i, p := range plugins {
		out[i] = p.Name()
	}
	return out
}

func assertNames(t *testing.T, got []api.AgentPlugin, want ...string) {
	t.Helper()
	gotNames := names(got)
	if len(gotNames) != len(want) {
		t.Fatalf("plugin list = %v, want %v", gotNames, want)
	}
	for i := range want {
		if gotNames[i] != want[i] {
			t.Fatalf("plugin[%d] = %q, want %q (full: %v)", i, gotNames[i], want[i], gotNames)
		}
	}
}

func req(mode, comms string) *api.AgentRunRequest {
	r := &api.AgentRunRequest{Mode: mode, UserCommsInput: comms}
	// Assembly now keys the driven (keepalive + complete-hide) behavior off
	// Finish, not the mode name. The factory resolves chat → driven; these unit
	// tests bypass the factory, so mirror that resolution here.
	if mode == string(presets.ModeChat) {
		r.Finish = string(presets.FinishDriven)
	}
	return r
}

// TestBuild_PlainWorker pins the minimal composition: resolved plugins +
// the always-on built-ins + loop-safety. No chat, no comms, no trace,
// no fallback, no guards.
func TestBuild_PlainWorker(t *testing.T) {
	ps := Build(Inputs{
		Request:  req("worker", ""),
		Resolved: []api.AgentPlugin{&fakePlugin{name: "alpha"}, &fakePlugin{name: "beta"}},
	})
	assertNames(t, ps.Plugins(),
		"alpha", "beta",
		"output_recovery", "abort_cleanup",
		"loop-safety",
		// spawn_permissions attaches even for nil restrictions (the
		// hook handles the nil/AllowAll cases itself) — pre-assembly
		// behavior, preserved.
		"spawn_permissions",
	)
}

// TestBuild_KitchenSink pins the full composition order with every
// conditional active.
func TestBuild_KitchenSink(t *testing.T) {
	ps := Build(Inputs{
		Request: &api.AgentRunRequest{
			Mode:             "chat",
			Finish:           string(presets.FinishDriven),
			UserCommsInput:   "direct",
			SpawnPermissions: &api.SpawnPermissions{AllowedTools: []string{"glob"}},
			WorkspacePaths:   &api.WorkspacePaths{ReadPaths: []string{"src/"}},
		},
		Resolved:      []api.AgentPlugin{&fakePlugin{name: "alpha", hasComplete: true}},
		TraceWriter:   &bytes.Buffer{},
		FallbackModel: "claude-haiku",
	})
	assertNames(t, ps.Plugins(),
		"alpha", // chat-wrapped (name passes through)
		"chat-loop-keepalive",
		"user-comms-adapter",
		"output_recovery", "abort_cleanup",
		"debug",
		"model_fallback",
		"loop-safety",
		"spawn_permissions",
		"workspace_paths",
	)
}

// TestBuild_ChatFiltersComplete pins the chat wrapper's effect: the
// resolved plugin's `complete` tool disappears from the catalog while
// its other tools survive.
func TestBuild_ChatFiltersComplete(t *testing.T) {
	ps := Build(Inputs{
		Request:  req("chat", ""),
		Resolved: []api.AgentPlugin{&fakePlugin{name: "alpha", hasComplete: true}},
	})
	var tools []string
	for _, p := range ps.Plugins() {
		for _, d := range p.ToolDefs() {
			tools = append(tools, d.Name)
		}
	}
	for _, name := range tools {
		if name == "complete" {
			t.Fatalf("complete tool leaked through the chat filter: %v", tools)
		}
	}
	found := false
	for _, name := range tools {
		if name == "alpha-tool" {
			found = true
		}
	}
	if !found {
		t.Fatalf("non-complete tools must survive the wrapper: %v", tools)
	}
}

// TestBuild_NonChatKeepsComplete: outside chat mode nothing is wrapped
// and no keepalive attaches.
func TestBuild_NonChatKeepsComplete(t *testing.T) {
	ps := Build(Inputs{
		Request:  req("worker", ""),
		Resolved: []api.AgentPlugin{&fakePlugin{name: "alpha", hasComplete: true}},
	})
	for _, p := range ps.Plugins() {
		if p.Name() == "chat-loop-keepalive" {
			t.Fatal("keepalive must not attach outside chat mode")
		}
	}
	var hasComplete bool
	for _, p := range ps.Plugins() {
		for _, d := range p.ToolDefs() {
			if d.Name == "complete" {
				hasComplete = true
			}
		}
	}
	if !hasComplete {
		t.Fatal("complete tool must survive outside chat mode")
	}
}

// TestBuild_FinishDrivesBehaviorNotMode pins the decoupling: the keepalive +
// complete-hide behavior follows Finish, NOT the mode name. A non-chat mode run
// launched finish:driven (e.g. an agent_as_human chat child) gets keepalive +
// the complete filter; the same mode with finish:self does not.
func TestBuild_FinishDrivesBehaviorNotMode(t *testing.T) {
	driven := Build(Inputs{
		Request:  &api.AgentRunRequest{Mode: "background", Finish: string(presets.FinishDriven)},
		Resolved: []api.AgentPlugin{&fakePlugin{name: "alpha", hasComplete: true}},
	})
	var hasKeepalive, completeVisible bool
	for _, p := range driven.Plugins() {
		if p.Name() == "chat-loop-keepalive" {
			hasKeepalive = true
		}
		for _, d := range p.ToolDefs() {
			if d.Name == "complete" {
				completeVisible = true
			}
		}
	}
	if !hasKeepalive {
		t.Error("finish:driven must attach keepalive regardless of mode")
	}
	if completeVisible {
		t.Error("finish:driven must hide the complete tool")
	}

	self := Build(Inputs{
		Request:  &api.AgentRunRequest{Mode: "background", Finish: string(presets.FinishSelf)},
		Resolved: []api.AgentPlugin{&fakePlugin{name: "alpha", hasComplete: true}},
	})
	for _, p := range self.Plugins() {
		if p.Name() == "chat-loop-keepalive" {
			t.Error("finish:self must NOT attach keepalive")
		}
	}
}

// TestBuild_CommsAdapterCondition: the adapter attaches exactly when
// the request carries a recognized channel.
func TestBuild_CommsAdapterCondition(t *testing.T) {
	has := func(ps []api.AgentPlugin, name string) bool {
		for _, p := range ps {
			if p.Name() == name {
				return true
			}
		}
		return false
	}
	with := Build(Inputs{Request: req("worker", "inbox"), Resolved: nil})
	if !has(with.Plugins(), "user-comms-adapter") {
		t.Error("adapter missing for inbox channel")
	}
	without := Build(Inputs{Request: req("worker", ""), Resolved: nil})
	if has(without.Plugins(), "user-comms-adapter") {
		t.Error("adapter attached without a channel")
	}
}

// TestBuild_LoopSafetyReadsLiveAttrs: the attrs lookup wired into
// loop-safety sees the assembled set's tool attributes.
func TestBuild_LoopSafetyReadsLiveAttrs(t *testing.T) {
	idleTool := &fakeIdleToolPlugin{}
	ps := Build(Inputs{Request: req("worker", ""), Resolved: []api.AgentPlugin{idleTool}})
	// Find loop-safety and probe it indirectly: 3 identical calls on the
	// POLLING tool must NOT trip (threshold 10), proving the lookup
	// resolved the attribute from the live set.
	var safety api.AgentPlugin
	for _, p := range ps.Plugins() {
		if p.Name() == "loop-safety" {
			safety = p
		}
	}
	if safety == nil {
		t.Fatal("loop-safety not assembled")
	}
	hook := safety.Hooks().OnPreToolExec
	if hook == nil {
		t.Fatal("loop-safety has no OnPreToolExec hook")
	}
	input := map[string]any{"id": "x"}
	for i := 0; i < 5; i++ {
		td, err := hook(nil, "poller", input)
		if err != nil {
			t.Fatalf("hook error: %v", err)
		}
		if td.Decision == api.Deny {
			t.Fatalf("polling tool tripped at call %d — live attrs lookup broken", i+1)
		}
	}
}

type fakeIdleToolPlugin struct{}

func (p *fakeIdleToolPlugin) Name() string          { return "poller-owner" }
func (p *fakeIdleToolPlugin) SystemMessage() string { return "" }
func (p *fakeIdleToolPlugin) Stable() bool          { return true }
func (p *fakeIdleToolPlugin) ToolDefs() []api.ToolDef {
	return []api.ToolDef{{Name: "poller", Polling: true}}
}
func (p *fakeIdleToolPlugin) HandleToolCall(_ context.Context, name string, _ json.RawMessage) (*api.ToolCallResult, error) {
	return nil, fmt.Errorf("fake: no tool %s", name)
}
func (p *fakeIdleToolPlugin) Hooks() api.PluginHooks { return api.PluginHooks{} }
