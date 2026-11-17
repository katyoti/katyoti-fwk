// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Tier-2: skillTaskLibrary coverage. Closes the audit's
// test-coverage.md priority-7 gap (skill auto-wrap was scoped out of
// ms07d.1 T4). Five tests covering: ListTasks filtering by
// `provides: [task]`, GetTask URI lookup, GetTask bare-name lookup,
// the two edge cases (no provides; provides without "task") that
// must NOT surface skills as tasks.

// testSkill is the fixture shape: picker fields and wiring fields side by
// side, because a test wants to say both in one place even though the
// production envelope keeps them apart.
type testSkill struct {
	Name        string
	Description string
	Provides    []string
}

// buildSkillCatalog builds a fake catalog populated with skill resources,
// storing each the way the production provider does: picker fields in meta,
// wiring fields in the body.
func buildSkillCatalog(t *testing.T, perProvider map[string][]testSkill) *fakeCatalog {
	t.Helper()
	manifests := map[string]*mbapi.Manifest{}
	for provider, skills := range perProvider {
		m := &mbapi.Manifest{Provider: provider}
		for _, s := range skills {
			body, err := json.Marshal(mbapi.SkillBody{Provides: s.Provides})
			if err != nil {
				t.Fatalf("marshal skill body: %v", err)
			}
			m.Contributions = append(m.Contributions, contribapi.Contribution{
				Provider: provider,
				Type:     mbapi.ResourceTypeSkill,
				ID:       s.Name,
				Metadata: &contribapi.ContributionMeta{Name: s.Name, Description: s.Description},
				Layers:   contribapi.InlineData(body),
			})
		}
		manifests[provider] = m
	}
	return &fakeCatalog{manifests: manifests}
}

// withCatalogCtx wires a catalog into an Application context so
// `application.UseService[mbapi.Catalog](ctx)` resolves to it. Mirrors
// the skills/commands test harness.
func withCatalogCtx(t *testing.T, cat mbapi.Catalog) context.Context {
	t.Helper()
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", cat)
	return application.WithApplication(context.Background(), app)
}

// TestSkillTaskLibrary_ListTasks_FiltersByProvidesTask — only skills
// with `provides: [task]` in their frontmatter surface as tasks.
// Skills without provides, or with provides that don't include "task",
// stay invisible to ListTasks.
func TestSkillTaskLibrary_ListTasks_FiltersByProvidesTask(t *testing.T) {
	cat := buildSkillCatalog(t, map[string][]testSkill{
		"ktw-x": {
			{Name: "task-skill", Description: "I am a task", Provides: []string{"task"}},
			{Name: "skill-only", Description: "Plain skill — no provides"},
			{Name: "cli-skill", Description: "CLI only", Provides: []string{"cli"}},
			{Name: "multi", Description: "Both", Provides: []string{"task", "cli"}},
		},
	})
	lib := NewSkillTaskLibrary()
	ctx := withCatalogCtx(t, cat)

	resp, err := lib.ListTasks(ctx, taskapi.ListTasksRequest{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(resp.Tasks) != 2 {
		t.Errorf("got %d tasks, want 2 (task-skill + multi); %+v", len(resp.Tasks), resp.Tasks)
	}
	uris := map[string]bool{}
	for _, ts := range resp.Tasks {
		uris[ts.URI] = true
	}
	if !uris["ktw-x:task-skill"] {
		t.Errorf("missing task-skill in output: have %v", uris)
	}
	if !uris["ktw-x:multi"] {
		t.Errorf("missing multi-provides skill in output: have %v", uris)
	}
	if uris["ktw-x:skill-only"] {
		t.Error("skill-only (no provides) leaked into ListTasks output")
	}
	if uris["ktw-x:cli-skill"] {
		t.Error("cli-skill (provides:[cli], no task) leaked into ListTasks output")
	}
}

// TestSkillTaskLibrary_GetTask_URIExactMatch — `provider:name` URI
// lookup resolves to the right skill-derived task.
func TestSkillTaskLibrary_GetTask_URIExactMatch(t *testing.T) {
	cat := buildSkillCatalog(t, map[string][]testSkill{
		"ktw-docs": {
			{Name: "doc-create", Description: "Create docs", Provides: []string{"task"}},
		},
	})
	lib := NewSkillTaskLibrary()
	ctx := withCatalogCtx(t, cat)

	info, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "ktw-docs:/skill/doc-create"})
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if info.Name != "doc-create" {
		t.Errorf("Name = %q, want doc-create", info.Name)
	}
	if info.Provider != "ktw-docs" {
		t.Errorf("Provider = %q, want ktw-docs", info.Provider)
	}
	if info.Type != "agent" {
		// Skill-derived tasks always wrap as type:agent — the body is the
		// LLM prompt that loads the skill's methodology.
		t.Errorf("Type = %q, want agent (skill wrapper)", info.Type)
	}
	if !strings.Contains(info.Body, "doc-create") {
		t.Errorf("Body should mention the skill name; got %q", info.Body)
	}
	if !strings.Contains(info.Body, "skills-get-instruction") {
		t.Errorf("Body should reference get_skill_instructions tool; got %q", info.Body)
	}
}

// TestSkillTaskLibrary_GetTask_BareNameUnique — a bare name (no
// `provider:` prefix) resolves when there's exactly one match across
// all providers. Mirrors the localTaskLibrary semantics.
func TestSkillTaskLibrary_GetTask_BareNameUnique(t *testing.T) {
	cat := buildSkillCatalog(t, map[string][]testSkill{
		"ktw-docs": {
			{Name: "doc-create", Description: "Create docs", Provides: []string{"task"}},
		},
		"ktw-other": {
			{Name: "other-skill", Description: "Other", Provides: []string{"task"}},
		},
	})
	lib := NewSkillTaskLibrary()
	ctx := withCatalogCtx(t, cat)

	info, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "doc-create"})
	if err != nil {
		t.Fatalf("GetTask bare name: %v", err)
	}
	if info.Provider != "ktw-docs" {
		t.Errorf("Provider = %q, want ktw-docs (the unique provider for doc-create)", info.Provider)
	}
}

// TestSkillTaskLibrary_GetTask_BareNameAmbiguous — when the same bare
// name lives in two providers, GetTask returns an error citing both
// URIs so the caller can disambiguate.
func TestSkillTaskLibrary_GetTask_BareNameAmbiguous(t *testing.T) {
	cat := buildSkillCatalog(t, map[string][]testSkill{
		"ktw-a": {{Name: "build", Description: "Build A", Provides: []string{"task"}}},
		"ktw-b": {{Name: "build", Description: "Build B", Provides: []string{"task"}}},
	})
	lib := NewSkillTaskLibrary()
	ctx := withCatalogCtx(t, cat)

	_, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "build"})
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ambiguous") {
		t.Errorf("error %q should mention 'ambiguous'", msg)
	}
	// Both URIs should be in the message so the user can pick.
	for _, want := range []string{"ktw-a:build", "ktw-b:build"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}

// TestSkillTaskLibrary_GetTask_NotFound — unknown name returns a clear
// error.
func TestSkillTaskLibrary_GetTask_NotFound(t *testing.T) {
	cat := buildSkillCatalog(t, map[string][]testSkill{
		"ktw-x": {{Name: "real-skill", Description: "Real", Provides: []string{"task"}}},
	})
	lib := NewSkillTaskLibrary()
	ctx := withCatalogCtx(t, cat)

	_, err := lib.GetTask(ctx, taskapi.GetTaskRequest{Name: "doesnt-exist"})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should mention 'not found'", err.Error())
	}
}
