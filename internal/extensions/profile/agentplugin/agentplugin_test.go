// SPDX-License-Identifier: Apache-2.0

package agentplugin

import (
	"context"
	"strings"
	"testing"
)

// A run gets the person's language as a system-prompt block; with no language
// set (or no reachable profile) it gets no plugin at all.
func TestPersonLanguagePlugin(t *testing.T) {
	saved := languageOf
	t.Cleanup(func() { languageOf = saved })

	languageOf = func(context.Context) string { return "fr" }
	plugins, err := personLanguageFactory().CreatePlugins(context.Background(), nil, nil)
	if err != nil || len(plugins) != 1 {
		t.Fatalf("fr: %d plugins, %v", len(plugins), err)
	}
	if msg := plugins[0].SystemMessage(); !strings.Contains(msg, "French") || plugins[0].Name() != PersonLanguagePlugin {
		t.Errorf("fr block = %q", msg)
	}

	languageOf = func(context.Context) string { return "" }
	if plugins, _ := personLanguageFactory().CreatePlugins(context.Background(), nil, nil); len(plugins) != 0 {
		t.Errorf("no language: %d plugins, want none", len(plugins))
	}
}
