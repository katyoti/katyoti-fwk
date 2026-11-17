// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api/permrules"
	"github.com/katyoti/katyoti-fwk/internal/extensions/workspace"
)

// The file tools an agent actually gets come from the `workspace` composite
// plugin, not from one NewToolPlugin per tool. The gate must see them: a
// profile that denies file-write (the shipped plan-mode profile does) blocks
// a file-write call made through that plugin. When the composite did not
// expose its tools, LookupTool returned nil and the gate let every file
// write through, profile or not.
func TestPermissionGate_WorkspacePluginFileWrite_ProfileDenies(t *testing.T) {
	profile := &permrules.Profile{
		Name:      "plan-mode",
		DenyRules: []permrules.Rule{permrules.MustParseRule("file-write:**")},
	}
	ctx := newAppCtxWithGateAndProfiles(t, nil, &fakeProfiles{profile: profile})

	_, create := workspace.StrategyDef.Strategy.NewTypeConfig()
	factory, err := create(ctx)
	if err != nil {
		t.Fatalf("workspace factory: %v", err)
	}
	plugins, err := factory.CreatePlugins(ctx, nil, []agentapi.PluginRef{})
	if err != nil || len(plugins) == 0 {
		t.Fatalf("workspace plugins = %v, %v", plugins, err)
	}
	d := NewDispatcher()
	for _, p := range plugins {
		d.RegisterPlugin(p)
	}

	for _, name := range []string{"file-read", "file-write", "file-edit", "glob", "grep"} {
		if d.LookupTool(name) == nil {
			t.Errorf("the gate cannot see the workspace plugin's %s tool", name)
		}
	}

	in, _ := json.Marshal(map[string]string{"workspace_path": "notes/plan.md", "content": "x"})
	out := applyPermissionGate(ctx, d, "file-write", json.RawMessage(in))
	if out.allow {
		t.Fatal("a file-write through the workspace plugin passed a profile that denies file-write")
	}
	if !strings.Contains(out.errMsg, "plan-mode") {
		t.Errorf("errMsg = %q, want the denying profile named", out.errMsg)
	}
}

// bareToolPlugin advertises a tool without exposing a typed Tool — the shape
// of an MCP or hand-built plugin.
type bareToolPlugin struct{ name string }

func (p bareToolPlugin) Name() string                 { return "bare" }
func (p bareToolPlugin) SystemMessage() string        { return "" }
func (p bareToolPlugin) ToolDefs() []agentapi.ToolDef { return []agentapi.ToolDef{{Name: p.name}} }
func (p bareToolPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p bareToolPlugin) Stable() bool                 { return true }
func (p bareToolPlugin) HandleToolCall(context.Context, string, json.RawMessage) (*agentapi.ToolCallResult, error) {
	return agentapi.StringResult("ran"), nil
}

// A profile's deny rule names a tool, not the way its plugin was built: a
// tool the gate has no typed Tool for is still blocked by a deny on its name.
func TestPermissionGate_UnwrappedTool_ProfileDenyStillHolds(t *testing.T) {
	profile := &permrules.Profile{
		Name:      "locked",
		DenyRules: []permrules.Rule{permrules.MustParseRule("scripts-run:**")},
	}
	ctx := newAppCtxWithGateAndProfiles(t, nil, &fakeProfiles{profile: profile})
	d := NewDispatcher()
	d.RegisterPlugin(bareToolPlugin{name: "scripts-run"})
	d.RegisterPlugin(bareToolPlugin{name: "scripts-list"})

	if out := applyPermissionGate(ctx, d, "scripts-run", json.RawMessage(`{"name":"x"}`)); out.allow {
		t.Fatal("an unwrapped tool passed a profile that denies it")
	}
	if out := applyPermissionGate(ctx, d, "scripts-list", json.RawMessage(`{}`)); !out.allow {
		t.Fatalf("an unwrapped tool no rule names was blocked: %s", out.errMsg)
	}
}
