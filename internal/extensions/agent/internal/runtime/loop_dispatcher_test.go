// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// ── Synthetic tool helpers ──────────────────────────────────────────────
//
// These helpers are also used by the permission-gate tests in
// agent_machine_permission_test.go. The synthetic tool implements the
// full toolsapi.Tool interface with hooks for both ToolCall behavior
// and CheckPermissions behavior so each test can drive a precise
// scenario without spinning up a real handler.

type permInput struct {
	Echo string `json:"echo,omitempty"`
}

type permOutput struct {
	Got string `json:"got"`
}

// permissionTestTool is a hand-rolled toolsapi.Tool used by both the
// dispatcher tests below and the permission-gate tests in
// agent_machine_permission_test.go. The PermissionResult and any
// ToolCall side-effect (e.g. capturing the input that arrived) are
// fully configurable per call.
type permissionTestTool struct {
	name      string
	aliases   []string
	pres      toolsapi.PermissionResult
	gotInput  json.RawMessage // captured by ToolCall — used to assert ModifiedInput substitution
	callCount int
}

func newPermTool(name string, pres toolsapi.PermissionResult, aliases ...string) *permissionTestTool {
	return &permissionTestTool{name: name, aliases: aliases, pres: pres}
}

func (t *permissionTestTool) ToolDef() agentapi.ToolDef {
	return agentapi.ToolDef{
		Name:        t.name,
		Description: "synthetic test tool",
		// InputSchema empty — the synthetic tool doesn't validate input.
	}
}
func (t *permissionTestTool) Aliases() []string                 { return t.aliases }
func (t *permissionTestTool) IsEnabled() bool                   { return true }
func (t *permissionTestTool) Describe(_ json.RawMessage) string { return t.name }
func (t *permissionTestTool) SearchHint() string                { return "" }
func (t *permissionTestTool) IsReadOnly() bool                  { return true }
func (t *permissionTestTool) IsConcurrencySafe() bool           { return true }
func (t *permissionTestTool) IsDestructive() bool               { return false }
func (t *permissionTestTool) InterruptBehavior() toolsapi.InterruptBehavior {
	return toolsapi.InterruptBlock
}
func (t *permissionTestTool) MaxResultSize() int { return 0 }
func (t *permissionTestTool) ShouldDefer() bool  { return false }
func (t *permissionTestTool) Prompt() string     { return "" }

func (t *permissionTestTool) ToolCall(_ context.Context, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	t.callCount++
	t.gotInput = append(json.RawMessage(nil), input...)
	return &agentapi.ToolCallResult{Data: permOutput{Got: string(input)}}, nil
}

func (t *permissionTestTool) CheckPermissions(_ context.Context, _ json.RawMessage) toolsapi.PermissionResult {
	return t.pres
}

// ── Dispatcher tests ────────────────────────────────────────────────────

func TestDispatcher_RegisterPlugin_IndexesWrappedTools(t *testing.T) {
	d := NewDispatcher()
	tool := newPermTool("synth_a", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow})
	plugin := toolsapi.NewToolPlugin(tool)

	d.RegisterPlugin(plugin)

	if got := d.LookupTool("synth_a"); got == nil {
		t.Errorf("LookupTool(%q) = nil, want the registered tool", "synth_a")
	}
}

func TestDispatcher_RegisterPlugin_IndexesAliases(t *testing.T) {
	d := NewDispatcher()
	tool := newPermTool("synth_canon", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}, "synth_alias_1", "synth_alias_2")
	plugin := toolsapi.NewToolPlugin(tool)

	d.RegisterPlugin(plugin)

	for _, name := range []string{"synth_canon", "synth_alias_1", "synth_alias_2"} {
		if got := d.LookupTool(name); got == nil {
			t.Errorf("LookupTool(%q) = nil, want the registered tool", name)
		}
	}
}

func TestDispatcher_LookupTool_ReturnsNilForUnknown(t *testing.T) {
	d := NewDispatcher()
	if got := d.LookupTool("nonexistent"); got != nil {
		t.Errorf("LookupTool(unknown) = %v, want nil", got)
	}
}

// nonWrapperPlugin is an AgentPlugin that does NOT come from
// NewToolPlugin / NewToolGroupPlugin. The dispatcher must register
// it for routing but MUST NOT index any Tools for the permission
// gate — LookupTool must return nil for the tools it advertises.
// This mirrors how MCP and sidecar plugins behave today.
type nonWrapperPlugin struct {
	name    string
	defs    []agentapi.ToolDef
	handler func(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error)
}

func (p *nonWrapperPlugin) Name() string                 { return p.name }
func (p *nonWrapperPlugin) SystemMessage() string        { return "" }
func (p *nonWrapperPlugin) ToolDefs() []agentapi.ToolDef { return p.defs }
func (p *nonWrapperPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (p *nonWrapperPlugin) Stable() bool                 { return true }
func (p *nonWrapperPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	if p.handler != nil {
		return p.handler(ctx, name, input)
	}
	return &agentapi.ToolCallResult{Data: nil}, nil
}

func TestDispatcher_NonWrapperPlugin_NoToolIndexed(t *testing.T) {
	d := NewDispatcher()
	plugin := &nonWrapperPlugin{
		name: "mcp-ish",
		defs: []agentapi.ToolDef{{Name: "remote_op", Description: "from MCP"}},
	}
	d.RegisterPlugin(plugin)

	// Plugin is reachable for routing.
	if !d.Has("remote_op") {
		t.Error("Has(remote_op) = false, want true (plugin should be routable)")
	}

	// But no Tool is indexed for the permission gate.
	if got := d.LookupTool("remote_op"); got != nil {
		t.Errorf("LookupTool(remote_op) on non-wrapper plugin = %v, want nil (no per-tool gate)", got)
	}
}

func TestDispatcher_GroupPlugin_IndexesAllMembers(t *testing.T) {
	// A group of synthetic tools wrapped via NewToolGroupPlugin should
	// expose every member to LookupTool.
	d := NewDispatcher()
	tools := []toolsapi.Tool{
		newPermTool("g_one", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}),
		newPermTool("g_two", toolsapi.PermissionResult{Behavior: toolsapi.PermissionDeny, Message: "no"}),
		newPermTool("g_three", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAsk, Message: "?"}),
	}
	plugin := toolsapi.NewToolGroupPlugin("g", "", tools)

	d.RegisterPlugin(plugin)

	for _, name := range []string{"g_one", "g_two", "g_three"} {
		if got := d.LookupTool(name); got == nil {
			t.Errorf("LookupTool(%q) = nil, want the group member", name)
		}
	}
}

func TestDispatcher_DispatchStillWorks(t *testing.T) {
	// Sanity: the existing Dispatch path is unchanged. A registered
	// tool wrapper should still route correctly through HandleToolCall.
	d := NewDispatcher()
	tool := newPermTool("d_route", toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow})
	d.RegisterPlugin(toolsapi.NewToolPlugin(tool))

	res, err := d.Dispatch(context.Background(), "d_route", json.RawMessage(`{"echo":"hi"}`))
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if res == nil {
		t.Fatal("Dispatch returned nil result")
	}
	if tool.callCount != 1 {
		t.Errorf("ToolCall called %d times, want 1", tool.callCount)
	}
}
