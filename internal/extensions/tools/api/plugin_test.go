// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// --- NewToolPluginFactory ---

func TestNewToolPluginFactory_NamedAfterTool(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	factory := NewToolPluginFactory(tool)
	if factory.Name() != "sample" {
		t.Errorf("factory.Name() = %q, want %q", factory.Name(), "sample")
	}
}

func TestNewToolPluginFactory_CreatesSingleToolPlugin(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	factory := NewToolPluginFactory(tool)

	plugins, err := factory.CreatePlugins(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("CreatePlugins failed: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	plugin := plugins[0]
	if plugin.Name() != "sample" {
		t.Errorf("plugin name = %q, want %q", plugin.Name(), "sample")
	}
	defs := plugin.ToolDefs()
	if len(defs) != 1 {
		t.Fatalf("expected 1 tool def, got %d", len(defs))
	}
	if defs[0].Name != "sample" {
		t.Errorf("tool def name = %q, want %q", defs[0].Name, "sample")
	}
}

func TestNewToolPluginFactory_HandleToolCallRoundtrip(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	factory := NewToolPluginFactory(tool)
	plugins, _ := factory.CreatePlugins(context.Background(), nil, nil)

	res, err := plugins[0].HandleToolCall(context.Background(), "sample", json.RawMessage(`{"path":"x"}`))
	if err != nil {
		t.Fatalf("HandleToolCall failed: %v", err)
	}
	out, ok := res.Data.(sampleOutput)
	if !ok || !out.OK {
		t.Errorf("unexpected result: %+v", res)
	}
}

// --- NewToolGroupPlugin ---

func TestNewToolGroupPlugin_AdvertisesAllTools(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "Language server.", tools)

	if plugin.Name() != "lsp" {
		t.Errorf("plugin.Name() = %q, want %q", plugin.Name(), "lsp")
	}
	if plugin.SystemMessage() != "Language server." {
		t.Errorf("plugin.SystemMessage() = %q", plugin.SystemMessage())
	}
	defs := plugin.ToolDefs()
	if len(defs) != 3 {
		t.Fatalf("expected 3 tool defs, got %d", len(defs))
	}
}

func TestNewToolGroupPlugin_RoutesByName(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "", tools)

	res, err := plugin.HandleToolCall(context.Background(), "lsp-goto-definition",
		json.RawMessage(`{"file":"main.go","line":10}`))
	if err != nil {
		t.Fatalf("HandleToolCall failed: %v", err)
	}
	out, ok := res.Data.(gotoDefOutput)
	if !ok {
		t.Fatalf("Data should be gotoDefOutput, got %T", res.Data)
	}
	if out.File != "main.go" || out.Line != 10 || !out.Found {
		t.Errorf("unexpected output: %+v", out)
	}
}

func TestNewToolGroupPlugin_UnknownToolErrors(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "", tools)

	_, err := plugin.HandleToolCall(context.Background(), "lsp-does-not-exist", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error should mention 'unknown tool', got %q", err.Error())
	}
}

func TestNewToolGroupPlugin_ResolvesAliases(t *testing.T) {
	// A function tool with an alias, wrapped via NewToolGroupPlugin (using a
	// single-tool slice — same routing path as a real group).
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler,
		WithReadOnly(),
		WithAliases("legacy_name"),
	)
	plugin := NewToolGroupPlugin("group", "", []Tool{tool})

	// Canonical name works.
	if _, err := plugin.HandleToolCall(context.Background(), "sample", json.RawMessage(`{"path":"x"}`)); err != nil {
		t.Fatalf("canonical name failed: %v", err)
	}
	// Alias works.
	if _, err := plugin.HandleToolCall(context.Background(), "legacy_name", json.RawMessage(`{"path":"x"}`)); err != nil {
		t.Fatalf("alias name failed: %v", err)
	}
}

func TestNewToolGroupPlugin_StableTrue(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "", tools)
	if !plugin.Stable() {
		t.Error("group plugin should be Stable() = true")
	}
}

func TestNewToolGroupPlugin_HooksEmpty(t *testing.T) {
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "", tools)
	hooks := plugin.Hooks()
	if hooks.OnTurnStart != nil || hooks.OnPostToolExec != nil {
		t.Error("group plugin should have no hooks by default")
	}
}

// --- ToolExtension (existing helper still works with new Tool interface) ---

func TestToolExtension_ProducesValidStrategy(t *testing.T) {
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	def := ToolExtension(tool)

	if def.Name != "sample" {
		t.Errorf("strategy name = %q, want %q", def.Name, "sample")
	}
	if def.Strategy == nil {
		t.Fatal("strategy is nil")
	}
}

// --- ToolsFromPlugin (per-tool permission gate plumbing) ---

func TestToolsFromPlugin_FunctionWrapper(t *testing.T) {
	// A toolPlugin should expose its single wrapped Tool.
	tool := NewFunctionTool[sampleInput, sampleOutput]("sample", "test", sampleHandler, WithReadOnly())
	plugin := NewToolPlugin(tool)

	got := ToolsFromPlugin(plugin)
	if len(got) != 1 {
		t.Fatalf("ToolsFromPlugin returned %d tools, want 1", len(got))
	}
	if got[0].ToolDef().Name != "sample" {
		t.Errorf("returned tool name = %q, want %q", got[0].ToolDef().Name, "sample")
	}
}

func TestToolsFromPlugin_GroupWrapper(t *testing.T) {
	// A toolGroupPlugin should expose every wrapped Tool. fakeLSP has 3 methods.
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "", tools)

	got := ToolsFromPlugin(plugin)
	if len(got) != 3 {
		t.Fatalf("ToolsFromPlugin returned %d tools, want 3", len(got))
	}

	// Each returned tool should be discoverable by name (sanity: they
	// match what HandleToolCall would route).
	names := make(map[string]bool, len(got))
	for _, tt := range got {
		names[tt.ToolDef().Name] = true
	}
	if !names["lsp-goto-definition"] {
		t.Errorf("expected lsp-goto-definition in returned tools, got %v", names)
	}
}

func TestToolsFromPlugin_DefensiveCopy(t *testing.T) {
	// Group wrapper returns a defensive copy — mutating the returned
	// slice must not affect future calls.
	tools := NewToolGroup[fakeLSP]("lsp", "LSP.", &fakeLSPImpl{}, WithReadOnly())
	plugin := NewToolGroupPlugin("lsp", "", tools)

	first := ToolsFromPlugin(plugin)
	first[0] = nil // mutate the caller's copy

	second := ToolsFromPlugin(plugin)
	if second[0] == nil {
		t.Error("second call returned a slice affected by mutation of the first; defensive copy missing")
	}
}

func TestToolsFromPlugin_NonWrapperPlugin_ReturnsNil(t *testing.T) {
	// Plugins that are NOT toolPlugin / toolGroupPlugin must return
	// nil — the kernel falls back to fail-closed defaults for them.
	got := ToolsFromPlugin(&notAWrapperPlugin{})
	if got != nil {
		t.Errorf("ToolsFromPlugin on a non-wrapper plugin = %v, want nil", got)
	}
}

// notAWrapperPlugin is a minimal AgentPlugin that does NOT implement
// the unexported toolHolder interface. Used to verify the type
// assertion in ToolsFromPlugin fails closed.
type notAWrapperPlugin struct{}

func (*notAWrapperPlugin) Name() string                 { return "not-a-wrapper" }
func (*notAWrapperPlugin) SystemMessage() string        { return "" }
func (*notAWrapperPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (*notAWrapperPlugin) Hooks() agentapi.PluginHooks  { return agentapi.PluginHooks{} }
func (*notAWrapperPlugin) Stable() bool                 { return true }
func (*notAWrapperPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, nil
}
