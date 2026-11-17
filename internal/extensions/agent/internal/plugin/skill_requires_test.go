// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

type fakeRequires map[string][]string

func (f fakeRequires) GetSkillRequires(name string) []string { return f[name] }

// A task's `skills:` list desugars to one ref with the selectors in Props
// and an empty name. Until 2026-09-22 the requirements hook looked up that
// empty name and found nothing: `requires:` was honoured only for the legacy
// `skill:<name>` form. Exact selectors resolve now; globs cannot name a
// skill and are left to the catalog.
func TestSkillGetRequirements_ResolvesTheListForm(t *testing.T) {
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	application.PutService[skillRequiresReader](app, "default", fakeRequires{
		"doc-create": {"workspace", "skill:template-engine"},
		"doc-fix":    {"workspace"},
	})
	ctx := application.WithApplication(context.Background(), app)

	ref := api.PluginRef{Type: "skill", Props: map[string]any{"skills": []any{"doc-create", "ktw-docs:/skill/doc-fix", "*", "trend-*"}}}
	got, err := skillGetRequirements(ref, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Ref() != "workspace" || got[1].Ref() != "skill:template-engine" {
		t.Fatalf("requires = %v, want [workspace skill:template-engine] (each once, globs skipped)", got)
	}

	// The legacy colon form still resolves by name.
	got, err = skillGetRequirements(api.PluginRef{Type: "skill", Name: "doc-fix"}, ctx)
	if err != nil || len(got) != 1 || got[0].Ref() != "workspace" {
		t.Fatalf("legacy form: %v %v", got, err)
	}
}
