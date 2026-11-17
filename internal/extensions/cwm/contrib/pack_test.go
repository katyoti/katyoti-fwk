// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// providersPoint registers the point the manifest extension normally owns, so
// the test runs the real phase sequence without the manifest service.
func providersPoint(init *application.ApplicationInit) {
	init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
		return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
	})
}

func objectType(name string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte("name: " + name + "\nfolder: objects\ncapabilities: [container]\n")}
}

func runApp(t *testing.T, exts ...application.Extension) (*application.Application, error) {
	t.Helper()
	app := application.New(application.Config(), append([]application.Extension{providersPoint}, exts...)...)
	return app, app.Run()
}

// packProvider is one provider of a CWM pack's kind, as the tests name it.
type packProvider struct {
	*manifestcontrib.EmbedFSResourceProvider
	kind, provider string
}

// cwmProviders are the providers of the application's CWM packs: the generic
// producer over the provider's pack parts, one per kind — the flat data-types
// publication left out.
func cwmProviders(app *application.Application) []*packProvider {
	var out []*packProvider
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if t, ok := p.(*manifestcontrib.EmbedFSResourceProvider); ok && t.ResourceType != contribapi.ResourceTypeDataType {
			out = append(out, &packProvider{EmbedFSResourceProvider: t, kind: t.ResourceType, provider: t.ProviderName})
		}
	}
	return out
}

// onePerKind fails when two providers publish one kind under one name.
func onePerKind(t *testing.T, ps []*packProvider) {
	t.Helper()
	seen := map[string]bool{}
	for _, p := range ps {
		key := p.provider + "/" + p.kind
		if seen[key] {
			t.Errorf("two providers of %s", key)
		}
		seen[key] = true
	}
}

func enumerateIDs(t *testing.T, p *packProvider) map[string]bool {
	t.Helper()
	got, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatalf("Enumerate %s/%s: %v", p.provider, p.kind, err)
	}
	ids := map[string]bool{}
	for _, c := range got {
		ids[c.ID] = true
	}
	return ids
}

// TestExtensionPackPublishesUnderItsHost: a sidecar's own pack and a pack its
// extension contributes are published by ONE provider per kind, under the
// host's name — so a type moved from the sidecar pack into its extension keeps
// its URI, and the catalog holds no duplicate provider.
func TestExtensionPackPublishesUnderItsHost(t *testing.T) {
	host := fstest.MapFS{"object-types/host-thing.yaml": objectType("host-thing")}
	ext := fstest.MapFS{"object-types/goal.yaml": objectType("goal")}

	app, err := runApp(t,
		ContributeCWMTypes("ktw-host-x", host),
		manifestcontrib.ContributePackPart("ktw-host-x", "goals", "cwm", ext),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ps := cwmProviders(app)
	onePerKind(t, ps)
	var objectTypes *packProvider
	for _, p := range ps {
		if p.provider != "ktw-host-x" {
			t.Errorf("provider %q, want the host's name", p.provider)
		}
		if p.kind == "object-type" {
			objectTypes = p
		}
	}
	ids := enumerateIDs(t, objectTypes)
	if !ids["host-thing"] || !ids["goal"] {
		t.Fatalf("object types = %v, want both the host's and the extension's", ids)
	}
}

// TestPacksOfDistinctProvidersStayApart: a binary hosting packs for two
// provider names (ktw-host does) gets one provider set per name.
func TestPacksOfDistinctProvidersStayApart(t *testing.T) {
	app, err := runApp(t,
		ContributeCWMTypes("pack-a", fstest.MapFS{"object-types/a.yaml": objectType("a")}),
		ContributeCWMTypes("pack-b", fstest.MapFS{"object-types/b.yaml": objectType("b")}),
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ps := cwmProviders(app)
	onePerKind(t, ps)
	if len(ps) != 2 {
		t.Fatalf("got %d cwm providers, want one object-type provider per name", len(ps))
	}
	for _, p := range ps {
		if p.kind != "object-type" {
			continue
		}
		ids := enumerateIDs(t, p)
		want := map[string]string{"pack-a": "a", "pack-b": "b"}[p.provider]
		if len(ids) != 1 || !ids[want] {
			t.Errorf("%s object types = %v, want only %q", p.provider, ids, want)
		}
	}
}

// TestOverlappingPacksAreRefused: two packs of one provider declaring the same
// thing would shadow each other silently (layering is last-wins). Boot fails
// and names both owners.
func TestOverlappingPacksAreRefused(t *testing.T) {
	_, err := runApp(t,
		ContributeCWMTypes("ktw-host-x", fstest.MapFS{"object-types/goal.yaml": objectType("goal")}),
		manifestcontrib.ContributePackPart("ktw-host-x", "goals", "cwm", fstest.MapFS{"object-types/goal.yaml": objectType("goal")}),
	)
	if err == nil {
		t.Fatal("Run succeeded; want the overlap refused")
	}
	for _, want := range []string{"object-type/goal", "ktw-host-x", "goals"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

// TestExtensionPackPublishesItsDataTypes: an extension has no bootstrap of its
// own, so its pack's field-group schemas are published with it.
func TestExtensionPackPublishesItsDataTypes(t *testing.T) {
	ext := fstest.MapFS{
		"capabilities/goal-extra.yaml":       {Data: []byte("name: goal-extra\nschema: ktw-host-x:/data-type/cap-goal-extra\n")},
		"data-types/cap-goal-extra.dsl.yaml": {Data: []byte("$schema: ktw-schema/v1\nname: cap-goal-extra\nroot: X\ntypes:\n  X:\n    type: object\n")},
	}
	app, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "goals", "cwm", ext))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	found := false
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if p.Type() != contribapi.ResourceTypeDataType || p.Provider() != "ktw-host-x" {
			continue
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got {
			if c.ID == "cap-goal-extra" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("the extension pack's data-type cap-goal-extra is not published")
	}
}

// TestAPackPublishesItsWorkflows: a `workflows/*.yaml` in a pack is a `workflow`
// contribution, named by its body (or its file), with its states and transitions.
func TestAPackPublishesItsWorkflows(t *testing.T) {
	ext := fstest.MapFS{"workflows/review.yaml": {Data: []byte(
		"initial: draft\nstates:\n  - {name: draft}\n  - {name: done, terminal: true}\n" +
			"transitions:\n  - {name: finish, from: [draft], to: done, gate: \"by.kind == 'person'\"}\n")}}
	app, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "planification", "cwm", ext))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, p := range cwmProviders(app) {
		if p.kind != "workflow" {
			continue
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].ID != "review" {
			t.Fatalf("workflows = %+v, want review", got)
		}
		var w api.Workflow
		if err := json.Unmarshal(got[0].Data(), &w); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if w.Name != "review" || w.Initial != "draft" || len(w.Transitions) != 1 || w.Transitions[0].Gate == "" {
			t.Fatalf("workflow = %+v", w)
		}
		return
	}
	t.Fatal("no workflow provider")
}

// TestAPackFolderNoKindReadsIsRefused: a pack folder no CWM kind's
// declaration names refuses the boot instead of being dropped without a word.
func TestAPackFolderNoKindReadsIsRefused(t *testing.T) {
	ext := fstest.MapFS{"notes/first.yaml": {Data: []byte("text: hello\n")}}
	_, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "goals", "cwm", ext))
	if err == nil || !strings.Contains(err.Error(), "notes/ in goals") {
		t.Fatalf("err = %v, want the unread notes/ folder named", err)
	}
	// An extension's kind is a CWM kind: its folder is read with no
	// registration by the extension.
	gates := fstest.MapFS{"gates/review.yaml": {Data: []byte("name: review\n")}}
	if _, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "goals", "cwm", gates)); err != nil {
		t.Fatalf("a gates/ folder: %v", err)
	}
}

// TestAPackPublishesItsHookTypesAndHooks: `hook-types/*.yaml` and `hooks/*.yaml`
// in a pack are `hook-type` and `hook` contributions, named after their files.
func TestAPackPublishesItsHookTypesAndHooks(t *testing.T) {
	ext := fstest.MapFS{
		"hook-types/review.yaml": {Data: []byte("timing: on-demand\nanswer: findings\n")},
		"hooks/second-look.yaml": {Data: []byte("type: review\nuri: ktw-x:review/look\nfilter: {types: [doc]}\ninput: {id: \"${id}\"}\n")},
	}
	app, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "review", "cwm", ext))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range cwmProviders(app) {
		if p.kind != "hook-type" && p.kind != "hook" {
			continue
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got {
			seen[p.kind+"/"+c.ID] = true
			if p.kind == "hook" {
				var h api.Hook
				if err := json.Unmarshal(c.Data(), &h); err != nil || h.Type != "review" || h.URI == "" || len(h.Filter.Types) != 1 || h.Input["id"] != "${id}" {
					t.Fatalf("hook = %+v (%v)", h, err)
				}
			}
		}
	}
	if !seen["hook-type/review"] || !seen["hook/second-look"] {
		t.Fatalf("published = %v, want hook-type/review and hook/second-look", seen)
	}
}

// A pack (or an override tree) laid out before the rename of a kind's folder
// is still read: `object_types/` and `edge_types/` are the object-type and
// edge-type kinds' former folders (their declarations' `former:`).
func TestAPackOfFormerFolderNamesIsStillRead(t *testing.T) {
	ext := fstest.MapFS{
		"object_types/memo.yaml": {Data: []byte("name: memo\n")},
		"edge_types/cites.yaml":  {Data: []byte("name: cites\nfrom: any\nto: any\n")},
	}
	app, err := runApp(t, manifestcontrib.ContributePackPart("ktw-host-x", "legacy", "cwm", ext))
	if err != nil {
		t.Fatalf("a pack of former folder names refuses the boot: %v", err)
	}
	seen := map[string]bool{}
	for _, p := range cwmProviders(app) {
		for id := range enumerateIDs(t, p) {
			seen[p.kind+"/"+id] = true
		}
	}
	if !seen["object-type/memo"] || !seen["edge-type/cites"] {
		t.Fatalf("published %v, want object-type/memo and edge-type/cites from the former folders", seen)
	}
}
