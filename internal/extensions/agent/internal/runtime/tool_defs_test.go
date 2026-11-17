// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// buildToolDefs is the hand-off to the model: the union of every plugin's
// tool definitions, one per name, first plugin wins, in plugin order. It had
// no test until 2026-09-22 — a plugin shadowing another's tool was invisible.
func TestBuildToolDefs_UnionFirstPluginWinsInOrder(t *testing.T) {
	set := NewPluginSet()
	set.Add(&nonWrapperPlugin{name: "first", defs: []agentapi.ToolDef{{Name: "file-read", Description: "first's"}, {Name: "complete"}}})
	set.Add(&nonWrapperPlugin{name: "second", defs: []agentapi.ToolDef{{Name: "file-read", Description: "second's"}, {Name: "web-fetch"}}})
	m := &AgentLoopMachine{plugins: set}

	defs := m.buildToolDefs()
	got := make([]string, 0, len(defs))
	for _, d := range defs {
		got = append(got, d.Name)
	}
	if len(got) != 3 || got[0] != "file-read" || got[1] != "complete" || got[2] != "web-fetch" {
		t.Fatalf("tool list = %v, want [file-read complete web-fetch] (union, first wins, plugin order)", got)
	}
	if defs[0].Description != "first's" {
		t.Errorf("the first plugin's definition must win, got %q", defs[0].Description)
	}
	first := m.lastToolsHash
	m.buildToolDefs()
	if m.lastToolsHash != first {
		t.Error("the tools hash must be stable for an unchanged catalog (prompt-cache key)")
	}
	set.Add(&nonWrapperPlugin{name: "third", defs: []agentapi.ToolDef{{Name: "glob"}}})
	m.buildToolDefs()
	if m.lastToolsHash == first {
		t.Error("the tools hash must move when a tool is added")
	}
}
