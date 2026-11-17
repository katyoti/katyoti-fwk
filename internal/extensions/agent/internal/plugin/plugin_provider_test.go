// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestEmbeddedGroupsWin pins Gap-3: a framework group is binary-owned, so a stale
// workspace Plugins.yaml cannot reintroduce bash into `standard`, while a
// workspace's OWN new group still survives.
func TestEmbeddedGroupsWin(t *testing.T) {
	embedded := []api.PluginRef{
		{Type: "group", Name: "standard", Props: map[string]any{"members": []any{"workspace", "task"}}},
	}
	user := []api.PluginRef{
		// Stale copy: standard still lists bash.
		{Type: "group", Name: "standard", Props: map[string]any{"members": []any{"workspace", "bash"}}},
		// A genuinely workspace-defined group — must survive.
		{Type: "group", Name: "myteam", Props: map[string]any{"members": []any{"workspace"}}},
	}
	p := &PluginsYAMLProvider{EmbeddedPlugins: embedded, UserPlugins: user}
	if _, err := p.ProvideConfigFor("agent.plugin"); err != nil {
		t.Fatal(err)
	}

	for _, m := range GlobalGroups["standard"] {
		if m.Type == "bash" || m.Name == "bash" {
			t.Errorf("stale workspace reintroduced bash into the binary-owned `standard`: %+v", GlobalGroups["standard"])
		}
	}
	if _, ok := GlobalGroups["myteam"]; !ok {
		t.Error("a workspace-only group must survive the merge")
	}
}
