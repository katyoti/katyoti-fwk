// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

const projectOrigin = "project:acme"

// readPack enumerates a sidecar's pack read through l: its entries by URI,
// what each override tree served, and what was dropped.
func readPack(t *testing.T, l loader.Loader) (map[string]contribapi.Contribution, []contribapi.Override, []contribapi.Drop) {
	t.Helper()
	kinds := overrideKinds(t)
	entries := map[string]contribapi.Contribution{}
	var overrides []contribapi.Override
	var drops []contribapi.Drop
	for _, p := range PackProviders("ktw-x", l, "embed", kinds) {
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("enumerate %s: %v", p.ResourceType, err)
		}
		for _, c := range got {
			entries[c.ResolvedURI()] = c
		}
		overrides = append(overrides, p.Overrides()...)
		drops = append(drops, p.Drops()...)
	}
	return entries, overrides, drops
}

// overrideKinds is the system set plus the skill and task kinds as their
// packs declare them (their home is their pack, not the system set; the leaf
// packages import this one, so the test plants their source blocks).
func overrideKinds(t *testing.T) contribapi.Kinds {
	t.Helper()
	entries, err := contribution.SystemKindEntries()
	if err != nil {
		t.Fatal(err)
	}
	skill := contribapi.KindDeclaration{Name: "skill",
		Layers: map[string]contribapi.LayerDecl{"data": {}, "instructions": {Default: true}, "source": {}},
		Source: &contribapi.SourceDecl{Folder: "skills", Match: []string{"*.md"}, ID: "name", Format: contribapi.SourceFrontmatter,
			Layers: map[string]contribapi.LayerFrom{
				"data":         {From: contribapi.FromFrontmatter},
				"instructions": {From: contribapi.FromBody},
				"source":       {From: contribapi.FromFile},
			}}}
	task := contribapi.KindDeclaration{Name: "task", Execution: "executable",
		Layers: map[string]contribapi.LayerDecl{"data": {Default: true}, "source": {}},
		Source: &contribapi.SourceDecl{Folder: "tasks", Match: []string{"*.md"}, ID: "name", Claims: "TASK.md", Format: contribapi.SourceFrontmatter,
			Layers: map[string]contribapi.LayerFrom{
				"data":   {From: contribapi.FromFrontmatter},
				"body":   {From: contribapi.FromBody, Fold: "body"},
				"source": {From: contribapi.FromFile},
			}}}
	for _, d := range []contribapi.KindDeclaration{skill, task} {
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		c := contribapi.Contribution{Provider: "ktw-pack", Type: contribapi.ResourceTypeContributionType, ID: d.Name}
		c.SetData(b)
		entries = append(entries, c)
	}
	return contribapi.KindsFrom(entries)
}

func embedded() loader.Loader {
	return loader.NewMapLoader(map[string]string{
		"embed/skills/greet.md":      "---\nname: greet\ndescription: Say hello\n---\nEMBED hello.\n",
		"embed/tasks/review/TASK.md": "---\nname: review\ntype: agent\ndescription: d\n---\nEMBED review.\n",
		"embed/guide/tour.yaml":      "name: Tour\ndescription: a tour\nsteps: []\n",
		"embed/skills/untouched.md":  "---\nname: untouched\ndescription: d\n---\nEMBED only.\n",
	})
}

// untrustedTasks refuses an override under tasks/, as an untrusted project's.
func untrustedTasks(path string) string {
	if strings.HasPrefix(path, "embed/tasks/") {
		return "an override of executable content from project acme, which the universe does not trust"
	}
	return ""
}

// A project's override of a skill is served and said: the manifest names the
// entry, each layer the override served and the tree it came from; an
// untouched entry names none.
func TestAnOverriddenEntryNamesItsLayersAndTheirSource(t *testing.T) {
	over := loader.NewMapLoader(map[string]string{
		"embed/skills/greet.md": "---\nname: greet\ndescription: Say hello\n---\nPROJECT hello.\n",
	})
	entries, overrides, _ := readPack(t, loader.NamedOverlay(embedded(), over, projectOrigin, nil))
	if _, ok := entries["ktw-x:/skill/greet"]; !ok {
		t.Fatalf("entries = %v", entries)
	}
	layers := map[string]string{}
	for _, o := range overrides {
		if o.URI == "ktw-x:/skill/untouched" {
			t.Errorf("an untouched entry names an override: %+v", o)
		}
		if o.URI == "ktw-x:/skill/greet" {
			layers[o.Layer] = o.From
		}
	}
	if layers[contribapi.LayerSource] != projectOrigin || layers[contribapi.LayerInstructions] != projectOrigin {
		t.Fatalf("greet's overridden layers = %v; want source and instructions from %s", layers, projectOrigin)
	}
}

// A translations file alone in the override tree adds the entry's
// translations layer and leaves its other layers unchanged.
func TestALoneTranslationsOverrideAddsOnlyThatLayer(t *testing.T) {
	plain, _, _ := readPack(t, embedded())
	over := loader.NewMapLoader(map[string]string{
		"embed/guide/tour.i18n.yaml": "fr:\n  name: Visite\n",
	})
	entries, overrides, _ := readPack(t, loader.NamedOverlay(embedded(), over, projectOrigin, nil))
	before, after := plain["ktw-x:/guide-recipe/tour"], entries["ktw-x:/guide-recipe/tour"]
	if _, had := before.Layers[contribapi.LayerTranslations]; had {
		t.Fatal("the embedded tour already has translations: the fixture proves nothing")
	}
	if _, ok := after.Layers[contribapi.LayerTranslations]; !ok {
		t.Fatalf("tour's layers = %v; want the translations layer added", after.Layers)
	}
	for name, l := range before.Layers {
		if after.Layers[name].Hash != l.Hash {
			t.Errorf("layer %s changed: %s -> %s", name, l.Hash, after.Layers[name].Hash)
		}
	}
	if len(overrides) != 1 || overrides[0].Layer != contribapi.LayerTranslations || overrides[0].From != projectOrigin {
		t.Fatalf("overrides = %+v; want only tour's translations from %s", overrides, projectOrigin)
	}
}

// A task override from an untrusted project is not served: the embedded task
// is, the refused file is a drop that says why; a NEW task from it is not
// published at all, and is dropped too.
func TestAnUntrustedTaskOverrideIsNotServedAndIsReported(t *testing.T) {
	over := loader.NewMapLoader(map[string]string{
		"embed/tasks/review/TASK.md": "---\nname: review\ntype: agent\ndescription: d\n---\nPROJECT review.\n",
		"embed/tasks/sneak/TASK.md":  "---\nname: sneak\ntype: agent\ndescription: d\n---\nPROJECT sneak.\n",
	})
	plain, _, _ := readPack(t, embedded())
	entries, overrides, drops := readPack(t, loader.NamedOverlay(embedded(), over, projectOrigin, untrustedTasks))
	got, ok := entries["ktw-x:/task/review"]
	if !ok || got.Layers[contribapi.LayerSource].Hash != plain["ktw-x:/task/review"].Layers[contribapi.LayerSource].Hash {
		t.Fatalf("review = %+v; want the embedded task served", got.Layers)
	}
	if _, ok := entries["ktw-x:/task/sneak"]; ok {
		t.Fatal("an untrusted project's new task is published")
	}
	for _, o := range overrides {
		if strings.HasPrefix(o.URI, "ktw-x:/task/") {
			t.Errorf("a refused task is named as overridden: %+v", o)
		}
	}
	dropped := map[string]string{}
	for _, d := range drops {
		dropped[d.Path] = d.Reason
	}
	for _, p := range []string{"embed/tasks/review/TASK.md", "embed/tasks/sneak/TASK.md"} {
		if !strings.Contains(dropped[p], "does not trust") {
			t.Errorf("drop for %s = %q; want the refusal said", p, dropped[p])
		}
	}
}
