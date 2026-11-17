// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"

	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/api"
	sccmds "github.com/katyoti/katyoti-fwk/internal/appcore/sidecar/commands"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// SidecarExt returns an Extension that publishes the sidecar contract
// services: ConfigService (config lifecycle) and LLMService (LLM-facing interface).
//
// Place immediately after NewAppExt(...) in buildExtensions. One line per sidecar.
//
// The services are published with WithKTSPContainer() so they get
// KTSP methods and HTTP endpoints. CLI commands are grouped manually.
//
// See: modules/sidecars/tools.md §1
func SidecarExt(efs fs.FS) application.Extension {
	return func(init *application.ApplicationInit) {
		// --- Phase D-5 wiring: every sidecar embeds the same system
		// contributions (contribution-type / data-type / service
		// definitions) AND publishes typed Producers for its own
		// services and method I/O Go types. Without these three
		// extensions, `<binary>:resources/list` only shows
		// hand-contributed entries — no system bootstrap, no
		// auto-derived interfaces or data-types from PublishedServices.
		//
		// Provider "" defers to app.Provider() at AppInit, which
		// appcore.NewAppExt populates from cfg.AppName — so each
		// sidecar's Contributions are stamped with its own provider
		// name automatically.
		init.AddExtension(contribution.SystemContributionsExtension())
		init.AddExtension(publish.ServicesContributionsExtension(""))

		// --- Phase E-4 wiring: emit ONE `Type: "sidecar-info"`
		// Contribution per binary describing its identity (name +
		// version + description). Body shape is locked by
		// `system:/data-type/sidecar-info`; the contribution-type
		// registration is at `system:/contribution-type/sidecar-info`
		// — both ship in the embedded system layer.
		//
		// Catalog callers cache `<binary>:/sidecar-info/default` to
		// answer identity queries without round-tripping the binary's
		// live `Sidecar.Info` slot. The live slot still exists for the
		// dynamic state (current service list, tool/key health, skill
		// enumeration, capabilities) which can't be content-addressed.
		init.AddAppInit("contribution", ".sidecar-info.provider", func(app *application.Application) error {
			provider := app.Provider()
			if provider == "" {
				return fmt.Errorf("appcore.SidecarExt: app.Provider() empty — cannot register sidecar-info producer")
			}
			// Snapshot identity from the cobra root NOW so Enumerate can
			// produce the Contribution without re-resolving services. The
			// manifest Catalog SimpleService factory calls Enumerate while
			// holding the ServiceManager mutex — any `UseService` /
			// `UseNamedService` call inside Enumerate self-deadlocks.
			cr, err := application.AppUseNamedService[CobraCommand](app, "root")
			if err != nil {
				return fmt.Errorf("appcore.SidecarExt: resolve cobra root for sidecar-info: %w", err)
			}
			version := cr.Cmd.Version
			if version == "" {
				version = "dev"
			}
			sourceLang, err := packSourceLang(efs)
			if err != nil {
				return fmt.Errorf("appcore.SidecarExt: %w", err)
			}
			return application.Contribute(app, contribapi.ContributionProvider(&sidecarInfoProvider{
				provider:    provider,
				name:        cr.Cmd.Use,
				version:     version,
				description: cr.Cmd.Short,
				sourceLang:  sourceLang,
			}))
		})

		// --- Tool declarations: a program's install recipes are refused at
		// boot when an installer engine could not run them (unknown kind or
		// platform, a download without sha256, …) — the binary does not start
		// with a declaration its describe and config check would carry wrong.
		init.AddAppInit("sidecar", ".tools.validate", func(app *application.Application) error {
			for _, ps := range application.UseAppExtensions[publish.PublishedService](app) {
				if err := publish.ValidateTools(ps.Tools); err != nil {
					return fmt.Errorf("appcore.SidecarExt: invalid program declaration: %w", err)
				}
			}
			return nil
		})

		// --- Sidecar service (identity + Info introspection) ---
		// Phase E §6 scope review (2026-06-05): WithLocal() — verified by
		// codebase search that NO consumer calls `<provider>:sidecar/info`
		// over the bus. The only consumer is `<binary> llm manifest` which
		// resolves the service in-process via `application.UseService[api.Sidecar](ctx)`.
		// For cross-binary identity, ktw-hub catalog refresh and remote
		// agents use the static `<provider>:/sidecar-info/default`
		// Contribution (Phase E-4) — cacheable, content-addressable. The
		// live Info() method stays for the in-process `llm manifest`
		// composition; the bus surface is gone.
		init.AddExtension(PublishSimpleService[api.Sidecar](
			func(ctx context.Context) (api.Sidecar, error) {
				cr, err := application.UseNamedService[CobraCommand](ctx, "root")
				if err != nil {
					return nil, fmt.Errorf("sidecar: %w", err)
				}
				version := cr.Cmd.Version
				if version == "" {
					version = "dev"
				}
				return sidecar.NewSidecar(cr.Cmd.Use, version, cr.Cmd.Short), nil
			},
			WithServiceDef(api.SidecarServiceDef),
			WithLocal(),
		))

		// --- ConfigService (config lifecycle — 5 methods) ---
		// Bus-exposed by intent: today's caller graph has NO remote
		// consumers (codebase search confirmed), but a centralized
		// "config skill" surface is on the roadmap — a future ktw-hub
		// or admin agent that drives `<sidecar>:config/check|get|set|
		// setup|validate` over the bus. Keeping the slot wired now
		// (and bearing the catalog noise) is cheaper than introducing
		// a flag day later when that consumer ships. CLI suppressed —
		// contract requires the grouped `config <verb>` even on
		// flat-CLI sidecars; grouped CLI command added manually below.
		init.AddExtension(PublishSimpleService[api.SidecarConfigService](
			func(ctx context.Context) (api.SidecarConfigService, error) {
				app := application.AppFromContext(ctx)
				if app == nil {
					return nil, fmt.Errorf("config: no Application in context")
				}
				cr, err := application.UseNamedService[CobraCommand](ctx, "root")
				if err != nil {
					return nil, fmt.Errorf("config: %w", err)
				}
				return sidecar.NewConfigService(app, cr.Cmd.Use, efs), nil
			},
			WithServiceDef(api.SidecarConfigServiceDef),
			WithKTSPContainer(),
			WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))

		// --- LLMService (LLM-facing facade) ---
		// Phase E §6 scope review: WithLocal() — codebase search confirms
		// NO consumer calls `<provider>:llm/*` over the bus. The only
		// consumer is the in-process `<binary> llm manifest` command which
		// resolves it via `application.UseService[api.LLMService](ctx)`.
		// My prior claim that ktw-hub's MCP bridge consumed llm/info over
		// the bus was speculation — the bridge only forwards slots tagged
		// with `TagSlotMCPPrefix`, which LLMService isn't. Remote LLM
		// agents already drive sidecars via `<provider>:resources/*`
		// (which IS bus-exposed) — no second LLM-driving surface needed.
		init.AddExtension(PublishSimpleService[api.LLMService](
			func(ctx context.Context) (api.LLMService, error) {
				cr, err := application.UseNamedService[CobraCommand](ctx, "root")
				if err != nil {
					return nil, fmt.Errorf("llm: %w", err)
				}
				return sidecar.NewLLMServiceFromContext(ctx, cr.Cmd.Use, "cli"), nil
			},
			WithServiceDef(api.LLMServiceDef),
			WithLocal(),
		))

		// --- Grouped CLI commands ---
		// `config` lands under the `sc` cobra group from
		// SidecarGroupsExt (per the command-surface decision — three
		// command families: domain / sc / llm).
		// The `llm` subtree is wired by the standalone `llmlocal.Extension()`
		// — every sidecar imports it in buildExtensions alongside SidecarExt.
		// `server` is hoisted from root to `sc` here too (NewAppExt's
		// bootstrap fires BEFORE SidecarGroupsExt's, so it has to land at
		// root first; we relocate it now that the `sc` group exists).
		// `sidecar info` is API-only (KTSP/HTTP) — no CLI command.
		init.AddBootstrap("sidecar", ".cli", func(app *application.Application) error {
			scGrp, err := application.AppUseNamedService[CobraCommand](app, GroupNameSidecar)
			if err != nil {
				return nil // no sc group — SidecarGroupsExt may not be wired yet on this binary
			}
			scGrp.Cmd.AddCommand(sccmds.NewConfigCommand())

			// Promote framework commands from root → `sc`. NewAppExt's
			// bootstrap (which fires BEFORE SidecarGroupsExt's) attaches
			// `server`, `dirs`, `ktsp`, `extensions` at root because the
			// `sc` group didn't exist at that point. Relocate them here
			// — the canonical layout has only domain / sc / llm at root
			// (plus framework-reserved `version` / `completion` / `help`).
			//
			// `ktsp` and `extensions` are transport/discovery entry
			// points spawned programmatically by ktw-hub, ktw-gui,
			// connmgr, and the orchestrator's `ktransport.StartStdio`.
			// Every consumer must therefore pass `"sc", "ktsp", "stdio"`
			// (or `"sc", "ktsp", "nats"`) instead of the old
			// `"ktsp", "stdio"` form. Consumers were updated in the
			// same G3 follow-up that landed this promote.
			cr, err := application.AppUseNamedService[CobraCommand](app, "root")
			if err == nil {
				promote := map[string]bool{
					"server":     true,
					"dirs":       true,
					"ktsp":       true,
					"extensions": true,
				}
				// Snapshot first — cobra's Commands() returns a slice that
				// reflects live mutations, and removing during iteration
				// would skip entries.
				children := append([]*cobra.Command(nil), cr.Cmd.Commands()...)
				for _, child := range children {
					if promote[child.Use] {
						cr.Cmd.RemoveCommand(child)
						scGrp.Cmd.AddCommand(child)
					}
				}
			}
			return nil
		})

		// --- Help text: append invitation to root cmd.Long ---
		init.AddBootstrap("sidecar", ".help", func(app *application.Application) error {
			cr, err := application.AppUseNamedService[CobraCommand](app, "root")
			if err != nil {
				return nil
			}
			binName := cr.Cmd.Use
			// LLM banner goes FIRST so an agent reading --help acts on it
			// before scanning the verb list. Appending it at the end let
			// LLMs improvise from the verbs and skip `llm info` entirely.
			banner := fmt.Sprintf("► LLM START HERE: run `%s llm info` before anything else.\n  It returns this binary's tools, resources, and skills in one call.\n  Do NOT explore subcommands manually until you have read it.\n", binName)
			footer := fmt.Sprintf("\nConfiguration: run `%s sc config check` to verify dependencies.", binName)
			body := cr.Cmd.Long
			if body == "" {
				body = cr.Cmd.Short
			}
			cr.Cmd.Long = banner + "\n" + body + footer
			return nil
		})
	}
}

// sidecarInfoProvider emits ONE `Type: "sidecar-info"` Contribution per
// binary at `<provider>:/sidecar-info/default`, carrying the stable
// identity payload (name, version, description). Body shape is locked
// by `system:/data-type/sidecar-info` — the same identity fields the
// live `Sidecar.Info` slot returns at the top of its InfoReport.
//
// Identity is read from the cobra root at Enumerate time so a binary
// that mutates its root.Use / root.Version mid-init still produces a
// fresh payload. The provider is the binary's self-provider, resolved
// at AppInit time.
//
// Per contribution-meta-model.md §4 (Phase E-4 addendum): the static
// identity Contribution and the dynamic Sidecar.Info slot serve
// complementary roles — content-addressable identity for catalogs, vs.
// fresh mutable-state snapshot for live introspection.
type sidecarInfoProvider struct {
	provider    string
	name        string
	version     string
	description string
	sourceLang  string // the pack's source_lang (embed/pack.yaml); "" = English
	epoch       contribapi.ImmutableEpoch
}

// PackFile is a pack's own declaration in its embedded FS: what holds for
// every contribution the binary publishes. Today one field, `source_lang` —
// the language the pack's texts are written in when it is not English (a
// customer pack authored in French ships `en:` translations instead of `fr:`).
const PackFile = "embed/pack.yaml"

// packSourceLang reads the pack's source_lang; "" when the pack declares
// none (English) or ships no pack file.
func packSourceLang(efs fs.FS) (string, error) {
	if efs == nil {
		return "", nil
	}
	raw, err := fs.ReadFile(efs, PackFile)
	if err != nil {
		return "", nil
	}
	var pack struct {
		SourceLang string `yaml:"source_lang"`
	}
	if err := yaml.Unmarshal(raw, &pack); err != nil {
		return "", fmt.Errorf("%s: %w", PackFile, err)
	}
	if pack.SourceLang != "" && !contribapi.IsLanguageCode(pack.SourceLang) {
		return "", fmt.Errorf("%s: source_lang %q is not a two-letter language code", PackFile, pack.SourceLang)
	}
	return pack.SourceLang, nil
}

const sidecarInfoID = "default"

func (p *sidecarInfoProvider) Type() string     { return "sidecar-info" }
func (p *sidecarInfoProvider) Provider() string { return p.provider }

func (p *sidecarInfoProvider) Enumerate(_ context.Context) ([]contribapi.Contribution, error) {
	body, err := p.identityBody()
	if err != nil {
		return nil, err
	}
	c := contribapi.Contribution{
		Provider: p.provider,
		Type:     "sidecar-info",
		ID:       sidecarInfoID,
	}
	c.SetData(body)
	return []contribapi.Contribution{c}, nil
}

// Open implements ContributionProvider.Open from this provider's own
// enumeration — one answer, the entry Enumerate already carries.
func (p *sidecarInfoProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// identityBody emits the canonical {name, version, description} payload
// as JSON from the cached identity snapshot. Caching at registration
// time avoids re-entering `ServiceManager` mid-build (`Enumerate` is
// called from inside the manifest Catalog SimpleService factory while
// the SM mutex is held).
func (p *sidecarInfoProvider) identityBody() (json.RawMessage, error) {
	payload := struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		SourceLang  string `json:"sourceLang,omitempty"`
	}{
		Name:        p.name,
		Version:     p.version,
		Description: p.description,
		SourceLang:  p.sourceLang,
	}
	return json.Marshal(payload)
}

// Identity: the binary's own name, version and description — fixed for the
// process, so immutable, named by its enumeration digest (computed once).
func (p *sidecarInfoProvider) Identity() contribapi.Identity { return p.epoch.Identity(p) }
