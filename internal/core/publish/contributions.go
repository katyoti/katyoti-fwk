// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// ServicesContributionsExtension returns an Extension that registers
// ONE `ContributionProvider` per binary which emits one
// `Type: "service"` contribution for every published service that
// carries a `meta.ServiceDef` (`WithServiceDef(...)` at publish time).
//
// Wire shape (per contribution-meta-model.md §3):
//
//	URI:      <provider>:/service/<bus-name>
//	Type:     service
//	Schema:   system:/data-type/service
//	Body:     meta.ServiceDef as JSON
//
// The catalog (in-process + hub-aggregated) serves these alongside
// every other contribution kind — no per-kind bus slot, no parallel
// EM point. Consumers query
// `ResourceService.List({Types: ["service"]})` and get every
// service interface the binary publishes.
//
// `provider` is the binary's self-provider name. Pass "" to defer
// resolution to `app.Provider()` at Contribute time (the normal case
// for self-publishing binaries).
func ServicesContributionsExtension(provider string) application.Extension {
	return func(init *application.ApplicationInit) {
		// AppInit (not Contribute) — appcore.NewAppExt sets the
		// self-provider during its Contribute hook; ordering within
		// Contribute is undefined, so resolving app.Provider() at
		// Contribute time races against appcore's SetProvider. AppInit
		// runs strictly after every Contribute hook completes, so the
		// provider is reliably populated.
		init.AddAppInit("contribution", ".services.provider", func(app *application.Application) error {
			resolved := provider
			if resolved == "" {
				resolved = app.Provider()
			}
			if resolved == "" {
				return fmt.Errorf("publish.ServicesContributionsExtension: provider is empty and app.Provider() also returned empty")
			}
			return application.Contribute(app, contribapi.ContributionProvider(&servicesProvider{
				app:      app,
				provider: resolved,
			}))
		})
	}
}

// servicesProvider lazily walks PublishedServicesPoint at Enumerate
// time. Stateless besides the resolved provider name + app reference
// — the underlying point is the source of truth and stays mutable
// until the manifest catalog freezes it.
type servicesProvider struct {
	app      *application.Application
	provider string
	epoch    contribapi.ImmutableEpoch
}

func (p *servicesProvider) Type() string     { return "service" }
func (p *servicesProvider) Provider() string { return p.provider }

func (p *servicesProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	entries := application.UseAppExtensions[PublishedService](p.app)
	out := make([]contribapi.Contribution, 0, len(entries))
	for _, ps := range entries {
		if ps.Definition == nil {
			// Services published without a ServiceDef (rare; mostly
			// internal infra) don't appear in the interface registry.
			continue
		}
		body, err := json.Marshal(ps.Definition)
		if err != nil {
			return nil, fmt.Errorf("services contribution %q: marshal ServiceDef: %w", ps.Name, err)
		}
		id := ps.Definition.ResolvedBusName()
		if id == "" {
			id = ps.Name
		}
		// Scope discriminator (Phase E §4.3): WithLocal()-marked services
		// emit Contributions under `Provider: "local"` so the
		// ResourceService bus-boundary filter strips them from
		// remote callers. Bus-exposed services keep the binary's
		// self-provider name.
		prov := p.provider
		if ps.Local {
			prov = contribapi.LocalProvider
		}
		c := contribapi.Contribution{
			Provider: prov,
			Type:     "service",
			ID:       id,
		}
		c.SetData(body)
		out = append(out, c)
	}
	return out, nil
}

// Open implements ContributionProvider.Open from this provider's own
// enumeration — one answer, the entry Enumerate already carries.
func (p *servicesProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// Identity: service interfaces are reflected from types compiled into the
// binary, so the provider is immutable and names its version by its
// enumeration digest (computed once).
func (p *servicesProvider) Identity() contribapi.Identity { return p.epoch.Identity(p) }
