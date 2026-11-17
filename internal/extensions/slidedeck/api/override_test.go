// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// The deck readers go through the catalog lanes, so an override entry the
// catalog applies (contribapi.ApplyOverride, as the hub's catalog does for a
// project's override) is what the deck reads, lints and renders - never the
// producer's embedded content.

// laneCatalog is a universe catalog serving fixed entries on the three lanes.
type laneCatalog struct {
	entries []contribapi.Contribution
}

func (c laneCatalog) List(context.Context) ([]*mbapi.Manifest, error) { return nil, nil }

func (c laneCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, e := range c.entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

func (c laneCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	for _, e := range c.entries {
		if e.ResolvedURI() == uri {
			return e.Data(), nil
		}
	}
	return nil, errors.New("no body")
}

func (c laneCatalog) OpenLayer(_ context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	for _, e := range c.entries {
		if e.ResolvedURI() == uri {
			ref, ok := e.Layers[layer]
			if !ok {
				return nil, contribapi.LayerRef{}, errors.New("no such layer")
			}
			inline := ref.Inline
			ref.Inline = nil
			return io.NopCloser(bytes.NewReader(inline)), ref, nil
		}
	}
	return nil, contribapi.LayerRef{}, errors.New("no such entry")
}

const exampleProvider = "ktw-example"

func layoutEntry(provider string, l LayoutBody) contribapi.Contribution {
	raw, _ := json.Marshal(l)
	return contribapi.Contribution{Provider: provider, Type: ContributionTypeDeckLayout, ID: l.Name, Layers: contribapi.InlineData(raw)}
}

func assetEntry(provider, id string, content string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: ContributionTypeDeckAsset, ID: id}
	c.SetSource([]byte(content), "text/plain")
	return c
}

// overridden is base served with ov's layers, as the catalog applies it.
func overridden(t *testing.T, base, ov contribapi.Contribution) contribapi.Contribution {
	t.Helper()
	out, record, err := contribapi.ApplyOverride(base, contribapi.OverrideEntry{
		Overrides: base.ResolvedURI(), Entry: ov, Origin: contribapi.ProjectOrigin("acme-project"),
	})
	if err != nil || len(record) == 0 {
		t.Fatalf("ApplyOverride: %v (record %v)", err, record)
	}
	return out
}

var (
	baseSplit = LayoutBody{Name: "split", Description: "Two columns", Slots: []LayoutSlot{{Name: "left"}}}
	ownSplit  = LayoutBody{Name: "split", Description: "Two columns, the project's way", Slots: []LayoutSlot{{Name: "left"}, {Name: "right"}}}
	acme      = ThemeBody{Name: "acme", Description: "A theme", Notices: []string{"OFL.txt"}}
)

// overrideUniverse is a universe whose split layout and acme theme file are
// overridden by a project.
func overrideUniverse(t *testing.T) mbapi.Q {
	t.Helper()
	theme, _ := json.Marshal(acme)
	return mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: laneCatalog{entries: []contribapi.Contribution{
		overridden(t, layoutEntry(exampleProvider, baseSplit), layoutEntry("project-acme", ownSplit)),
		{Provider: exampleProvider, Type: ContributionTypeDeckTheme, ID: "acme", Layers: contribapi.InlineData(theme)},
		overridden(t, assetEntry(exampleProvider, "acme/OFL.txt", "embedded licence"), assetEntry("project-acme", "acme/OFL.txt", "project licence")),
	}}}
}

func TestLayoutsReadTheOverride(t *testing.T) {
	ctx := context.Background()
	q := overrideUniverse(t)
	layouts, err := Layouts(ctx, q)
	if err != nil || len(layouts) != 1 || layouts[0].Description != ownSplit.Description || len(layouts[0].Slots) != 2 {
		t.Fatalf("Layouts = %+v, %v; want the overridden split", layouts, err)
	}
	g, err := GrammarAt(ctx, q)
	if err != nil || len(g.Layouts) != 1 || g.Layouts[0].Description != ownSplit.Description {
		t.Fatalf("GrammarAt layouts = %+v, %v; want the overridden split", g.Layouts, err)
	}
}

func TestResolveThemeReadsTheOverriddenAsset(t *testing.T) {
	r, err := ResolveTheme(context.Background(), overrideUniverse(t), "acme")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(r.Assets["OFL.txt"]); got != "project licence" {
		t.Fatalf("asset OFL.txt = %q, want the project's override", got)
	}
}

func TestLintAcceptsASlotOnlyTheOverrideDeclares(t *testing.T) {
	md := "<!-- slide: layout=split -->\n::left::\na\n\n::right::\nb\n"
	unknown := func(g Grammar) bool {
		for _, f := range Lint(md, g) {
			if f.Rule == "slot-unknown" {
				return true
			}
		}
		return false
	}
	g, err := GrammarAt(context.Background(), overrideUniverse(t))
	if err != nil {
		t.Fatal(err)
	}
	if unknown(g) {
		t.Fatalf("lint refused the override's slot: %+v", Lint(md, g))
	}
	// The same deck against the producer's own layout names the slot unknown:
	// the override is what made it pass.
	if !unknown(Grammar{Layouts: []LayoutBody{baseSplit}}) {
		t.Fatal("lint accepted slot right against the embedded layout")
	}
}

// selfLayouts is the host's own ResourceService, serving the embedded split.
type selfLayouts struct {
	mbapi.ResourceService
	entries []contribapi.Contribution
}

func (s selfLayouts) List(_ context.Context, in mbapi.ContributionListInput) (mbapi.ContributionListOutput, error) {
	var out []contribapi.Contribution
	for _, e := range s.entries {
		if len(in.Filter.Types) == 0 || in.Filter.Types[0] == e.Type {
			out = append(out, e)
		}
	}
	return mbapi.ContributionListOutput{Contributions: out}, nil
}

func TestLayoutsInReachPrefersTheUniverse(t *testing.T) {
	app := application.New(application.Config())
	application.PutService[mbapi.ResourceService](app, "default", selfLayouts{entries: []contribapi.Contribution{layoutEntry(exampleProvider, baseSplit)}})
	ctx := application.WithApplication(context.Background(), app)

	got := LayoutsInReach(ctx, overrideUniverse(t))
	if len(got) != 1 || got[0].Description != ownSplit.Description {
		t.Fatalf("LayoutsInReach = %+v; want the universe's overridden split", got)
	}
	// A universe serving no layout falls back to the host's own.
	got = LayoutsInReach(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: laneCatalog{}})
	if len(got) != 1 || got[0].Description != baseSplit.Description {
		t.Fatalf("LayoutsInReach on an empty universe = %+v; want the host's own split", got)
	}
}
