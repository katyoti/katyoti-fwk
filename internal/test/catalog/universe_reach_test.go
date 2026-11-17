// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	manifestuniverse "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/universe"
	"github.com/spf13/cobra"
)

// universe_reach_test.go — a binary that is not the host obtains the universe
// view, using only what any sidecar wires.
//
// The point is the absence: nothing from app/ktw-agents appears below. Before
// c13 the universe view was assembled by that binary's bootstrap and reachable
// nowhere else, so this test could not have been written.

// pureSidecarApp builds an application wired the way a pure sidecar is: the
// manifest extension plus universe reach, and nothing host-specific.
func pureSidecarApp(t *testing.T) context.Context {
	t.Helper()
	// Isolate the environment before construction: an application reads it
	// while being built, and a universe query must not reach the developer's
	// own running hub and make the result depend on local state.
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("KTW_CACHE", t.TempDir())

	// UniverseCatalogExtension resolves this binary's identity from the cobra
	// root, exactly as the self-only catalog does.
	root := &cobra.Command{Use: "ktw-testsidecar", Version: "0.0.0-test"}
	rootExt := application.Extension(func(i *application.ApplicationInit) {
		i.AddContribute("test", ".root", func(app *application.Application) error {
			application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: root})
			return nil
		})
	})

	app := application.New(application.Config(),
		rootExt,
		// The meta-model every binary embeds identically — the thing a
		// universe query can always be asked about.
		contribution.SystemContributionsExtension(),
		manifestlocal.Extension(),
		manifestuniverse.UniverseCatalogExtension(),
	)
	if err := app.Run(); err != nil {
		t.Fatalf("run pure-sidecar application: %v", err)
	}
	return application.WithApplication(context.Background(), app)
}

// TestPureSidecar_UniverseReach asserts a non-host binary can ask for
// ReachUniverse and be answered.
//
// With no aggregator running the answer is this binary's own contributions —
// that IS the contract: an unreachable universe degrades the view, it does not
// fail the query, and a binary still starts and works.
func TestPureSidecar_UniverseReach(t *testing.T) {
	ctx := pureSidecarApp(t)

	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil {
		t.Fatalf("universe catalog not published: %v", err)
	}

	// The kind every binary embeds identically, so this is answerable with or
	// without a universe.
	got, err := mbapi.Query(ctx, mbapi.Q{
		Reach:   mbapi.ReachUniverse,
		Kind:    "contribution-type",
		Catalog: cat,
	})
	if err != nil {
		t.Fatalf("universe query from a pure sidecar: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("universe reach returned nothing; expected at least the embedded meta-model")
	}

	// Reach is still mandatory here — the surface does not become lenient
	// because the caller happens to hold a catalog.
	if _, err := mbapi.Query(ctx, mbapi.Q{Kind: "contribution-type", Catalog: cat}); err == nil {
		t.Error("a Q with no reach was accepted")
	}
}

// TestPureSidecar_SelfReachNeedsNoUniverse asserts the other reach still works
// with nothing running — the property that makes a standalone sidecar usable.
func TestPureSidecar_SelfReachNeedsNoUniverse(t *testing.T) {
	ctx := pureSidecarApp(t)
	got, err := mbapi.Query(ctx, mbapi.Q{Reach: mbapi.ReachSelf, Kind: "contribution-type"})
	if err != nil {
		t.Fatalf("self reach from a pure sidecar: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("self reach returned nothing; expected the embedded meta-model")
	}
}
