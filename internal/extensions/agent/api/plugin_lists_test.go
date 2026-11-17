// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

func TestDesugarPluginLists_OneRefPerListWithProps(t *testing.T) {
	r := &AgentRunRequest{
		Plugins: []PluginRef{{Type: "standard"}},
		Skills:  []string{"render-data-as-ui", " cwm-* ", ""},
		Tools:   []string{"ktw-weather:weather-service/*"},
	}
	r.DesugarPluginLists()

	if r.Skills != nil || r.Tools != nil {
		t.Fatalf("sugar lists not cleared: skills=%v tools=%v", r.Skills, r.Tools)
	}
	// standard + one skill ref + one tool ref (NOT N skill refs).
	if len(r.Plugins) != 3 {
		t.Fatalf("plugins = %d, want 3 (standard + skill + tool)", len(r.Plugins))
	}

	var skillRef, toolRef *PluginRef
	for i := range r.Plugins {
		switch r.Plugins[i].Type {
		case "skill":
			skillRef = &r.Plugins[i]
		case "tool":
			toolRef = &r.Plugins[i]
		}
	}
	if skillRef == nil || toolRef == nil {
		t.Fatalf("missing skill/tool ref: %+v", r.Plugins)
	}

	// The skills list is in Props, trimmed + empty-dropped, provider `:` intact.
	got := StringList(skillRef.Props["skills"])
	if len(got) != 2 || got[0] != "render-data-as-ui" || got[1] != "cwm-*" {
		t.Errorf("skills props = %v, want [render-data-as-ui cwm-*]", got)
	}
	gotTools := StringList(toolRef.Props["tools"])
	if len(gotTools) != 1 || gotTools[0] != "ktw-weather:weather-service/*" {
		t.Errorf("tools props = %v, want the weather wildcard with : intact", gotTools)
	}
}

func TestDesugarPluginLists_EmptyAddsNothing(t *testing.T) {
	r := &AgentRunRequest{Plugins: []PluginRef{{Type: "standard"}}}
	r.DesugarPluginLists()
	if len(r.Plugins) != 1 {
		t.Errorf("plugins = %d, want 1 (no sugar lists → no extra refs)", len(r.Plugins))
	}
}
