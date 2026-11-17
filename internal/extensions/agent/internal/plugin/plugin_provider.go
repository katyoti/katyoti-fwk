// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/katyoti/katyoti-fwk/internal/core/provider"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// GlobalGroups holds the group registry populated during config loading.
// Read by PluginManager at instantiation time for Layer 1 expansion.
var GlobalGroups GroupRegistry

// PluginsYAMLProvider implements ConfigProvider.
// It converts pre-parsed PluginRef slices into ConfigItems for the agent.plugin domain.
// Group refs are extracted into GlobalGroups (Layer 1) — they don't reach the domain.
type PluginsYAMLProvider struct {
	EmbeddedPlugins []api.PluginRef
	UserPlugins     []api.PluginRef
}

func (p *PluginsYAMLProvider) ProvideConfigFor(domain string) ([]provider.ConfigItem, error) {
	if domain != "agent.plugin" {
		return nil, nil
	}

	// Separate groups from regular refs (Layer 1 extraction).
	embedded, embeddedGroups := ExtractGroups(p.EmbeddedPlugins)
	user, userGroups := ExtractGroups(p.UserPlugins)

	// Merge groups into GlobalGroups. Framework groups are BINARY-OWNED: the
	// EMBEDDED definition wins, so a stale workspace `Plugins.yaml` (a first-run
	// copy that has since diverged) can NOT override a built-in group like
	// `standard` — e.g. it cannot reintroduce `bash` after the binary dropped it.
	// A workspace may still define its OWN new groups (no embedded conflict);
	// per-task needs are met by a task's own `plugins:` list, not by redefining a
	// framework group. (Reversed 2026-06-24: user-wins silently re-armed bash for
	// every task using `standard` on an old workspace.)
	GlobalGroups = make(GroupRegistry)
	for k, v := range userGroups {
		GlobalGroups[k] = v
	}
	for k, v := range embeddedGroups {
		GlobalGroups[k] = v
	}

	var items []provider.ConfigItem
	for _, ref := range embedded {
		items = append(items, pluginRefToConfigItem(ref, "embedded"))
	}
	for _, ref := range user {
		items = append(items, pluginRefToConfigItem(ref, "plugins.yaml"))
	}
	return items, nil
}

func pluginRefToConfigItem(ref api.PluginRef, source string) provider.ConfigItem {
	name := ref.Name
	if name == "" {
		name = ref.Type
	}

	data := map[string]any{
		"name": name,
		"type": ref.Type,
	}
	for k, v := range ref.Props {
		data[k] = v
	}

	return provider.ConfigItem{
		Data:   data,
		Source: source,
	}
}
