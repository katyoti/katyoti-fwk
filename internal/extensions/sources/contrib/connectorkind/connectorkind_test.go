// SPDX-License-Identifier: Apache-2.0

package connectorkind_test

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib/connectorkind"
)

// published is the leaf's contribution-type as a binary publishes it under
// provider (builtin.ContributeLeafKinds), not rebuilt by hand.
func published(t *testing.T, provider string) contribapi.Contribution {
	t.Helper()
	point := func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
	}
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)),
		point, builtin.ContributeLeafKinds(provider, connectorkind.Leaf()))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Type == contribapi.ResourceTypeContributionType && c.ID == "source-connector" {
				return c
			}
		}
	}
	t.Fatalf("%s:/contribution-type/source-connector not published", provider)
	return contribapi.Contribution{}
}

// Every binary shipping connectors declares the kind from the one leaf: two
// declarations hash alike once their provider is set aside, so the universe
// holds one kind (no kind-duplicate finding); the kind reads its entries
// from the source-connectors folder and derives their meta from the data.
func TestLeafDeclaresTheSourceConnectorKindOnceAcrossBinaries(t *testing.T) {
	k := contribapi.KindsFrom([]contribapi.Contribution{published(t, "ktw-a"), published(t, "ktw-b")})
	if got := k.Declarers("source-connector"); len(got) != 2 {
		t.Fatalf("declarers %v, want both", got)
	}
	for _, f := range k.Check() {
		if f.Kind == "source-connector" {
			t.Errorf("two binaries' declarations of the kind differ: %s", f)
		}
	}
	d, ok := k.Get("source-connector")
	if !ok {
		t.Fatal("the kind is not declared")
	}
	if got := d.SourceFolder(); got != "source-connectors" {
		t.Errorf("source folder %q, want source-connectors", got)
	}
	if layer, fields := d.MetaFrom(); layer != "data" || len(fields) != 2 {
		t.Errorf("meta from %q %v, want the data's name and description", layer, fields)
	}
	if f := contribapi.DataTypeFormat(must(connectorkind.Leaf().Schema.Document())); f != "1.5" {
		t.Errorf("schema format %q, want 1.5 - the shape the generated data-type had", f)
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
