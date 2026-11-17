// SPDX-License-Identifier: Apache-2.0

// Package guide proves, in-process, that a sidecar ships guide recipes with
// one builder option and no guide code (modules/guide goal G-003). It is its
// own package because an application registers into process-wide extension
// points: one application per test binary.
package guide

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	contentlint "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/lint"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

//go:embed embed
var fixtureFS embed.FS

const provider = "guide-fixture"

// TestGuideRecipeFromAnotherSidecar is the G-003 probe of modules/guide. A
// sidecar that ships embed/guide/*.yaml (+ an optional .md) with
// WithGuideRecipes publishes exactly those files as entries of the SYSTEM
// guide-recipe kind — identity = file name, meta derived from the data layer,
// the .md as the instructions layer — and every entry belongs to that
// sidecar, so a universe without it lists none. The manifest passes the same
// validation and content lint the census applies to every binary.
func TestGuideRecipeFromAnotherSidecar(t *testing.T) {
	census.Isolate(t)
	sub, err := fs.Sub(fixtureFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	app := application.New(application.Config(), sidecarkit.NewSidecar(appcore.AppConfig{
		AppName: provider, Version: "0.0.1", Short: "guide fixture", EmbeddedFS: sub,
	})...) // its embed/guide is part of its pack
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	ctx := application.WithApplication(context.Background(), app)
	m, err := manifestlocal.NewBuilder(provider, "0.0.1").Build(ctx)
	if err != nil {
		t.Fatal(err)
	}

	recipes := map[string]contribapi.Contribution{}
	for _, c := range m.Contributions {
		if c.Type == guideapi.Kind {
			recipes[c.ResolvedURI()] = c
		}
	}
	var got []string
	for uri := range recipes {
		got = append(got, uri)
	}
	sort.Strings(got)
	want := []string{provider + ":/guide-recipe/add-a-thing", provider + ":/guide-recipe/look-around"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("recipes published = %v, want exactly %v (the .md is a layer, not an entry)", got, want)
	}

	for uri, c := range recipes {
		if c.Provider != "" && c.Provider != provider {
			t.Errorf("%s: provider %q — a recipe belongs to the sidecar that ships it", uri, c.Provider)
		}
		var body guideapi.Recipe
		if err := json.Unmarshal(dataLayer(t, c), &body); err != nil {
			t.Fatalf("%s: data layer: %v", uri, err)
		}
		if c.Metadata.Name != body.Name || c.Metadata.Name == "" {
			t.Errorf("%s: meta name %q, data name %q — the picker title is derived from the data", uri, c.Metadata.Name, body.Name)
		}
		if len(c.Metadata.Hashtags) == 0 || c.Metadata.Hashtags[0] != body.Hashtags[0] {
			t.Errorf("%s: meta hashtags %v, data hashtags %v", uri, c.Metadata.Hashtags, body.Hashtags)
		}
	}
	thing := recipes[want[0]]
	if _, ok := thing.Layers[contribapi.LayerInstructions]; !ok {
		t.Errorf("add-a-thing: no instructions layer from add-a-thing.md: %v", thing.Layers)
	}
	if _, ok := recipes[want[1]].Layers[contribapi.LayerInstructions]; ok {
		t.Errorf("look-around has no .md, yet carries an instructions layer")
	}
	// The .i18n.yaml beside a recipe is its translations layer, not an entry
	// of its own (exactly two entries above), and only where the file exists.
	if _, ok := thing.Layers[contribapi.LayerTranslations]; !ok {
		t.Errorf("add-a-thing: no translations layer from add-a-thing.i18n.yaml: %v", thing.Layers)
	}
	if _, ok := recipes[want[1]].Layers[contribapi.LayerTranslations]; ok {
		t.Errorf("look-around has no .i18n.yaml, yet carries a translations layer")
	}
	tr, err := openLayer(t, ctx, want[0], contribapi.LayerTranslations)
	if err != nil || len(tr) == 0 {
		t.Fatalf("add-a-thing translations: %v (inline %d bytes)", err, len(tr))
	}
	parsed, err := guideapi.ParseTranslations(tr)
	if err != nil || parsed["fr"].Steps["open"] != "Ouvrez le panneau des choses." {
		t.Errorf("translations layer = %q (%v)", tr, err)
	}

	// The census's own checks: schema, required layers, derived meta, and
	// the content lint at self scope.
	census.AssertValid(t, provider, m)
	opener := func(uri, layer string) ([]byte, error) { return openLayer(t, ctx, uri, layer) }
	if errs := contentlint.Errors(contentlint.LintManifest(m, opener, contentlint.NewUniverse([]*mbapi.Manifest{m}), contentlint.ScopeSelf)); len(errs) > 0 {
		t.Errorf("content lint: %v", errs)
	}
}

func dataLayer(t *testing.T, c contribapi.Contribution) []byte {
	t.Helper()
	l, ok := c.Layers[contribapi.LayerData]
	if !ok || len(l.Inline) == 0 {
		t.Fatalf("%s: no inline data layer: %+v", c.ResolvedURI(), c.Layers)
	}
	return l.Inline
}

// openLayer reads one layer the way a reader does: through the binary's
// ResourceService (the content lane), since the manifest keeps inline only
// the layers its kind declares resident.
func openLayer(t *testing.T, ctx context.Context, uri, layer string) ([]byte, error) {
	t.Helper()
	svc, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		return nil, err
	}
	out, err := svc.OpenLayer(ctx, mbapi.OpenLayerInput{URI: uri, Layer: layer})
	if err != nil {
		return nil, err
	}
	return out.Content, nil
}
