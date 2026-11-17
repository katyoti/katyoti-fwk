// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// TestKindSourcesReadAsTheirProducersDid: the reference, contribution-type,
// display-engine, guide-recipe and data-type kinds declare how their files
// become layers (their `source:` block), so the generic producer reads them.
// Over every shipped folder of each (and the files-only fixtures'), it publishes what the kind's own producer published:
// the same ids, the same data, the same layers, the same media type (the
// kind's default layer's where a producer recorded none).
func TestKindSourcesReadAsTheirProducersDid(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	root := repoRoot(t)
	for _, tc := range kindSourceCases() {
		d, ok := kinds.Get(tc.kind)
		if !ok || d.Source == nil || d.SourceFolder() != tc.folder {
			t.Fatalf("%s: the system set declares no source in %s/", tc.kind, tc.folder)
		}
		folders := shippedKindFolders(root, tc.folder)
		if len(folders) == 0 {
			// The tree ships no folder of this kind (the packs that do are not
			// published); TestEveryKindSourceHasAShippedFolder holds the
			// coverage where they are.
			t.Logf("%s: no shipped %s/ folder in this tree", tc.kind, tc.folder)
			continue
		}
		entries := 0
		for _, f := range folders {
			ldr := loader.NewFSLoader(filepath.Dir(f))
			was := enumerateExt(t, tc.old("x", ldr, tc.folder))
			now := map[string]contribapi.Contribution{}
			for _, p := range manifestcontrib.KindProviders("x", ldr, tc.folder, d) {
				got, err := p.Enumerate(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				for _, c := range got {
					now[c.Type+"/"+c.ID] = c
				}
			}
			where, _ := filepath.Rel(root, f)
			if !sameIDs(was, now) {
				t.Errorf("%s: ids were %v, are %v", where, keys(was), keys(now))
				continue
			}
			for id, w := range was {
				n := now[id]
				entries++
				if !sameJSON(w.Data(), n.Data()) {
					t.Errorf("%s %s: data was %s, is %s", where, id, w.Data(), n.Data())
				}
				for l, ref := range w.Layers {
					if l == contribapi.LayerData {
						continue
					}
					if n.Layers[l].Hash != ref.Hash {
						t.Errorf("%s %s: layer %s was %s, is %s", where, id, l, ref.Hash, n.Layers[l].Hash)
					}
				}
				for l := range n.Layers {
					if _, had := w.Layers[l]; !had {
						t.Errorf("%s %s: layer %s is new", where, id, l)
					}
				}
				// A producer that recorded none now records the kind's default
				// layer's (the index says how every declared kind renders).
				want := mediaType(w)
				if want == "" {
					want = d.Layers[d.DefaultLayer()].MediaType
				}
				if mediaType(n) != want {
					t.Errorf("%s %s: media type was %q, is %q", where, id, mediaType(w), mediaType(n))
				}
			}
		}
		if entries == 0 {
			t.Errorf("%s: no shipped entry compared", tc.kind)
		}
	}
}

// kindSourceCase is one kind whose files the generic producer reads, with
// the producer that read them before.
type kindSourceCase struct {
	kind, folder string
	old          func(provider string, ldr loader.Loader, dir string) application.Extension
}

// kindSourceCases are the kinds TestKindSourcesReadAsTheirProducersDid compares.
func kindSourceCases() []kindSourceCase {
	return []kindSourceCase{
		{"reference", "references", manifestcontrib.ContributeReferenceResources},
		{"contribution-type", "contribution-types", manifestcontrib.ContributeContributionTypeResources},
		{"display-engine", "display-engines", func(p string, l loader.Loader, d string) application.Extension {
			return manifestcontrib.ContributeKindResources(p, "display-engine", l, d,
				manifestcontrib.WithSiblingLayer(contribapi.LayerWidget, ".html"))
		}},
		{"guide-recipe", "guide", func(p string, l loader.Loader, d string) application.Extension {
			return manifestcontrib.ContributeKindResources(p, "guide-recipe", l, d,
				manifestcontrib.WithSiblingLayer(contribapi.LayerInstructions, ".md"))
		}},
		// As a sidecar's and a host's data-types are published (sub-folders
		// kept in the id).
		{"data-type", "data-types", func(p string, l loader.Loader, d string) application.Extension {
			return manifestcontrib.ContributeDataTypeResources(p, l, d)
		}},
	}
}

// shippedKindFolders are the tree's shipped folders of a kind (the apps',
// the extensions' and the files-only fixtures').
func shippedKindFolders(root, folder string) []string {
	folders, _ := filepath.Glob(filepath.Join(root, "app", "*", "embed", folder))
	ext, _ := filepath.Glob(filepath.Join(root, "internal", "extensions", "*", "contrib", "embed", folder))
	fixtures, _ := filepath.Glob(filepath.Join(root, "internal", "test", "fixtures", "contributions", "*", folder))
	return append(append(folders, ext...), fixtures...)
}

// enumerateExt boots an application with one producer extension and returns
// what its providers publish, by kind/id.
func enumerateExt(t *testing.T, ext application.Extension) map[string]contribapi.Contribution {
	t.Helper()
	points := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(application.Config(), points, ext)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	out := map[string]contribapi.Contribution{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got {
			out[c.Type+"/"+c.ID] = c
		}
	}
	return out
}

// mediaType is the media type the index records for an entry ("" when none).
func mediaType(c contribapi.Contribution) string {
	if c.Metadata == nil {
		return ""
	}
	return c.Metadata.MediaType
}

func sameIDs(a, b map[string]contribapi.Contribution) bool {
	return strings.Join(keys(a), ",") == strings.Join(keys(b), ",")
}

func keys(m map[string]contribapi.Contribution) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sameJSON compares two JSON documents by value (key order aside).
func sameJSON(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	xa, _ := json.Marshal(x)
	ya, _ := json.Marshal(y)
	return bytes.Equal(xa, ya)
}
