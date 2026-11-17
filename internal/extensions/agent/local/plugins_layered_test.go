// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestLoadPluginsConfigLayered_AppendsAcrossLayers verifies the APPEND
// semantics promised by the universes-and-config-scoping ADR for Plugins.yaml:
// global plugin list + universe plugin list are concatenated, NOT replaced.
func TestLoadPluginsConfigLayered_AppendsAcrossLayers(t *testing.T) {
	// Two MapLoaders simulate the global + per-universe Plugins.yaml layers.
	// Build the parent (global) ConfigService with one defaults entry, one plugin.
	parentCfg := config.NewConfigService(loader.NewMapLoader(map[string]string{
		"Plugins.yaml": `defaults:
  - global-default
plugins:
  - type: skill
    name: global-skill
`,
	}))

	// Build the child (universe) ConfigService with another defaults entry + plugin.
	childCfg := parentCfg.Sub(loader.NewMapLoader(map[string]string{
		"Plugins.yaml": `defaults:
  - universe-default
plugins:
  - type: task
    name: universe-task
`,
	}))

	// Wire into an Application so loadPluginsConfigLayered can resolve it.
	app := application.New(childCfg)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)

	merged, err := loadPluginsConfigLayered(ctx)
	if err != nil {
		t.Fatalf("loadPluginsConfigLayered: %v", err)
	}

	// Defaults concatenated, parent first.
	if len(merged.Defaults) != 2 {
		t.Fatalf("Defaults len = %d, want 2 (global + universe)", len(merged.Defaults))
	}
	if merged.Defaults[0] != "global-default" {
		t.Errorf("Defaults[0] = %q, want global-default", merged.Defaults[0])
	}
	if merged.Defaults[1] != "universe-default" {
		t.Errorf("Defaults[1] = %q, want universe-default", merged.Defaults[1])
	}

	// Plugins concatenated, parent first.
	if len(merged.Plugins) != 2 {
		t.Fatalf("Plugins len = %d, want 2", len(merged.Plugins))
	}
	if merged.Plugins[0].Name != "global-skill" {
		t.Errorf("Plugins[0].Name = %q, want global-skill", merged.Plugins[0].Name)
	}
	if merged.Plugins[1].Name != "universe-task" {
		t.Errorf("Plugins[1].Name = %q, want universe-task", merged.Plugins[1].Name)
	}
}

// TestLoadPluginsConfigLayered_OnlyOneLayer — when no parent (single-tenant
// deployment), the helper degrades cleanly to a single-layer load.
func TestLoadPluginsConfigLayered_OnlyOneLayer(t *testing.T) {
	cfg := config.NewConfigService(loader.NewMapLoader(map[string]string{
		"Plugins.yaml": `defaults:
  - solo-default
plugins:
  - type: skill
    name: solo-skill
`,
	}))
	app := application.New(cfg)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)

	merged, err := loadPluginsConfigLayered(ctx)
	if err != nil {
		t.Fatalf("loadPluginsConfigLayered: %v", err)
	}
	if len(merged.Defaults) != 1 || merged.Defaults[0] != "solo-default" {
		t.Errorf("Defaults = %v, want [solo-default]", merged.Defaults)
	}
	if len(merged.Plugins) != 1 || merged.Plugins[0].Name != "solo-skill" {
		t.Errorf("Plugins = %v, want one entry named solo-skill", merged.Plugins)
	}
}

// TestLoadPluginsConfigLayered_EmptyChildKeepsParent — universe with no
// Plugins.yaml inherits the global list verbatim (no replacement).
func TestLoadPluginsConfigLayered_EmptyChildKeepsParent(t *testing.T) {
	parent := config.NewConfigService(loader.NewMapLoader(map[string]string{
		"Plugins.yaml": `defaults:
  - global-only
plugins:
  - type: skill
    name: global-skill
`,
	}))
	// Child has NO Plugins.yaml.
	child := parent.Sub(loader.NewMapLoader(nil))
	app := application.New(child)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)

	merged, err := loadPluginsConfigLayered(ctx)
	if err != nil {
		t.Fatalf("loadPluginsConfigLayered: %v", err)
	}
	if len(merged.Defaults) != 1 || merged.Defaults[0] != "global-only" {
		t.Errorf("Defaults = %v, want [global-only] (inherited from parent)", merged.Defaults)
	}
	if len(merged.Plugins) != 1 {
		t.Errorf("Plugins len = %d, want 1 (inherited from parent)", len(merged.Plugins))
	}
}

// Sanity: the helper's signature matches what factory.go consumes.
var _ = func() *api.PluginsConfig {
	merged, _ := loadPluginsConfigLayered(context.Background())
	return &merged
}
