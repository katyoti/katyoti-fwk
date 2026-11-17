// SPDX-License-Identifier: Apache-2.0

// Package local publishes the ktw-gocode services (GocodeService +
// GoToolService) as L4 services via appcore.PublishService. KTSP +
// HTTP capabilities are auto-derived from the api.*ServiceDef
// declarations.
//
// The single public surface is Extension(). Consumers (the
// ktw-gocode sidecar binary at app/ktw-gocode/, or any binary that
// wants to bundle gocode in-process) add it to their application
// via application.Extension(...).
package local

import (
	stdctx "context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
)

// Default HTTP ports — adjacent to the existing sidecar port range
// (cwm: 8895, kb: 8896, docs: 8890). Pick 8897 / 8898 to avoid
// collisions. Overridable via Options.
const (
	defaultGocodeHTTPPort = 8897
	defaultGoToolHTTPPort = 8898
)

// Options configures Extension at build time.
type Options struct {
	// GocodeHTTPPort overrides defaultGocodeHTTPPort. 0 → use default.
	GocodeHTTPPort int

	// GoToolHTTPPort overrides defaultGoToolHTTPPort. 0 → use default.
	GoToolHTTPPort int
}

// Extension declares both services with default options.
func Extension() application.Extension {
	return ExtensionWith(Options{})
}

// ExtensionWith is the customizable variant for callers that need
// to override ports (containerised deployments / multi-instance).
func ExtensionWith(opts Options) application.Extension {
	gocodePort := opts.GocodeHTTPPort
	if gocodePort <= 0 {
		gocodePort = defaultGocodeHTTPPort
	}
	goToolPort := opts.GoToolHTTPPort
	if goToolPort <= 0 {
		goToolPort = defaultGoToolHTTPPort
	}
	return func(init *application.ApplicationInit) {
		// GocodeService — orchestration + items + tasktest verbs.
		init.AddExtension(appcore.PublishService[api.GocodeService, core.Config](
			func(ctx stdctx.Context, cfg *core.Config) (api.GocodeService, error) {
				return core.NewGocodeService(ctx, cfg)
			},
			appcore.WithServiceDef(api.GocodeServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(gocodePort),
			appcore.WithConfigDirs("config"),
		))

		// GoToolService — pure Go-toolchain wrappers. Reads GocodeService.yaml,
		// the file GocodeService declares (no file of its own).
		init.AddExtension(appcore.PublishService[api.GoToolService, core.Config](
			func(ctx stdctx.Context, _ *core.Config) (api.GoToolService, error) {
				shared, err := application.UseConfig[core.GocodeServiceConfig](ctx)
				if err != nil {
					return nil, err
				}
				cfg := core.Config(shared)
				return core.NewGoToolService(ctx, &cfg)
			},
			appcore.WithServiceDef(api.GoToolServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(goToolPort),
			appcore.WithConfigDirs("config"),
			appcore.WithoutConfigFile(),
		))

		// PackageFactsService — the derived store of the Go packages' facts.
		init.AddExtension(publish.PublishSimpleService[api.PackageFactsService](
			func(stdctx.Context) (api.PackageFactsService, error) { return core.NewPackageFacts(), nil },
			publish.WithServiceDef(api.PackageFactsServiceDef),
			publish.WithKTSPContainer(),
		))

		// LanguagePack — KTSP slot (SlotType "LanguagePack"). The LANGUAGE is
		// the provider (ktw-gocode = go), so a remote consumer (ktw-code's
		// BusLanguagePack) dispatches over the bus to
		// `ktw-gocode:/LanguagePack/<method>`. This is the sidecar-topology half
		// of "one contract, both topologies" (B7) — the goPack methods are
		// uniform ctx+serializable→serializable by design, so the
		// auto-derived KTSP container exposes them directly.
		init.AddExtension(appcore.PublishService[codeapi.LanguagePack, core.Config](
			func(_ stdctx.Context, _ *core.Config) (codeapi.LanguagePack, error) {
				return &goPack{}, nil
			},
			appcore.WithServiceDef(codeapi.LanguagePackServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithConfigDirs("config"),
			appcore.WithoutConfigFile(), // the pack reads no config
		))

		// In-process named instance "go" — kept for any binary that bundles
		// gocode and resolves the pack in-process via
		// UseNamedService[LanguagePack](ctx, "go") during coexistence. The
		// sidecar topology uses the KTSP slot above instead.
		init.AddBootstrap("gocode", ".language-pack", func(app *application.Application) error {
			application.PutService[codeapi.LanguagePack](app, "go", &goPack{})
			return nil
		})

		// Pack CONTENT (personas / skills / tasks) is NO LONGER bundled here.
		// It lives in app/ktw-gocode/embed/{personas,skills,tasks} and is
		// contributed by the ktw-gocode BINARY from its own embed (see
		// app/ktw-gocode/cmd/bootstrap.go); the in-process CWM host (ktw-agents)
		// consumes it from the aggregate catalog. gocodelocal.Extension now ships
		// only BEHAVIOR (the two services + the LanguagePack), so a binary that
		// bundles it for behavior does NOT also duplicate the content
		// (migrate-pack-content-to-sidecars).
	}
}
