// SPDX-License-Identifier: Apache-2.0

package plugin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	plugin "github.com/katyoti/katyoti-fwk/internal/extensions/agent/internal/plugin"
)

// --- Test helpers ---

var emptyLoop agentapi.LoopContext

type testPlugin struct {
	name string
}

func (p testPlugin) Name() string                 { return p.name }
func (p testPlugin) SystemMessage() string        { return "" }
func (p testPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p testPlugin) HandleToolCall(ctx context.Context, name string, input json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("not implemented")
}
func (p testPlugin) Hooks() agentapi.PluginHooks { return agentapi.PluginHooks{} }
func (p testPlugin) Stable() bool                { return true }

// --- Test strategy ---

type testStrategy struct {
	name      string
	factories map[string]agentapi.PluginFactory
	requires  map[string][]agentapi.PluginRef // per-ref requirements
}

func (s *testStrategy) Name() string { return s.name }

func (s *testStrategy) GetRequirements(ref agentapi.PluginRef) ([]agentapi.PluginRef, error) {
	if s.requires != nil {
		return s.requires[ref.Name], nil
	}
	return nil, nil
}

func (s *testStrategy) ResolveAll(refs []agentapi.PluginRef) ([]agentapi.PluginFactory, error) {
	var factories []agentapi.PluginFactory
	for _, ref := range refs {
		f, ok := s.factories[ref.Name]
		if !ok {
			return nil, fmt.Errorf("unknown %s ref: %q", s.name, ref.Name)
		}
		factories = append(factories, f)
	}
	return factories, nil
}

func makeTestStrategy(name string, pluginNames ...string) *testStrategy {
	factories := make(map[string]agentapi.PluginFactory)
	for _, pn := range pluginNames {
		pn := pn
		factories[pn] = &agentapi.FuncFactory{
			FactoryName: name + ":" + pn,
			CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
				return []agentapi.AgentPlugin{&testPlugin{name: name + ":" + pn}}, nil
			},
		}
	}
	return &testStrategy{name: name, factories: factories}
}

// --- Tests ---

func TestResolver_BasicRef(t *testing.T) {
	skill := makeTestStrategy("skill", "doc-create")
	resolver := plugin.NewPluginResolver([]agentapi.PluginStrategy{skill}, nil)

	resources := []agentapi.PluginRef{agentapi.ParsePluginRef("skill:doc-create")}
	plugins, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	if plugins[0].Name() != "skill:doc-create" {
		t.Errorf("expected name 'skill:doc-create', got %q", plugins[0].Name())
	}
}

func TestResolver_Dedup(t *testing.T) {
	skill := makeTestStrategy("skill", "doc-create")
	resolver := plugin.NewPluginResolver([]agentapi.PluginStrategy{skill}, nil)

	resources := []agentapi.PluginRef{
		agentapi.ParsePluginRef("skill:doc-create"),
		agentapi.ParsePluginRef("skill:doc-create"),
	}
	plugins, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 {
		t.Errorf("expected 1 plugin (dedup), got %d", len(plugins))
	}
}

func TestResolver_UnknownStrategy(t *testing.T) {
	resolver := plugin.NewPluginResolver(nil, nil)

	resources := []agentapi.PluginRef{agentapi.ParsePluginRef("unknown:foo")}
	_, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestResolver_FallbackLegacy(t *testing.T) {
	fallback := func(name string) (agentapi.PluginFactory, bool) {
		if name == "workspace" {
			return &agentapi.FuncFactory{
				FactoryName: "workspace",
				CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
					return []agentapi.AgentPlugin{&testPlugin{name: "workspace"}}, nil
				},
			}, true
		}
		return nil, false
	}
	resolver := plugin.NewPluginResolver(nil, fallback)

	resources := []agentapi.PluginRef{agentapi.ParsePluginRef("workspace")}
	plugins, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name() != "workspace" {
		t.Errorf("expected workspace plugin, got %v", plugins)
	}
}

func TestResolver_MultipleStrategies(t *testing.T) {
	skill := makeTestStrategy("skill", "doc-create", "code-review")
	task := makeTestStrategy("task", "generate-report")
	resolver := plugin.NewPluginResolver([]agentapi.PluginStrategy{skill, task}, nil)

	resources := []agentapi.PluginRef{
		agentapi.ParsePluginRef("skill:doc-create"),
		agentapi.ParsePluginRef("task:generate-report"),
		agentapi.ParsePluginRef("skill:code-review"),
	}
	plugins, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 3 {
		t.Fatalf("expected 3 plugins, got %d", len(plugins))
	}
}

func TestResolver_Requirements(t *testing.T) {
	skill := &testStrategy{
		name: "skill",
		factories: map[string]agentapi.PluginFactory{
			"doc-create": &agentapi.FuncFactory{
				FactoryName: "skill:doc-create",
				CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
					return []agentapi.AgentPlugin{&testPlugin{name: "skill:doc-create"}}, nil
				},
			},
			"template-engine": &agentapi.FuncFactory{
				FactoryName: "skill:template-engine",
				CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
					return []agentapi.AgentPlugin{&testPlugin{name: "skill:template-engine"}}, nil
				},
			},
		},
		requires: map[string][]agentapi.PluginRef{
			"doc-create": {agentapi.ParsePluginRef("skill:template-engine")},
		},
	}

	resolver := plugin.NewPluginResolver([]agentapi.PluginStrategy{skill}, nil)

	// Only request doc-create — template-engine should be pulled in via requirements
	resources := []agentapi.PluginRef{agentapi.ParsePluginRef("skill:doc-create")}
	plugins, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins (doc-create + template-engine), got %d", len(plugins))
	}
}

func TestResolver_RequirementsCrossStrategy(t *testing.T) {
	workspace := makeTestStrategy("workspace")
	workspace.factories[""] = &agentapi.FuncFactory{
		FactoryName: "workspace",
		CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
			return []agentapi.AgentPlugin{&testPlugin{name: "workspace"}}, nil
		},
	}

	skill := &testStrategy{
		name: "skill",
		factories: map[string]agentapi.PluginFactory{
			"doc-create": &agentapi.FuncFactory{
				FactoryName: "skill:doc-create",
				CreateFunc: func(_ context.Context, _ agentapi.LoopContext, _ []agentapi.PluginRef) ([]agentapi.AgentPlugin, error) {
					return []agentapi.AgentPlugin{&testPlugin{name: "skill:doc-create"}}, nil
				},
			},
		},
		requires: map[string][]agentapi.PluginRef{
			"doc-create": {agentapi.ParsePluginRef("workspace")}, // cross-strategy requirement
		},
	}

	resolver := plugin.NewPluginResolver([]agentapi.PluginStrategy{skill, workspace}, nil)
	resources := []agentapi.PluginRef{agentapi.ParsePluginRef("skill:doc-create")}
	plugins, err := resolver.Resolve(resources, context.Background(), emptyLoop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(plugins))
	}
}

func TestMergeResources_Override(t *testing.T) {
	defaults := []agentapi.PluginRef{
		agentapi.ParsePluginRef("skill:a"),
		agentapi.ParsePluginRef("skill:b"),
	}
	task := []agentapi.PluginRef{
		agentapi.ParsePluginRef("skill:b"), // override
		agentapi.ParsePluginRef("skill:c"), // new
	}

	merged := plugin.MergeResources(defaults, task)
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged resources, got %d", len(merged))
	}
	refs := make([]string, len(merged))
	for i, r := range merged {
		refs[i] = r.Ref()
	}
	expected := []string{"skill:a", "skill:b", "skill:c"}
	for i, e := range expected {
		if refs[i] != e {
			t.Errorf("merged[%d] = %q, want %q", i, refs[i], e)
		}
	}
}

func TestParsePluginRef(t *testing.T) {
	tests := []struct {
		input string
		typ   string
		name  string
	}{
		{"skill:doc-create", "skill", "doc-create"},
		{"task:ktw-docs:render", "task", "ktw-docs:render"},
		{"workspace", "workspace", ""},
		{"group:base_plugins", "group", "base_plugins"},
	}
	for _, tt := range tests {
		r := agentapi.ParsePluginRef(tt.input)
		if r.Type != tt.typ {
			t.Errorf("ParsePluginRef(%q).Strategy = %q, want %q", tt.input, r.Type, tt.typ)
		}
		if r.Name != tt.name {
			t.Errorf("ParsePluginRef(%q).Pattern = %q, want %q", tt.input, r.Name, tt.name)
		}
	}
}
