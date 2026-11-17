// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A CHAT STARTED ON AN OBJECT receives what AgentContext resolves — proven
// through the factory the agent loop calls, not only through the service.

func chatWorld(t *testing.T) context.Context {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"capabilities/content.yaml":   {Data: []byte("name: content\n")},
			"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/goal.yaml": {Data: []byte(`name: goal
capabilities: [container, content]
agent:
  system:
    - {id: how, title: Goals, text: "A goal of ${current.module} is reached, never done by hand."}
`)},
		}),
		ExtensionWith(api.ObjectContextConfig{Agent: &cwmapi.AgentDecl{System: []cwmapi.AgentSection{{ID: "framing", Text: "This workspace builds billing."}}}}),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	g := cwmapi.Object{ID: "billing.g1", Type: "goal"}
	g.Name, g.Parent = "Invoices go out on time", "billing"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: g}); err != nil {
		t.Fatal(err)
	}
	return ctx
}

func systemOf(plugins []agentapi.AgentPlugin) string {
	var parts []string
	for _, p := range plugins {
		parts = append(parts, p.SystemMessage())
	}
	return strings.Join(parts, "\n")
}

func TestAChatOnAnObjectReceivesItsContext(t *testing.T) {
	ctx := chatWorld(t)
	plugins, err := create(agentapi.WithPluginContext(ctx, map[string]any{agentapi.PluginContextTarget: "billing.g1"}), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	sys := systemOf(plugins)
	framing := strings.Index(sys, "This workspace builds billing.")
	goals := strings.Index(sys, "## Goals\nA goal of billing is reached, never done by hand.")
	current := strings.Index(sys, "## Current object\nYou are working on this object; read and change it through the cwm object tools.\nid: billing.g1\ntype: goal\nmodule: billing\nname: Invoices go out on time")
	if !strings.Contains(sys, `<cwm_object_context object="billing.g1">`) || framing < 0 || goals < 0 || current < 0 {
		t.Fatalf("system message lacks the object context:\n%s", sys)
	}
	if !(framing < goals && goals < current) {
		t.Errorf("sections out of order (project %d, type %d, object %d):\n%s", framing, goals, current, sys)
	}
	if !strings.Contains(sys, pluginSystemMessage) {
		t.Errorf("the object-effective tool group is gone:\n%s", sys)
	}
}

// No target, or one the service cannot resolve: the tool group alone, no
// invented context — a chat is still a chat.
func TestWithoutAResolvableObjectItIsTheToolGroup(t *testing.T) {
	ctx := chatWorld(t)
	for _, pc := range []map[string]any{nil, {agentapi.PluginContextTarget: "billing.ghost"}} {
		plugins, _ := create(agentapi.WithPluginContext(ctx, pc), nil, nil)
		if len(plugins) != 1 || strings.Contains(systemOf(plugins), "cwm_object_context") {
			t.Fatalf("%v: plugins = %d, system:\n%s", pc, len(plugins), systemOf(plugins))
		}
	}
}

// Tasks become typed tools here; skills go in one skill ref, plugins in their
// own refs, every other URI in one tool-proxy ref.
func TestTheToolsSplitIntoTasksAndRefs(t *testing.T) {
	tasks, refs := refsOf(api.AgentContextOutput{
		Skills: []api.Skill{{Selector: "goal-*"}, {Selector: "ktw-methods:/skill/*"}},
		Tools: []api.Tool{
			{URI: "ktw-methods:tasks/refine"}, {URI: "idea-curator-promote"},
			{URI: "ktw-methods:goals/add"}, {Plugin: "workspace"}, {URI: "ktw-example:example/search"},
		},
	})
	if strings.Join(tasks, " ") != "ktw-methods:tasks/refine idea-curator-promote" {
		t.Errorf("tasks = %v", tasks)
	}
	if len(refs) != 3 || refs[0].Type != "skill" || refs[1].Type != "workspace" || refs[2].Type != "tool" {
		t.Fatalf("refs = %+v", refs)
	}
	if got := refs[2].Props["tools"].([]any); len(got) != 2 || got[0] != "ktw-methods:goals/add" || got[1] != "ktw-example:example/search" {
		t.Errorf("tool-proxy uris = %v", got)
	}
}

// A trimmed section is not silently absent — the agent is told which ids it
// is working without.
func TestTrimmedSectionsAreNamedToTheAgent(t *testing.T) {
	block := render(api.AgentContextOutput{
		ObjectID: "m.x",
		System:   []api.Section{{ID: "a", Text: "kept"}},
		Notes:    []api.Note{{List: "system", Key: "framing", Kind: api.NoteTrimmed}, {List: "system", Key: "off", Kind: api.NoteSkipped}},
	})
	if !strings.Contains(block, "(Left out for size: framing.)") || strings.Contains(block, "off") {
		t.Fatalf("block:\n%s", block)
	}
}
