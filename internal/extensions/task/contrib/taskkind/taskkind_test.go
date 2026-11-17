// SPDX-License-Identifier: Apache-2.0

package taskkind_test

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/contrib/taskkind"
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
		point, builtin.ContributeLeafKinds(provider, taskkind.Leaf()))
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		cs, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Type == contribapi.ResourceTypeContributionType && c.ID == "task" {
				return c
			}
		}
	}
	t.Fatalf("%s:/contribution-type/task not published", provider)
	return contribapi.Contribution{}
}

// systemKind is the system declaration of kind as the system set carries it:
// the companion kind a task declaration names (data-type), which Check needs
// declared to judge the task declaration alone.
func systemKind(t *testing.T, kind string) contribapi.Contribution {
	t.Helper()
	entries, err := contribution.SystemKindEntries()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ID == kind {
			return e
		}
	}
	t.Fatalf("the system set declares no %s kind", kind)
	return contribapi.Contribution{}
}

// The task pack's declaration is the kind's one home: two binaries carrying
// the pack declare it alike (the provider-neutral declaration hash), so the
// universe holds one kind - compared through Kinds.Check, where two
// declarations of one kind that differ are a kind-duplicate finding. Its
// schema keeps the format the hand-written system one had (2.1).
func TestLeafDeclaresTheTaskKindOnceAcrossBinaries(t *testing.T) {
	k := contribapi.KindsFrom([]contribapi.Contribution{published(t, "ktw-a"), published(t, "ktw-b"),
		systemKind(t, contribapi.ResourceTypeDataType)})
	if got := k.Declarers("task"); len(got) != 2 {
		t.Fatalf("declarers %v, want both", got)
	}
	for _, f := range k.Check() {
		if f.Kind == "task" {
			t.Errorf("two binaries' declarations of the kind differ: %s", f)
		}
	}
	doc, err := taskkind.Leaf().Schema.Document()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := contribapi.DataTypeFormat(doc), "2.1"; got == "" || got != want {
		t.Errorf("leaf schema format %q, want the system one %q", got, want)
	}
}
