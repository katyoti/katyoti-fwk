// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib/drawiokind"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

const (
	packNotation = `name: flow
title: Flow
elements:
  - kind: app
    label: Application
`
	packViewpoint = `name: flow-map
title: Flow map
notation: flow
template_library: flow
`
	packLibrary = `name: flow
notation: flow
templates:
  app: {template: app}
`
	packDrawio = `<mxfile><diagram name="app"/></mxfile>`
)

// The same drawio content laid out twice: as ContributeDrawio reads it under
// old/, and as folders of a pack under pack/.
func drawioFiles() fstest.MapFS {
	fs := fstest.MapFS{}
	put := func(p, s string) { fs[p] = &fstest.MapFile{Data: []byte(s)} }
	for _, root := range []string{"old/", "pack/"} {
		put(root+"notations/flow.yaml", packNotation)
		put(root+"viewpoints/flow-map.yaml", packViewpoint)
		put(root+"drawio-templates/flow.yaml", packLibrary)
		put(root+"drawio-templates/flow.drawio", packDrawio)
	}
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

// leafKinds is the system set plus the drawio kinds as a binary publishes
// them from their leaf: what a sidecar that links no drawio/contrib knows.
func leafKinds(t *testing.T) contribapi.Kinds {
	t.Helper()
	sys, err := contribution.SystemKindEntries()
	if err != nil {
		t.Fatal(err)
	}
	entries := append([]contribapi.Contribution(nil), sys...)
	for _, c := range enumerate(t, builtin.ContributeLeafKinds("p", drawiokind.Leaves()...)) {
		if c.Type == contribapi.ResourceTypeContributionType {
			entries = append(entries, c)
		}
	}
	return contribapi.KindsFrom(entries)
}

func decodedData(t *testing.T, c contribapi.Contribution) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(c.Data(), &m); err != nil {
		t.Fatalf("%s: data %s: %v", c.ResolvedURI(), c.Data(), err)
	}
	return m
}

// A pack holding drawio-templates, notations and viewpoints folders, read by
// the generic pack reader with the kinds the drawio leaf declares, publishes
// the entries ContributeDrawio publishes: the same URIs, data (the .drawio
// beside a library folded in as `drawio`) and source.
func TestDrawioLibrariesAreReadFromAPack(t *testing.T) {
	ldr := loader.NewEmbedFSLoader(drawioFiles(), ".")
	old := map[string]contribapi.Contribution{}
	for _, c := range enumerate(t, contrib.ContributeDrawio("p", ldr, "old")) {
		old[c.ResolvedURI()] = c
	}
	got := map[string]contribapi.Contribution{}
	for _, p := range manifestcontrib.PackProviders("p", ldr, "pack", leafKinds(t)) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			got[c.ResolvedURI()] = c
		}
	}

	if len(old) != 3 {
		t.Fatalf("ContributeDrawio published %d entries, want a notation, a viewpoint and a library", len(old))
	}
	for uri, o := range old {
		n, ok := got[uri]
		if !ok {
			t.Errorf("%s: published by ContributeDrawio, not read from the pack", uri)
			continue
		}
		if string(n.Source()) != string(o.Source()) {
			t.Errorf("%s: source %q, want %q", uri, n.Source(), o.Source())
		}
		od, nd := decodedData(t, o), decodedData(t, n)
		if !reflect.DeepEqual(od, nd) {
			t.Errorf("%s: data %v, want %v", uri, nd, od)
		}
		if o.Type == drawiokind.Template().Kind && od["drawio"] != packDrawio {
			t.Errorf("%s: the .drawio beside the library is not folded into its data: %v", uri, od)
		}
	}
	for uri := range got {
		if _, ok := old[uri]; !ok {
			t.Errorf("%s: read from the pack, not published by ContributeDrawio", uri)
		}
	}
}
