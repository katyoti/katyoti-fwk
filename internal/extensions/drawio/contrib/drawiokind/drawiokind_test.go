// SPDX-License-Identifier: Apache-2.0

package drawiokind_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/contrib/drawiokind"
)

// published are the leaves' contribution-types as a binary publishes them
// under provider (builtin.ContributeLeafKinds), not rebuilt by hand.
func published(t *testing.T, provider string) contribapi.Kinds {
	t.Helper()
	point := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		point, builtin.ContributeLeafKinds(provider, drawiokind.Leaves()...))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	var entries []contribapi.Contribution
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Type == contribapi.ResourceTypeContributionType {
				entries = append(entries, c)
			}
		}
	}
	return contribapi.KindsFrom(entries)
}

// The drawio leaf declares notation, viewpoint and drawio-template, each
// with its pack folder, the schema provider-relative
// (`<provider>:/data-type/<kind>`) and a data-type rendered from its Go body;
// the declarations have no finding.
func TestDrawioLeafDeclaresTheDrawioKinds(t *testing.T) {
	kinds := published(t, "ktw-docs")
	folders := map[string]string{"notation": "notations", "viewpoint": "viewpoints", "drawio-template": "drawio-templates"}
	if len(drawiokind.Leaves()) != len(folders) {
		t.Fatalf("%d leaves, want %d", len(drawiokind.Leaves()), len(folders))
	}
	for _, l := range drawiokind.Leaves() {
		got, ok := kinds.Get(l.Kind)
		if !ok {
			t.Errorf("kind %s: not published by the leaf", l.Kind)
			continue
		}
		if s := "ktw-docs:/data-type/" + l.Kind; got.Schema != s {
			t.Errorf("kind %s: schema %q, want %q", l.Kind, got.Schema, s)
		}
		if got.Source == nil || got.SourceFolder() != folders[l.Kind] {
			t.Errorf("kind %s: source folder %q, want %q", l.Kind, got.SourceFolder(), folders[l.Kind])
		}
		doc, err := l.Schema.Document()
		if err != nil {
			t.Fatalf("%s: %v", l.Kind, err)
		}
		var rendered map[string]any
		if err := json.Unmarshal(doc, &rendered); err != nil || len(rendered) == 0 {
			t.Errorf("data-type %s: no schema rendered from its Go body (%v)", l.Kind, err)
		}
	}
	for _, f := range kinds.Check() {
		t.Errorf("the leaf's declarations have a finding: %s", f)
	}
}
