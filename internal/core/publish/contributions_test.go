// SPDX-License-Identifier: Apache-2.0

package publish_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/api/providertest"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// providersPointExt registers ContributionProvidersPoint at
// RegisterPoints phase — minimal stand-in for the manifest extension
// so the test doesn't drag in appcore + Cobra dependencies.
//
// In a real binary, manifestlocal.Extension() does this registration
// (see internal/extensions/manifest/local/extension.go); we replay it
// inline here purely to isolate the Producer wiring under test.
var providersPointExt application.Extension = func(init *application.ApplicationInit) {
	init.AddRegisterPoints("test", ".providers-point", func(app *application.Application) error {
		var providersPoint = em.NewListPoint[contribapi.ContributionProvider]()
		return application.RegisterPoint(app, providersPoint)
	})
}

// fooSvc + fooImpl + fooServiceDef — same fake-service shape used in
// earlier phase tests. Cheapest way to give the publish framework a
// real ServiceDef to walk.
type fooInput struct {
	Topic string `json:"topic"`
}
type fooOutput struct {
	Reply string `json:"reply"`
}
type fooSvc interface {
	DoThing(ctx context.Context, in fooInput) (fooOutput, error)
}
type fooImpl struct{}

func (fooImpl) DoThing(_ context.Context, in fooInput) (fooOutput, error) {
	return fooOutput{Reply: "did:" + in.Topic}, nil
}

var fooServiceDef = meta.ServiceDef{
	Name:    "FooService",
	Short:   "Phase-D integration test fake",
	Version: "0.0.1",
	Methods: map[string]meta.MethodDef{
		"DoThing": meta.Method("does the thing"),
	},
}

// TestPhaseD_ProducersEmitContributions exercises the full chain that
// replaces Phase B/C:
//
//  1. publish.PublishSimpleService + WithServiceDef + WithKTSPContainer
//     enrolls fooSvc into PublishedServicesPoint.
//  2. contribution.SystemContributionsExtension contributes one
//     ContributionProvider per <type> subdirectory in embed/.
//  3. publish.ServicesContributionsExtension contributes a Producer
//     that emits `Type:"service"` contributions from
//     PublishedServicesPoint.
//  4. After app.Run, the registered providers — read from EM via
//     application.UseAppExtensions[ContributionProvider] — Enumerate
//     to the expected mix of (system, service) entries.
//
// Service method I/O is NOT a separate data-type contribution: the
// method signature carries its schema inline (ktsp.Method.InputSchema/
// OutputSchema). The former eager reflected-data-type producer was
// dropped (drop-reflected-method-datatypes) as unconsumed duplication.
func TestPhaseD_ProducersEmitContributions(t *testing.T) {
	cfgSvc := config.NewConfigService(loader.NewMapLoader(nil))

	app := application.New(
		cfgSvc,
		providersPointExt, // stand-in for manifestlocal.Extension()
		publish.PublishSimpleService[fooSvc](
			func(_ context.Context) (fooSvc, error) { return fooImpl{}, nil },
			publish.WithServiceDef(fooServiceDef),
			publish.WithKTSPContainer(),
		),
		contribution.SystemContributionsExtension(),
		publish.ServicesContributionsExtension("ktw-test"),
	)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}

	providers := application.UseAppExtensions[contribapi.ContributionProvider](app)
	if len(providers) == 0 {
		t.Fatal("no ContributionProvider entries registered")
	}

	// Bucket Enumerate results by (Provider, Type) so each assertion
	// hits exactly the producer it cares about.
	type key struct{ provider, typ string }
	buckets := map[key][]contribapi.Contribution{}
	for _, p := range providers {
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatalf("Enumerate(%s:%s): %v", p.Provider(), p.Type(), err)
		}
		buckets[key{p.Provider(), p.Type()}] = append(buckets[key{p.Provider(), p.Type()}], got...)
		// Every registered producer (system + services) holds the contract.
		providertest.Run(t, p)
	}

	// (1) System layer — at least one contribution-type entry must
	// surface (the self-referential contribution-type) and one
	// data-type entry (Contribution).
	sysCT := buckets[key{contribapi.SystemProvider, "contribution-type"}]
	if len(sysCT) < 1 {
		t.Errorf("expected ≥1 system contribution-type, got %d (%v)", len(sysCT), uriSlice(sysCT))
	}
	sysDT := buckets[key{contribapi.SystemProvider, contribution.TypeDataType}]
	if len(sysDT) < 1 {
		t.Errorf("expected ≥1 system data-type, got %d (%v)", len(sysDT), uriSlice(sysDT))
	}

	// (2) Service producer — fooServiceDef must appear at URI
	// ktw-test:/service/foo-service with the def in the body.
	svcs := buckets[key{"ktw-test", "service"}]
	if len(svcs) != 1 {
		t.Fatalf("expected exactly 1 service contribution, got %d (%v)", len(svcs), uriSlice(svcs))
	}
	got := svcs[0]
	if got.ID != "foo-service" {
		t.Errorf("service ID = %q, want %q", got.ID, "foo-service")
	}
	var sd meta.ServiceDef
	if err := json.Unmarshal(got.Data(), &sd); err != nil {
		t.Errorf("service Body decode: %v", err)
	}
	if sd.Name != "FooService" {
		t.Errorf("decoded ServiceDef Name = %q, want %q", sd.Name, "FooService")
	}

	// (3) Service method I/O is carried INLINE by the method signature
	// (ktsp.Method.InputSchema/OutputSchema), not as a separate data-type
	// contribution — so NO `ktw-test:/data-type/...` entries are emitted by
	// the publish layer. Assert the absence to lock the contract.
	if dts := buckets[key{"ktw-test", "data-type"}]; len(dts) != 0 {
		t.Errorf("expected 0 publish-layer data-type contributions (method I/O is inline in the signature), got %d (%v)", len(dts), uriSlice(dts))
	}
}

// uriSlice returns just the URIs from a slice of contributions, for
// terse failure messages.
func uriSlice(cs []contribapi.Contribution) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		if c.URI != "" {
			out[i] = c.URI
		} else {
			out[i] = c.Provider + ":/" + c.Type + "/" + c.ID
		}
	}
	return out
}
