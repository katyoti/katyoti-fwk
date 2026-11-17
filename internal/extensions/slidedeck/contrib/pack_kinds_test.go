// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib/deckkind"
	decklint "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/lint"
)

const packGroup = `name: our-team
description: Who we are.
slides: |
  <!-- slide: layout=cover -->
  # Our team
`

// A theme, its files and a group, laid out as a pack (pack/).
func deckFiles() fstest.MapFS {
	fs := fstest.MapFS{}
	put := func(p, s string) { fs[p] = &fstest.MapFile{Data: []byte(s)} }
	put("pack/deck-themes/brand.yaml", soundTheme)
	for f := range soundFiles {
		put("pack/deck-assets/"+f, "file "+f)
	}
	put("pack/slide-groups/our-team.yaml", packGroup)
	return fs
}

// enumerate runs exts in an app and returns every entry its providers list.
func enumerate(t *testing.T, exts ...application.Extension) []contribapi.Contribution {
	t.Helper()
	point := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), append([]application.Extension{point}, exts...)...)
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	var out []contribapi.Contribution
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, cs...)
	}
	return out
}

// deckKinds is the system set plus the deck kinds as a binary publishes them
// from their leaf: what a sidecar that links no slidedeck package knows.
func deckKinds(t *testing.T) contribapi.Kinds {
	t.Helper()
	sys, err := contribution.SystemKindEntries()
	if err != nil {
		t.Fatal(err)
	}
	entries := append([]contribapi.Contribution(nil), sys...)
	for _, c := range enumerate(t, builtin.ContributeLeafKinds("p", deckkind.Leaves()...)) {
		if c.Type == contribapi.ResourceTypeContributionType {
			entries = append(entries, c)
		}
	}
	k := contribapi.KindsFrom(entries)
	for _, name := range []string{deckkind.KindDeckTheme, deckkind.KindDeckAsset, deckkind.KindSlideGroup} {
		if _, ok := k.Get(name); !ok {
			t.Fatalf("kind %s not published by its leaf", name)
		}
	}
	for _, f := range k.Check() {
		if strings.HasPrefix(f.Kind, "deck-") || f.Kind == deckkind.KindSlideGroup {
			t.Errorf("a deck kind's declaration has a finding: %s", f)
		}
	}
	return k
}

func byURI(cs []contribapi.Contribution) map[string]contribapi.Contribution {
	out := map[string]contribapi.Contribution{}
	for _, c := range cs {
		out[c.ResolvedURI()] = c
	}
	return out
}

func decoded(t *testing.T, b json.RawMessage) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("data %s: %v", b, err)
	}
	return m
}

// A pack holding deck-themes, deck-assets and slide-groups folders, read by
// the generic pack reader with the kinds the deck leaves declare, publishes
// a theme, each of its files and a group: a theme's and a group's data is
// their YAML's tree and their picker fields derive from it (every binary
// declares the kind); a file's data is its path under deck-assets (the theme
// its first segment), its source the file, typed by its extension - and the
// slide-deck lint, not the contributor, reports a theme that is wrong.
func TestDeckThemesAreReadFromAPack(t *testing.T) {
	fs := deckFiles()
	ldr := loader.NewEmbedFSLoader(fs, ".")

	kinds := deckKinds(t)
	var read []contribapi.Contribution
	for _, p := range manifestcontrib.PackProviders("p", ldr, "pack", kinds) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for i := range cs {
			if err := contribapi.ApplyDerivedMeta(&cs[i], kinds); err != nil {
				t.Fatal(err)
			}
		}
		read = append(read, cs...)
	}
	got := byURI(read)

	want := map[string]string{ // URI -> the pack file it is read from
		"p:/deck-theme/brand":     "pack/deck-themes/brand.yaml",
		"p:/slide-group/our-team": "pack/slide-groups/our-team.yaml",
	}
	for f := range soundFiles {
		want["p:/deck-asset/"+f] = "pack/deck-assets/" + f
	}
	if len(got) != len(want) {
		t.Errorf("the pack published %d entries, want a theme, three files and a group", len(got))
	}
	for uri, file := range want {
		n, ok := got[uri]
		if !ok {
			t.Errorf("%s: not read from the pack", uri)
			continue
		}
		if string(n.Source()) != string(fs[file].Data) {
			t.Errorf("%s: source %q, want the file %s", uri, n.Source(), file)
		}
		nm := n.ResolvedMetadata()
		switch n.Type {
		case deckkind.KindDeckAsset:
			rel := strings.TrimPrefix(file, "pack/deck-assets/")
			if nd := decoded(t, n.Data()); nd["path"] != rel || len(nd) != 1 {
				t.Errorf("%s: data %v, want path %q", uri, nd, rel)
			}
			if nm.MediaType == "" {
				t.Errorf("%s: a file is typed by its extension, got no media type", uri)
			}
		default:
			var tree any
			if err := marshal.Unmarshal(fs[file].Data, marshal.YAML, &tree); err != nil {
				t.Fatal(err)
			}
			js, _ := json.Marshal(tree)
			if od, nd := decoded(t, js), decoded(t, n.Data()); !reflect.DeepEqual(od, nd) {
				t.Errorf("%s: data %v, want the YAML's tree %v", uri, nd, od)
			}
			od := decoded(t, js)
			if nm.Name != od["name"] || nm.Description != od["description"] {
				t.Errorf("%s: meta %q/%q, want the data's %v/%v", uri, nm.Name, nm.Description, od["name"], od["description"])
			}
		}
	}

	if errs := decklint.CheckPack(fs, "pack"); len(errs) != 0 {
		t.Errorf("a sound pack was refused: %v", errs)
	}
	fs["pack/deck-themes/brand.yaml"] = &fstest.MapFile{Data: []byte(strings.Replace(soundTheme, "truck:", "Camion:", 1))}
	fs["pack/deck-assets/brand/stray.png"] = &fstest.MapFile{Data: []byte("x")}
	var msgs []string
	for _, e := range decklint.CheckPack(fs, "pack") {
		msgs = append(msgs, e.Error())
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{`icon "Camion" is not kebab-case`, "brand/stray.png: shipped but not named"} {
		if !strings.Contains(all, want) {
			t.Errorf("the lint did not report %q; got:\n%s", want, all)
		}
	}
}
