// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// The task the runtime runs is the catalog's: without an
// override, exactly what Parse and the contract files beside it give; with a
// trusted override entry of its data layer and of its input contract, the
// override's.

var catalogTaskFiles = map[string]string{
	"tasks/greet.md": "---\nname: greet\ntype: agent\ndescription: Say hello\ntitle: Greet\nicon: x\n" +
		"tags: [quick-action]\npermission_profile: workspace-edit\ngroup: chat\ncapability: smart\n" +
		"session:\n  mode: session\nconfig:\n  max_turns: 7\n  temperature: 0.5\n  tools: [read]\n---\nSay hello.\n",
	"tasks/greet.input.dsl.yaml":        "name: greet-input\nroot: In\ntypes:\n  In:\n    type: object\n    fields:\n      who: {type: string}\n",
	"tasks/review/TASK.md":              "---\nname: code-review\ntype: agent\ndescription: d\nkind: object\ntypes: [module]\nadvertised: false\n---\nReview.\n",
	"tasks/review/TASK.output.dsl.yaml": "name: review-output\nroot: Out\ntypes:\n  Out:\n    type: object\n    fields:\n      ok: {type: boolean}\n",
}

// overrideProvider holds override entries only, as a project's object-backed
// provider does.
type overrideProvider struct{ entries []contribapi.OverrideEntry }

func (p overrideProvider) Type() string                  { return mbapi.ResourceTypeTask }
func (p overrideProvider) Provider() string              { return "project-example" }
func (p overrideProvider) Identity() contribapi.Identity { return contribapi.Identity{Mutable: true} }
func (p overrideProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	return nil, nil
}
func (p overrideProvider) Open(context.Context, string, string) (io.ReadCloser, contribapi.LayerRef, error) {
	return nil, contribapi.LayerRef{}, contribapi.ErrContributionNotFound{}
}
func (p overrideProvider) OverrideEntries() []contribapi.OverrideEntry { return p.entries }

// exampleCatalog is the catalog of a member serving catalogTaskFiles, with
// the override entries given, built by the manifest builder (which applies
// them).
func exampleCatalog(t *testing.T, ovs ...contribapi.OverrideEntry) mbapi.Catalog {
	t.Helper()
	ctx := context.Background()
	var ps []contribapi.ContributionProvider
	for _, p := range TaskProviders("ktw-example", loader.NewMapLoader(catalogTaskFiles), "tasks") {
		ps = append(ps, p)
	}
	if len(ovs) > 0 {
		ps = append(ps, overrideProvider{entries: ovs})
	}
	kinds, err := builtin.Kinds()
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifestlocal.ProvidersManifest(ctx, "ktw-example", ps, kinds)
	if err != nil {
		t.Fatal(err)
	}
	return manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{m}, nil)
}

func TestCatalogTask_IsWhatTheFileGives(t *testing.T) {
	ctx := context.Background()
	cat := exampleCatalog(t)
	fromFiles := DiscoverTasksFrom([]taskapi.TaskLoader{{Loader: loader.NewMapLoader(catalogTaskFiles), Root: "tasks", Provider: "ktw-example"}})
	for _, ref := range []string{"ktw-example:greet", "ktw-example:/task/code-review", "greet"} {
		got, err := CatalogTask(ctx, cat, ref)
		if err != nil {
			t.Fatalf("CatalogTask(%s): %v", ref, err)
		}
		want := *fromFiles["ktw-example:"+got.Name]
		want.Source = nil // the file is its source layer, not what runs
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("CatalogTask(%s) =\n  %+v\nParse + its contract files =\n  %+v", ref, *got, want)
		}
	}
	if greet := fromFiles["ktw-example:greet"]; len(greet.InputSchema) == 0 || greet.PermissionProfile == "" || greet.SessionMode != "session" {
		t.Fatalf("the fixture proves nothing: greet = %+v", greet)
	}
	if _, err := CatalogTask(ctx, cat, "absent"); !errors.Is(err, ErrTaskNotInCatalog) {
		t.Errorf("CatalogTask(absent) = %v, want ErrTaskNotInCatalog", err)
	}
}

func TestCatalogTask_RunsTheOverride(t *testing.T) {
	ctx := context.Background()
	data, _ := json.Marshal(map[string]any{
		"name": "greet", "type": "agent", "description": "Say hello, the project's way",
		"config": map[string]any{"max_turns": 3}, "body": "Say bonjour.",
	})
	task := contribapi.Contribution{Provider: "project-example", Type: mbapi.ResourceTypeTask, ID: "greet", Layers: contribapi.InlineData(data)}
	input := contribapi.Contribution{Provider: "project-example", Type: contribapi.ResourceTypeDataType, ID: "task/greet/input"}
	input.SetSource([]byte("name: greet-input\nroot: In\ntypes:\n  In:\n    type: object\n    fields:\n      whom: {type: string}\n"), "application/yaml")
	origin := contribapi.ProjectOrigin("example")
	cat := exampleCatalog(t,
		contribapi.OverrideEntry{Overrides: "ktw-example:/task/greet", Entry: task, Origin: origin, Trusted: true},
		contribapi.OverrideEntry{Overrides: "ktw-example:/data-type/task/greet/input", Entry: input, Origin: origin, Trusted: true},
	)
	for _, ref := range []string{"ktw-example:greet", "greet"} {
		got, err := CatalogTask(ctx, cat, ref)
		if err != nil {
			t.Fatalf("CatalogTask(%s): %v", ref, err)
		}
		if got.Provider != "ktw-example" || got.Body != "Say bonjour." || got.Description != "Say hello, the project's way" ||
			got.Config["max_turns"] != 3 || got.PermissionProfile != "" {
			t.Errorf("CatalogTask(%s) = %+v; want the override's data layer, under the overridden entry's provider", ref, *got)
		}
		if !strings.Contains(string(got.InputSchema), `"whom"`) {
			t.Errorf("CatalogTask(%s) input contract = %s; want the override's", ref, got.InputSchema)
		}
	}
}

func TestCatalogTask_BareNameAmbiguityPrefersTheOwnProvider(t *testing.T) {
	ctx := context.Background()
	m := func(provider string) *mbapi.Manifest {
		data, _ := json.Marshal(mbapi.TaskBody{Name: "same", Type: "agent", Body: provider})
		return &mbapi.Manifest{Provider: provider, Contributions: []contribapi.Contribution{
			{Provider: provider, Type: mbapi.ResourceTypeTask, ID: "same", Layers: contribapi.InlineData(data)},
		}}
	}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{m("ktw-example"), m("ktw-other")}, nil)
	if _, err := CatalogTask(ctx, cat, "same"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("CatalogTask(same) = %v, want ambiguous", err)
	}
	got, err := CatalogTask(ctx, cat, "same", "ktw-other")
	if err != nil || got.Provider != "ktw-other" || got.Body != "ktw-other" {
		t.Errorf("CatalogTask(same, prefer ktw-other) = %+v, %v; want ktw-other's", got, err)
	}
}
