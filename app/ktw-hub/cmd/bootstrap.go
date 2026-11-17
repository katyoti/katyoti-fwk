// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	connmgrcommands "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/commands"
	connmgrlocal "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/local"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcommands "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/commands"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/follow"
	panelserverlocal "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/local"
	pcapi "github.com/katyoti/katyoti-fwk/internal/extensions/projectcontributions/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
	schemalocal "github.com/katyoti/katyoti-fwk/internal/extensions/schema/local"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	mbsubscribe "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	"github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/busauth"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
	"github.com/spf13/cobra"
)

// buildExtensions wires the ktw-hub binary's extensions.
//
// Phase-1 scope (this commit): start an embedded NATS server, provision
// JetStream resources, and expose the connection via the MessageBus
// interface. ConnectionManager / Catalog / Rooms / Sessions / HumanGate
// router land in subsequent ms-bus-split todos.
func buildExtensions(efs embed.FS) []application.Extension {
	return []application.Extension{
		appcore.NewAppExt(appcore.AppConfig{
			AppName:    "ktw-hub",
			Version:    version,
			Short:      "ktw-hub — KTSP bus host (embedded NATS / cluster client)",
			Long:       longHelp,
			EmbeddedFS: efs,
		}),
		// manifest extension: registers ContributionProvidersPoint + the
		// `contributions list/get` Cobra commands. ktw-hub currently has no
		// resource providers of its own (it's the bus host, not a content
		// provider) — the commands are still useful for inspection.
		// `sc` / `llm` groups: the hub answers `sc resources manifest` like
		// every sidecar, so the compiled-binary checks judge the shipped hub
		// by the same rules (e24 / N4) — and the in-process self manifest
		// above is the same descriptor that command prints.
		appcore.SidecarGroupsExt(),
		manifestlocal.Extension(),
		// services provider: what the hub offers on the bus (catalog, resources,
		// schema, conn-mgr) is published as `ktw-hub:/service/<name>` like every
		// other binary's offerings — one mechanism serves every offering,
		// including the platform's own (G21; ms-contribution-ext e17).
		// system provider: the kinds and data-types every binary carries
		// identically (the rule concepts.md states), so the hub's own service
		// entries have a declared kind like everyone else's (ms-contribution-ext
		// e15: the hub and ktw-test were the two binaries without it).
		contribution.SystemContributionsExtension(),
		// The pack kinds (skill, task, persona): the hub reads them on its
		// folder members (universeKinds) and is the universe's one carrier
		// of the kinds every binary knows, so it publishes their
		// declarations too - a universe whose only member is the hub
		// resolves them. Once per binary.
		builtin.ContributeLeafKinds("", builtin.Leaves()...),
		publish.ServicesContributionsExtension("ktw-hub"),
		// The guide's shell recipes and welcome tour (embed/guide): carried by
		// the hub because it is in every universe's view (modules/guide, OD-011).
		manifestcontrib.ContributeKindResources("ktw-hub", "guide-recipe", loader.NewEmbedFSLoader(efs, "."), "embed/guide",
			manifestcontrib.WithSiblingLayer(contribapi.LayerInstructions, ".md")),
		// schema extension: publishes the SchemaService over KTSP so
		// polyglot consumers can fetch DSL ASTs + JSON Schemas without
		// shipping a DSL parser. The hub's catalog factory populates it
		// with data-type Contributions during Phase 1 of bootstrap
		// (per the hub-boot-order ADR). MUST come BEFORE catalogExtension
		// so the service is resolvable when catalog factory fires.
		schemalocal.Extension(),
		// catalog: in-host aggregator over every sidecar's cached
		// Manifest. Reads the `sidecars:` block of Univers.yaml + the
		// tier-1 manifest cache (<CacheDir>/sidecars/) at startup; does
		// NOT spawn any sidecar.
		catalogExtension(),
		// catalog over the bus — wraps the in-process Catalog as a
		// KTSP slot so ktw-gui (and any other client) can enumerate
		// the universe's skills / tasks / resources without spawning
		// a single sidecar. The whole point of the catalog work.
		catalogServiceExtension(),
		// connmgr: per-(universe, sidecar-name) lifecycle FSM. Publishes
		// the ConnectionManager service + the `connections` Cobra group.
		// Members loaded from Univers.yaml at first-Use; spawn happens
		// lazily on the first Acquire(name) call.
		connectionManagerExtension(),
		// conn-mgr over the bus — bus-friendly EnsureSpawned method
		// that drives the spawn lifecycle from remote clients. Powers
		// the A-RETRY lazy-spawn path: client gets NoResponders â†’
		// retry wrapper calls this â†’ hub spawns the sidecar â†’ client
		// retries the original request and succeeds.
		connMgrServiceExtension(),
		// The panel server: the one writer of each display
		// panel's log and snapshot in the universe's data folder, its reads
		// a KTSP slot (`ktw-hub:panel-service/<method>`); `serve` starts it
		// on the bus.
		panelserverlocal.Extension(),
		// CWM, reached on its host (ktw-agents) over the bus: the panel
		// server writes a kept panel's outcome as a document on the object
		// its work is on.
		cwmclient.RemoteExtension(),
		busclientbus.Extension(), // the bus-backed Sender the generated client calls through
		// Auto-build the KTSP endpoint from every PublishedService
		// declared above with WithKTSPContainer (currently:
		// ResourceService, CatalogService, ConnMgrService). Must run
		// AFTER all PublishService extensions have contributed — placed
		// at the end of the list to satisfy that ordering. Without this
		// the hub's slot handlers exist in-process but have no
		// `*ktsp.Endpoint` to host them, and `mbsubscribe.Subscribe`
		// (called from `serve`) can't bind anything to the bus.
		ktslocal.AutoKTSPContainer("ktw-hub", "ktw-hub KTSP container (bus host)", version),
		hubExtension(),
	}
}

// catalogExtension publishes the Catalog service and contributes the
// `catalog` Cobra command group. Backed by the tier-1 manifest cache
// (one folder per member under `<CacheDir>/sidecars/`, see
// modules/contributions/cache-layout.md). Members are loaded once at
// first-Use from the `sidecars:` block of the universe's Univers.yaml.
//
// The factory implements the two-phase bootstrap protocol locked in
// modules/contributions/decisions/OD-006-in-what-order-does-the-hub-build-its-catalog-and.cwm.md:
//
//	Phase 1 — raw load: NewCatalog reads the cached manifests; data-type
//	  Contributions are walked and registered with the SchemaService.
//	Phase 2 — reindex:  SetSchema attaches the schema; Reindex extracts
//	  edges; MarkReady flips the ready flag.
//
// Failure mode: schema service unresolvable or data-type parse errors
// log a warning and skip; the catalog still marks ready (Phase 1 state
// is usable, just without edges).
func catalogExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[mbapi.Catalog](
			func(ctx context.Context) (mbapi.Catalog, error) {
				// Sidecar manifests are a function of the BINARY — the
				// same binary produces the same manifest in any universe
				// — so the cache lives under `<CacheDir>/sidecars/` (not
				// `<DataDir>/cache/`) and a record is keyed by member
				// name AND declared binary: universes naming the same
				// binary share one record, universes naming one member
				// at two binaries hold two. CacheDir is the XDG-style
				// `<XDG_CACHE>/katyoti/` path that's shared across
				// universes; DataDir is per-universe state.
				root := filepath.Join(application.Env().CacheDir(), "sidecars")
				cache := universeCache(root)
				reg, err := universeMembers()
				if err != nil {
					return nil, fmt.Errorf("catalog: load Univers.yaml sidecars: %w", err)
				}
				// The hub's own offerings (its `ktw-hub:/service/*` and
				// `ktw-hub:/data-type/*` entries) are a member of the view
				// like any other: recorded in the tier-1 cache under the
				// hub's name, built in process (e24 / N11).
				if err := saveHubOwnManifest(ctx, cache); err != nil {
					slog.Warn("catalog: the hub's own manifest is not in the view", "error", err)
				}
				members := catalogMembers(reg.Sidecars)
				// Phase 1 — raw load.
				cat, err := manifestlocal.NewCatalog(cache, members)
				if err != nil {
					return nil, err
				}
				// Resolve SchemaService for Phase 2. If unavailable, log
				// and mark ready anyway (Phase 1 state is the fallback).
				schemaSvc, schemaErr := application.UseService[schemaapi.SchemaService](ctx)
				if schemaErr != nil || schemaSvc == nil {
					slog.Warn("catalog: SchemaService unresolvable; marking ready in Phase 1 state",
						"error", schemaErr)
					_ = cat.MarkReady(ctx)
					return cat, nil
				}
				// Phase 2 — populate schema + reindex + mark ready.
				applyHubPhase2(ctx, cat, schemaSvc)
				return cat, nil
			},
		))
		// Build the manifest catalog group + append the hub-specific
		// subcommands:
		//   - `refresh <name>` — bridges connmgr + manifest to re-fetch
		//   - `cache list|show|invalidate|gc` — inspect + maintain
		//     the per-sidecar manifest cache at <CacheDir>/sidecars/
		catGroup := manifestcommands.NewCatalogCommand()
		catGroup.AddCommand(newCatalogRefreshCommand())
		catGroup.AddCommand(newCacheCommand())
		init.AddExtension(appcore.CommandExt(catGroup))
	}
}

// hubProvider is the hub's provider name — its AppName, the `<provider>` of
// every entry it publishes, and its member name in the catalog view.
const hubProvider = projectfile.HubToken

// catalogMembers is the catalog's membership: every declared sidecar, then the
// hub itself. The hub is not a Univers.yaml member — it is not spawned, probed
// or connection-managed — but what it offers is part of the universe view
// (G21, e24 / N11), so the catalog lists it.
//
// Each member is addressed by its name AND the binary the universe declares
// for it — the tier-1 record's key — so the view never loads another
// universe's record of a same-named member at another binary (blind audit
// H1). The hub's own binary is the running executable.
func catalogMembers(sidecars []appcore.SidecarSpec) []manifestlocal.Member {
	out := make([]manifestlocal.Member, 0, len(sidecars)+1)
	for _, s := range sidecars {
		if s.Name == hubProvider {
			continue
		}
		out = append(out, manifestlocal.Member{Name: s.Name, Binary: recordSource(s), Folder: folderOf(s)})
	}
	hubExe, _ := hubOwnExecutable() // "" → no record; saveHubOwnManifest reports the error
	return append(out, manifestlocal.Member{Name: hubProvider, Binary: hubExe})
}

// memberNames projects a membership to its names — the GC allowlist and
// log lines.
func memberNames(members []manifestlocal.Member) []string {
	out := make([]string, 0, len(members))
	for _, m := range members {
		out = append(out, m.Name)
	}
	return out
}

// saveHubOwnManifest records the hub's own manifest in the tier-1 cache under
// the hub's name, so the catalog loads it like any member's (recovery,
// digests, `cache list` work the same).
//
// Built in process, through the ResourceService every binary answers
// `sc resources manifest` with — the same descriptor an exec probe of the hub
// would return, without the exec. It is the universe's ONE carrier of the
// system set (`system:/contribution-type/*`, `system:/data-type/*`, e26): the
// hub embeds it (contribution.SystemContributionsExtension) and is a member of
// every universe's view, while every other member is probed without it. So no
// member record depends on the universe that probed it, and a universe with no
// binary member still has the system kinds. The record is rewritten only when
// the manifest or its key moved, so a CLI invocation does not churn the cache.
func saveHubOwnManifest(ctx context.Context, cache *manifestlocal.Cache) error {
	want := sourceDigestsFor(hubProvider)
	m, err := hubOwnManifest(ctx, want.IncludeSystem)
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}
	if m == nil {
		return fmt.Errorf("build: nil manifest")
	}
	exe, err := hubOwnExecutable()
	if err != nil {
		return fmt.Errorf("own executable: %w", err)
	}
	// Load and IsStale both refuse a record of another binary; the key is
	// the running executable, as it is for every member's declared binary.
	if cached, meta, lerr := cache.Load(hubProvider, exe); lerr == nil && cached != nil && meta != nil &&
		cached.Hash == m.Hash {
		if stale, _ := cache.IsStale(meta, want); !stale {
			return nil
		}
	}
	return cache.Save(hubProvider, m, exe, want)
}

// hubOwnManifest builds the hub's own manifest in process through its
// ResourceService, with or without the system set. A package variable so the
// system-once tests can hand saveHubOwnManifest a fixture manifest and still
// exercise the real record writer (re-audit N21).
//
// The hub's record is a member record like any other, so it is asked for the
// way a member is probed (appcore.MemberManifestRequest): never with the
// hub's own `local:` entries, which are binary-private and belong in no
// aggregate (blind audit M3).
var hubOwnManifest = func(ctx context.Context, includeSystem bool) (*mbapi.Manifest, error) {
	svc, err := application.UseService[mbapi.ResourceService](ctx)
	if err != nil {
		return nil, fmt.Errorf("resource service: %w", err)
	}
	// includeSystem true: the hub is the one carrier (carriesSystemSet).
	out, err := svc.GetManifest(ctx, appcore.MemberManifestRequest(includeSystem))
	if err != nil {
		return nil, err
	}
	return out.Manifest, nil
}

// hubOwnExecutable is os.Executable behind a seam, for the same tests.
var hubOwnExecutable = os.Executable

// Phase 2 resolves a data-type's DSL from the catalog and NOWHERE ELSE.
//
// A fallback that fetched the content from the owning sidecar was tried here
// and removed: Phase 2 runs INSIDE the catalog's service factory, so reaching
// the ConnectionManager to spawn a sidecar makes the hub wait, during boot, on
// a process that needs the catalog it is still building. The hub simply never
// came up.
//
// The content has to already be here, which is what the kind's residency
// declaration is for (the kind's `layers.source.resident`, applied by the manifest builder). That is where
// the real bug was: the inline band silently revoked the declaration above
// 4 KiB, so an 18 KiB schema shipped as a hash with nothing behind it. Fixed at
// the producer, where a kind says what a listing needs — not by giving the
// aggregator a boot-time back-channel.

// applyHubPhase2 runs the two-phase bootstrap's Phase 2 sequence:
// walk the catalog for data-type Contributions, register each with the
// SchemaService, attach the schema adapter to the catalog, run
// Reindex, then MarkReady.
//
// Idempotent — safe to call after any catalog Reload (cache
// watcher, post-hydrate, catalog refresh). Errors are logged and
// swallowed; the catalog never refuses to start due to one bad
// Contribution per hub-boot-order ADR §5.
func applyHubPhase2(ctx context.Context, cat mbapi.Catalog, schemaSvc schemaapi.SchemaService) {
	// Register every data-type Contribution with the SchemaService.
	dataTypes, err := cat.GetContributionsByType(ctx, "data-type")
	if err != nil {
		slog.Warn("catalog Phase 2: enumerate data-types", "error", err)
	}
	// One implementation, shared with packaging: the envelope shape has drifted
	// once already, and a second copy is a second place for it to drift again.
	// Failures are returned as well as logged; a running aggregator logs and
	// carries on, because one bad schema must not cost a universe its other two
	// hundred.
	// A data-type's DSL is its source layer, resident in tier 1 since the
	// builder applies the kind's residency — so this fallback is the exception,
	// and it reads the SOURCE layer of the entry, never the body lane, which
	// answers the data layer only.
	registered, failed := schemalocal.RegisterDataTypes(ctx, schemaSvc, dataTypes,
		func(uri string) ([]byte, error) {
			if r, ok := cat.(mbapi.ContributionResolver); ok {
				if c, found := r.ContributionAt(uri); found {
					return c.Source(), nil
				}
			}
			return nil, nil
		})
	if len(failed) > 0 {
		slog.Warn("catalog Phase 2: some data-types did not register",
			"failed", len(failed), "registered", registered)
	}
	// The registration outcome IS a data-type's verdict (concepts §11). It used
	// to be logged and discarded here, so every data-type read "unchecked" in
	// every listing and a schema that did not parse was invisible on the wire.
	// Handed to the catalog before Reindex so Reindex stamps it on the entries.
	if setter, ok := cat.(mbapi.RegistrationOutcomeSetter); ok {
		outcomes := make(map[string]mbapi.RegistrationOutcome, len(dataTypes))
		failedBy := make(map[string]string, len(failed))
		for _, f := range failed {
			failedBy[f.URI] = f.Reason
		}
		for _, dt := range dataTypes {
			uri := dt.ResolvedURI()
			if reason, bad := failedBy[uri]; bad {
				outcomes[uri] = mbapi.RegistrationOutcome{Registered: false, Reason: reason}
			} else if len(dt.Source()) > 0 {
				outcomes[uri] = mbapi.RegistrationOutcome{Registered: true}
			}
		}
		setter.SetRegistrationOutcomes(outcomes)
	}

	// Attach + reindex.
	if err := cat.SetSchema(&catalogSchemaAdapter{svc: schemaSvc}); err != nil {
		slog.Warn("catalog Phase 2: SetSchema", "error", err)
	}
	// Install the central validation toggle so Reindex's schema-validation
	// checkpoint runs with the configured posture (Contributions.yaml;
	// absent file → warn-global / error-strict defaults). Duck-typed so
	// MemoryCatalog + test stubs don't need to implement it.
	if setter, ok := cat.(catalogValidationConfigSetter); ok {
		valCfg, _ := application.UseConfig[contribapi.ContributionsConfig](ctx)
		setter.SetValidationConfig(valCfg)
	}
	if err := cat.Reindex(ctx, nil); err != nil {
		slog.Warn("catalog Phase 2: Reindex", "error", err)
	}
	if err := cat.MarkReady(ctx); err != nil {
		slog.Warn("catalog Phase 2: MarkReady", "error", err)
	}
	slog.Info("catalog Phase 2 complete", "data-types", registered)
}

// catalogSchemaAdapter bridges schemaapi.SchemaService → the local
// mbapi.CatalogSchemaService contract the catalog uses for edge
// extraction + body validation. The catalog package intentionally
// defines its own minimal interface (rather than importing
// schemaapi.SchemaService verbatim) to avoid cross-extension imports
// in manifest/api.
type catalogSchemaAdapter struct {
	svc schemaapi.SchemaService
}

// GetDataTypeByURI returns the registered data-type AST as JSON bytes,
// matching CatalogSchemaService's contract. The catalog unmarshals at
// the call site.
func (a *catalogSchemaAdapter) GetDataTypeByURI(ctx context.Context, uri string) (json.RawMessage, error) {
	out, err := a.svc.GetDataType(ctx, schemaapi.GetDataTypeInput{URI: uri})
	if err != nil {
		return nil, err
	}
	if out.Schema == nil {
		return nil, nil
	}
	return json.Marshal(out.Schema)
}

// ValidateBody delegates to ValidateAgainst and surfaces the first
// error diagnostic when the body fails. nil = OK.
func (a *catalogSchemaAdapter) ValidateBody(ctx context.Context, dataTypeURI string, body json.RawMessage) error {
	out, err := a.svc.ValidateAgainst(ctx, schemaapi.ValidateAgainstInput{URI: dataTypeURI, Body: body})
	if err != nil {
		return err
	}
	if out.Valid {
		return nil
	}
	if len(out.Diagnostics) > 0 {
		return fmt.Errorf("schema: %s", out.Diagnostics[0].Message)
	}
	return fmt.Errorf("schema: validation failed for %s", dataTypeURI)
}

// connectionManagerExtension publishes the ConnectionManager service
// and contributes the `connections` Cobra command group. Members are
// loaded once at first-Use from the universe's Univers.yaml; this
// is the runtime peer of catalogExtension above.
//
// Bus-aware: when MessageBus("default") is available (during `serve`),
// the manager is built via NewManagerWithBus so sidecars declared with
// `transport: nats` actually spawn-via-NATS at Acquire. Outside `serve`
// (e.g. `connections list`), the bus isn't registered yet and the
// manager falls back to stdio-only — nats-mode entries error at Acquire
// but inspection works fine.
func connectionManagerExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[cmapi.ConnectionManager](
			func(ctx context.Context) (cmapi.ConnectionManager, error) {
				reg, err := universeMembers()
				if err != nil {
					return nil, fmt.Errorf("connmgr: load Univers.yaml sidecars: %w", err)
				}
				specs := make([]cmapi.SpawnSpec, 0, len(reg.Sidecars))
				for _, s := range reg.Sidecars {
					specs = append(specs, withVersionCheck(spawnSpecOf(s), s, hubCache()))
				}
				// Try to resolve the bus + scope. If `serve` has run, we
				// get the bus-aware manager; otherwise fall back to
				// stdio-only.
				app := application.AppFromContext(ctx)
				bus, err := application.UseNamedService[mbapi.MessageBus](ctx, "default")
				if err != nil || bus == nil {
					return withHubSelf(connmgrlocal.NewManager(specs))(app)
				}
				manifest, err := cenv.LoadUniverseManifest(application.Env().UniverseDir())
				if err != nil {
					return withHubSelf(connmgrlocal.NewManager(specs))(app)
				}
				tenantID := os.Getenv("KTW_TENANT_ID")
				if tenantID == "" {
					tenantID = "local"
				}
				dataDir := application.Env().DataDir()
				return withHubSelf(connmgrlocal.NewManagerWithBus(specs, connmgrlocal.BusScope{
					Bus:        bus,
					TenantID:   tenantID,
					UniverseID: manifest.Name,
					// Each member its own bus key.
					KeyFor: func(member string) (string, error) {
						if _, err := busauth.Ensure(dataDir, busauth.KindSidecar, member, tenantID); err != nil {
							return "", err
						}
						return busauth.CredentialPath(busauth.KeysDir(dataDir), busauth.KindSidecar, member), nil
					},
					// Who holds a member's serving lock, named with its
					// parent.
					ServingHolder: func(member string) string {
						return hubclient.DescribeHolder(ktslocal.ServingProviders(dataDir)[member])
					},
				}))(app)
			},
		))
		init.AddExtension(appcore.CommandExt(connmgrcommands.NewConnectionsCommand()))
	}
}

// withHubSelf makes the hub a producer the connection manager can reach: its
// own contributions are in the view (e24 / N11), so a Read or OpenLayer the
// catalog forwards to the entry's host may name the hub itself. The hub is not
// a Univers.yaml member and is never spawned — Acquire(hubProvider) answers
// with a sender over the hub's own ResourceService, in process. Every other
// name goes to the real manager unchanged.
func withHubSelf(mgr cmapi.ConnectionManager, err error) func(*application.Application) (cmapi.ConnectionManager, error) {
	return func(app *application.Application) (cmapi.ConnectionManager, error) {
		if err != nil || mgr == nil || app == nil {
			return mgr, err
		}
		return &hubSelfManager{ConnectionManager: mgr, self: &hubSelfSender{app: app}}, nil
	}
}

// hubSelfManager is a ConnectionManager that answers for the hub itself.
type hubSelfManager struct {
	cmapi.ConnectionManager
	self ktsp.Sender
}

func (m *hubSelfManager) Acquire(ctx context.Context, name string) (ktsp.Sender, error) {
	if name == hubProvider {
		return m.self, nil
	}
	return m.ConnectionManager.Acquire(ctx, name)
}

func (m *hubSelfManager) Release(name string) {
	if name == hubProvider {
		return
	}
	m.ConnectionManager.Release(name)
}

// SetMembers forwards the Univers.yaml watcher's membership swap.
func (m *hubSelfManager) SetMembers(specs []cmapi.SpawnSpec) error {
	if s, ok := m.ConnectionManager.(connmgrlocal.MemberSwapper); ok {
		return s.SetMembers(specs)
	}
	return nil
}

// hubSelfSender answers the two producer calls the catalog forwards —
// `resources/read` and `resources/open-layer` — from the hub's own
// ResourceService, the one that answers `sc resources` for the hub.
type hubSelfSender struct{ app *application.Application }

func (h *hubSelfSender) Call(ctx context.Context, path string, params any, _ ...ktsp.CallOption) (json.RawMessage, error) {
	svc, err := application.AppUseService[mbapi.ResourceService](h.app)
	if err != nil {
		return nil, fmt.Errorf("ktw-hub: own ResourceService: %w", err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var out any
	switch path {
	case "resources/read":
		var in mbapi.ContributionReadInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, ktsp.ErrInvalidRequest(err.Error())
		}
		out, err = svc.Read(ctx, in)
	case "resources/open-layer":
		var in mbapi.OpenLayerInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, ktsp.ErrInvalidRequest(err.Error())
		}
		out, err = svc.OpenLayer(ctx, in)
	default:
		return nil, ktsp.ErrSlotNotFound(hubProvider + ":" + path)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (h *hubSelfSender) Notify(context.Context, string, any, ...ktsp.CallOption) error {
	return nil
}

// hubExtension wires the embedded NATS lifecycle into the application.
//   - `ktw-hub serve` starts NATS, registers the MessageBus service, blocks.
//   - Other subcommands do NOT start NATS; useful for `--help`, future
//     introspection commands.
func hubExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.CommandExt(newServeCommand()))
	}
}

func newServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the bus host (embedded NATS + JetStream provisioning)",
		Long: `Start the embedded NATS server, provision JetStream streams + KV buckets,
register the MessageBus service for in-process consumers, and block until
SIGTERM/SIGINT.

In cloud mode (when nats.address points at an external cluster), this binary
becomes a pure NATS client. The embedded path is the local default.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// THE HUB ENDS CLEANLY: on SIGINT / SIGTERM, and — a hub a
			// command spawned (hubclient.Owned) — once no client is connected,
			// whoever spawned it and however that process ended (
			// the watch starts with the lockfile, below). Its context ends, the command returns, and the deferred
			// cleanup runs: the lockfile removed, the embedded NATS stopped,
			// the members closed with the application. A cleanup that hangs
			// is cut short: the lockfile removed, exit.
			// The hub serves others: no cause of its own (the profile's person
			// its start read is not who calls it; a keyed bus refuses the
			// person's cause it cannot attest -).
			//
			// ONE HUB PER UNIVERSE: a hub started while another
			// live hub's lockfile names the universe refuses to serve - its
			// members would find their serving locks held by the other hub's
			// and never come up; its spawner joins the running hub. Every
			// lockfile it removes - hiding, ending, cut short - only while
			// the lockfile names this hub.
			if running := hubclient.LiveHub(application.Env().DataDir()); running != nil && running.PID != os.Getpid() {
				return fmt.Errorf("ktw-hub: %w", &hubclient.HubRunningError{PID: running.PID, NATSURL: running.NATSURL})
			}
			ctx, stopSignals := signal.NotifyContext(cause.Without(cmd.Context()), os.Interrupt, syscall.SIGTERM)
			defer stopSignals()
			ctx, endOwned := context.WithCancel(ctx)
			defer endOwned()
			go func() {
				<-ctx.Done()
				time.Sleep(hubShutdownBudget)
				_ = hubclient.RemoveOwnLockFile(application.Env().DataDir(), os.Getpid())
				fmt.Fprintf(cmd.ErrOrStderr(), "ktw-hub: shutdown took more than %s — ending now\n", hubShutdownBudget)
				os.Exit(1)
			}()
			bootStart := time.Now()
			env := application.Env()
			storeDir := filepath.Join(env.DataDir(), "nats", "jetstream")

			// The bus checks keys: the hub mints its own, the
			// GUI's and the CLI's (a sidecar's before it spawns it,
			// connmgr), every one of the hub's tenant - the one account's.
			keyTenant := os.Getenv("KTW_TENANT_ID")
			if keyTenant == "" {
				keyTenant = "local"
			}
			hubKey, err := busauth.Ensure(env.DataDir(), busauth.KindHub, busauth.KindHub, keyTenant)
			if err != nil {
				return fmt.Errorf("ktw-hub: the bus keys: %w", err)
			}
			for _, kind := range []string{busauth.KindGUI, busauth.KindCLI} {
				if _, err := busauth.Ensure(env.DataDir(), kind, kind, keyTenant); err != nil {
					return fmt.Errorf("ktw-hub: the bus keys: %w", err)
				}
			}

			opts := natsembedded.EmbeddedOptions(storeDir, false /*verbose*/)
			h, err := natsembedded.StartEmbeddedKeyed(opts, hubKey.Registry(), hubKey)
			if err != nil {
				return fmt.Errorf("ktw-hub: %w", err)
			}
			defer h.Stop()

			// Wrap the connection in the MessageBus adapter and register
			// it as a service so other extensions can resolve it via
			// application.UseService[mbapi.MessageBus](ctx).
			app := application.AppFromContext(ctx)
			if app == nil {
				return fmt.Errorf("ktw-hub: no Application in context")
			}
			adapter := natsxport.NewKeyed(h.Client, h.Credential)
			application.PutService[mbapi.MessageBus](app, "default", adapter)

			// Expose the NATS URL via env BEFORE any spawn so the
			// connmgr's lazily spawned `<binary> sc ktsp nats` children
			// find the bus without needing the lockfile (which is written
			// only AFTER cache hydration — see the lockfile block below).
			// This avoids the chicken-and-egg where the lockfile is the
			// "hub is ready for outside clients" signal but spawned
			// sidecars also need it to dial the bus.
			natsURL := h.Server.ClientURL()
			_ = os.Setenv("KTW_NATS_URL", natsURL)

			fmt.Fprintf(cmd.OutOrStdout(),
				"ktw-hub: embedded NATS ready at %s (JetStream store: %s); MessageBus service registered\n",
				natsURL, storeDir)

			// Attach the contributions-changed event publisher to the
			// catalog so every Reload (via the cache watcher) and
			// MarkReady emits on the universe-scoped bus subject.
			// Cross-binary subscribers (ktw-agents, ktw-gui) listen via
			// `subjects.ContributionsChangedSubject(tenant, universe)`.
			//
			// Resolution: tenant from env (default "local"); universe
			// from the loaded universe manifest. Same chain
			// connectionManagerExtension uses (above) — keeps every
			// hub-side bus consumer on the same (tenant, universe) tuple.
			{
				tenantID := os.Getenv("KTW_TENANT_ID")
				if tenantID == "" {
					tenantID = "local"
				}
				universeID := ""
				if manifest, mErr := cenv.LoadUniverseManifest(application.Env().UniverseDir()); mErr == nil {
					universeID = manifest.Name
				}
				if universeID != "" {
					cat, catErr := application.UseService[mbapi.Catalog](ctx)
					if catErr == nil && cat != nil {
						if setter, ok := cat.(catalogPublisherSetter); ok {
							setter.SetEventPublisher(manifestlocal.NewNATSContributionsEventPublisher(adapter, tenantID, universeID))
							fmt.Fprintf(cmd.OutOrStdout(),
								"ktw-hub: contributions-changed publisher attached (tenant=%s universe=%s)\n",
								tenantID, universeID)
						}
					}
				}
			}

			// Subscribe the hub's own KTSP endpoint to its slot subjects.
			// Without this, the hub publishes CatalogService + ConnMgrService
			// as auto-KTSP slots IN-PROCESS but has NO responders on the bus —
			// every `ktw-hub:catalog-service/list-contributions` from ktw-gui hits
			// ErrNoResponders. Subscribing here closes that gap.
			//
			// (tenant, universe) match the resolution `connectionManagerExtension`
			// uses for outbound calls — same chain every other bus consumer reads.
			// `selfProvider = "ktw-hub"` keys the slot subjects + queue groups
			// under the hub binary's bus identity (matches AppConfig.AppName).
			endpoint, epErr := application.UseService[*ktsp.Endpoint](ctx)
			if epErr != nil {
				return fmt.Errorf("ktw-hub: resolve KTSP Endpoint: %w", epErr)
			}
			busTenant := os.Getenv("KTW_TENANT_ID")
			if busTenant == "" {
				busTenant = "local"
			}
			var busUniverse string
			if m, mErr := cenv.LoadUniverseManifest(env.UniverseDir()); mErr == nil && m != nil {
				busUniverse = m.Name
			} else {
				busUniverse = "default"
			}
			selfProvider := cmd.Root().Use
			group, subErr := mbsubscribe.Subscribe(ctx, adapter,
				busTenant, busUniverse, selfProvider, endpoint)
			if subErr != nil {
				return fmt.Errorf("ktw-hub: subscribe own slots: %w", subErr)
			}
			defer func() { _ = group.Drain() }()
			fmt.Fprintf(cmd.OutOrStdout(),
				"ktw-hub: subscribed %d local slot(s) on the bus (tenant=%s universe=%s provider=%s)\n",
				len(group.Slots()), busTenant, busUniverse, selfProvider)

			// The panel server keeps every display panel of the universe
			// : subscribed to its panel subjects, it writes
			// each panel's log and snapshot under <DataDir>/panels/.
			if stopPanels, pErr := panelserverlocal.Serve(ctx, adapter, busTenant, busUniverse); pErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "ktw-hub: the panel server does not run: %v\n", pErr)
			} else {
				defer stopPanels()
			}

			// Cache hydration — bring the tier-1 manifest cache up to date by
			// probing each member whose record is missing or stale. This is
			// the catalog's purpose: enumerate without running sidecars
			// long-term. The probe is exec-and-kill, NOT a long-running NATS
			// subscription.
			//
			// Staleness rule: a member's record is stale when `Cache.IsStale`
			// finds the executable's recorded size/mtime, or a served folder's
			// digest, moved (see cacheIsFresh below). Dev cycle (rebuild a
			// sidecar) triggers a re-probe automatically.
			//
			// The lockfile does not wait for a STALE record's re-probe: the
			// probe of a freshly rebuilt executable waits for the OS to scan
			// it first (11-29 s measured), and the catalog already serves that
			// member's previous record. Clients connect at once and read the
			// cached manifests; when the re-probe lands the catalog reloads
			// and announces contributions-changed. Only a member with NO
			// record is probed before the lockfile — there is nothing of it
			// to serve (bootHydration).
			//
			// Sidecars themselves are NOT spawned at hub boot — they come up
			// lazily on first slot request (client retries on NoResponders →
			// `ktw-hub:/ConnMgr/EnsureSpawned` → hub spawns → client
			// retries): "do not spawn all sidecars until they are needed,
			// this is why we created the catalog."
			// Sidecar cache keyed by member and declared binary — see the
			// catalog factory above for the rationale (CacheDir, not DataDir).
			cacheDir := filepath.Join(env.CacheDir(), "sidecars")
			cache := universeCache(cacheDir)
			var sidecars []appcore.SidecarSpec
			if reg, regErr := universeMembers(); regErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"ktw-hub: load Univers.yaml sidecars for cache hydrate: %v\n", regErr)
			} else {
				sidecars = reg.Sidecars
			}
			// Re-read the records into the catalog and re-run Phase 2, so the
			// freshly-probed sidecars' data-types reach SchemaService and their
			// Contributions get edges extracted. Reload announces
			// contributions-changed.
			refreshCatalog := func() {
				cat, err := application.UseService[mbapi.Catalog](ctx)
				if err != nil {
					return
				}
				if r, ok := cat.(catalogReloader); ok {
					_ = r.Reload()
				}
				if schemaSvc, schemaErr := application.UseService[schemaapi.SchemaService](ctx); schemaErr == nil && schemaSvc != nil {
					applyHubPhase2(ctx, cat, schemaSvc)
				}
			}
			hydrate := func(members []appcore.SidecarSpec) int {
				hydrateStart := time.Now()
				hydrated, probed, errs := hydrateCache(cache, members, cmd.ErrOrStderr())
				// A re-probe execs each changed member (and the OS may scan a
				// freshly built executable first): the boot's usual outlier,
				// so the log says how long it took.
				slog.Info("hub boot: tier-1 cache hydrated", "fresh", hydrated, "reprobed", probed,
					"errors", errs, "took", time.Since(hydrateStart).Round(time.Millisecond))
				fmt.Fprintf(cmd.OutOrStdout(),
					"ktw-hub: cache hydrated — %d cached, %d re-probed, %d errors\n",
					hydrated, probed, errs)
				return probed
			}

			// Hub is ready: write the lockfile so outside clients
			// (ktw-gui, ktw-agents CLI) can discover the bus. No
			// sidecar spawn yet — runtime invocations trigger lazy
			// spawn via the connmgr's EnsureSpawned path.
			lockWritten := false
			defer func() {
				if !lockWritten {
					return
				}
				if err := hubclient.RemoveOwnLockFile(env.DataDir(), os.Getpid()); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "ktw-hub: remove lockfile: %v\n", err)
				}
			}()
			writeLock := func() error {
				return hubclient.ClaimLockFile(env.DataDir(), hubclient.LockFile{
					NATSURL:    natsURL,
					HubVersion: version,
				})
			}
			ready := func() error {
				if err := writeLock(); err != nil {
					return fmt.Errorf("ktw-hub: write lockfile: %w", err)
				}
				lockWritten = true
				// A command's hub serves every command until none uses it. It
				// hides its lockfile under the spawn guard, so no command
				// starts a second hub while it decides.
				if hubclient.Owned() {
					go hubclient.EndOwnedWhenUnused(ctx, env.DataDir(), os.Getpid(),
						func() (int, int) {
							open, ever := h.Clients(hubclient.ClientName)
							return open + hubclient.Waiting(env.DataDir()), ever
						},
						writeLock,
						func() {
							fmt.Fprintln(cmd.OutOrStdout(), "ktw-hub: no client left — shutting down")
							endOwned()
						})
				}
				// A hub another live hub's lockfile replaced is no longer the
				// universe's: it ends, its members let their serving locks
				// go, its clients re-dial the registered hub.
				// One whose lockfile is gone or names a hub not alive writes
				// it again: a running hub is never unregistered.
				go hubclient.KeepRegistered(ctx, env.DataDir(), os.Getpid(), writeLock, func() {
					fmt.Fprintln(cmd.OutOrStdout(), "ktw-hub: another hub is registered for this universe — shutting down")
					endOwned()
				})
				fmt.Fprintf(cmd.OutOrStdout(),
					"ktw-hub: lockfile written at %s — outside clients can now connect\n",
					hubclient.LockFilePath(env.DataDir()))
				// What a GUI activating this universe waits for, from the hub's
				// side (an executable scan before main shows only in the
				// client's wait, never here).
				slog.Info("hub boot: ready for clients", "took", time.Since(bootStart).Round(time.Millisecond))
				return nil
			}

			// What compares the cache against its sources runs once the boot's
			// own re-probes are done — a watch started earlier would see the
			// same stale record and probe the same executable a second time.
			watchRecords := func() {
				// Watch the cache dir so the in-memory Catalog stays in sync
				// with on-disk cache changes (e.g. `catalog refresh`, manual
				// file drops, another hub instance). Errors are swallowed in
				// the goroutine — a watch failure shouldn't bring down the bus.
				go func() { _ = manifestlocal.WatchCache(ctx, cacheDir, refreshCatalog) }()
				fmt.Fprintf(cmd.OutOrStdout(),
					"ktw-hub: watching the tier-1 manifest cache %s for catalog updates\n", cacheDir)

				// Watch every declared member's SOURCES for a change the record
				// cannot see by itself, and re-probe the member so the universe
				// converges without a restart (concepts §10):
				//
				//   - a SERVED FOLDER: a member whose contributions live in a
				//     folder has the same executable whatever that folder holds,
				//     so the exe stat is blind to it; the folder's digest against
				//     the record is what makes an author's edit visible;
				//   - a COMPILED member's executable: a rebuild while the hub runs
				//     used to wait for a restart or `catalog refresh` (blind audit
				//     M1). One os.Stat per member per round, compared against the
				//     RECORD's stat, never a baseline.
				//
				// The membership is re-read each round rather than captured, so a
				// member added while the hub runs starts being watched without a
				// restart.
				exeWatch := newExeStatWatch(universeCache(cacheDir))
				go watchMembers(ctx, manifestlocal.ContentWatchInterval,
					declaredMembers,
					func(s appcore.SidecarSpec) (bool, string) {
						if s.ContentRoot != "" {
							return servedContentMoved(s.Name), "served content changed"
						}
						if exeWatch.moved(s.Name, recordSource(s)) {
							return true, "executable changed"
						}
						// A folder the member declared it reads (ktw-agents'
						// universe and global skills and tasks): its executable
						// is unchanged by an edit there.
						return exeWatch.cache.ReadsMoved(s.Name, recordSource(s))
					},
					func(name string) error { _, err := refreshMember(ctx, name); return err },
					cmd.OutOrStdout(), cmd.ErrOrStderr())
			}

			if err := bootHydration(cache, sidecars, bootSteps{
				hydrate: hydrate,
				refresh: refreshCatalog,
				ready:   ready,
				after:   watchRecords,
			}); err != nil {
				return err
			}

			// The object entries pushed for the project's member (review
			// R-A1): each push is applied to the member and its record rebuilt
			// and reloaded, no restart; once subscribed the hub asks the
			// process hosting CWM for each project's snapshot.
			{
				pushTenant, pushUniverse := cenv.ResolveTenantUniverse(env.HomeDir())
				pushes := &projectPushHandler{
					members: projectMembers,
					sidecars: func() ([]appcore.SidecarSpec, error) {
						reg, err := universeMembers()
						if err != nil {
							return nil, err
						}
						return reg.Sidecars, nil
					},
					cache:  cache,
					reload: refreshCatalog,
				}
				if sub, err := serveProjectPushes(ctx, adapter, pushTenant, pushUniverse, pushes); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "ktw-hub: the project's pushed contributions are not served: %v\n", err)
				} else {
					defer func() { _ = sub.Unsubscribe() }()
				}
				// Trusting a project is an edit of projects.yaml: on a change
				// the membership is read again and the record of each member
				// whose trust moved is rebuilt, no restart.
				projectsPath := filepath.Join(env.HomeDir(), cenv.AttachedProjectsFile)
				go func() {
					_ = manifestlocal.WatchUniverseSidecars(ctx, projectsPath, func() {
						rebuildTrustMoved(ctx, pushes, cmd.ErrOrStderr())
					})
				}()
			}

			// Override-tree freshness (dev only): when the universe opts into
			// workspace overrides, poll the overwrites root and re-hydrate the
			// cache on any change. The digest-aware IsStale then re-probes
			// exactly the sidecars whose overrides moved. A poll (not fsnotify)
			// because the override tree is deep + dev-churny and recursive
			// watches are flaky on Windows; compose also refreshes explicitly
			// after a write, so this is a safety net, not the primary path.
			// Only the universe's tree: an attached project's overrides come
			// as override entries through the project member's push.
			if cenv.AllowOverwrites(env.UniverseDir()) {
				overwritesRoot := filepath.Join(env.UniverseDir(), "overwrites")
				go watchOverwrites(ctx, universeCache(cacheDir), cacheDir, overwritesRoot, cmd.ErrOrStderr())
				fmt.Fprintf(cmd.OutOrStdout(),
					"ktw-hub: watching the universe's override tree for changes\n")
			}

			// Watch Univers.yaml for membership changes. On rewrite
			// (sidecar added/removed in `sidecars:`), reload the
			// registry, project to the new []SpawnSpec list, and call
			// SetMembers on both the catalog and the connection manager.
			// Live entries whose name remains keep their state; removed
			// entries get closed (per Connmgr.SetMembers semantics).
			regPath := appcore.UniverseSidecarsPath()
			go func() {
				_ = manifestlocal.WatchUniverseSidecars(ctx, regPath, func() {
					reg, err := universeMembers()
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"ktw-hub: Univers.yaml sidecars reload failed: %v\n", err)
						return
					}
					members := catalogMembers(reg.Sidecars)
					specs := make([]cmapi.SpawnSpec, 0, len(reg.Sidecars))
					for _, s := range reg.Sidecars {
						specs = append(specs, withVersionCheck(spawnSpecOf(s), s, hubCache()))
					}
					if cat, err := application.UseService[mbapi.Catalog](ctx); err == nil {
						if s, ok := cat.(catalogMemberSwapper); ok {
							_ = s.SetMembers(members)
						}
						// Re-run Phase 2 after membership change so newly-added
						// sidecars' data-types reach SchemaService and removed
						// sidecars' edges are purged.
						if schemaSvc, schemaErr := application.UseService[schemaapi.SchemaService](ctx); schemaErr == nil && schemaSvc != nil {
							applyHubPhase2(ctx, cat, schemaSvc)
						}
					}
					if mgr, err := application.UseService[cmapi.ConnectionManager](ctx); err == nil {
						if s, ok := mgr.(connmgrlocal.MemberSwapper); ok {
							_ = s.SetMembers(specs)
						}
					}
				})
			}()
			fmt.Fprintf(cmd.OutOrStdout(),
				"ktw-hub: watching Univers.yaml at %s for membership updates\n", regPath)

			// Follow the universe's keys and config files:
			// a change made while it runs - by hand, a vault, a config-set -
			// restarts the running members that read it.
			if mgr, err := application.UseService[cmapi.ConnectionManager](ctx); err == nil {
				home := env.WorkspaceDir()
				if home == "" {
					home = env.HomeDir()
				}
				places := follow.Places{
					EnvFiles:   []string{filepath.Join(env.HomeDir(), ".env"), filepath.Join(env.GlobalHomeDir(), ".env")},
					ConfigDirs: []string{env.ConfigDir()},
				}
				reads := func(ctx context.Context, member string) (follow.Reads, error) {
					return follow.MemberReads(ctx, home, member)
				}
				go func() {
					_ = follow.Watch(ctx, places, managerHub{mgr}, reads, nil, cmd.ErrOrStderr(), nil)
				}()
				fmt.Fprintf(cmd.OutOrStdout(),
					"ktw-hub: following the universe's keys and config at %s\n", env.ConfigDir())
			}

			// Block until ctx is cancelled (SIGTERM / SIGINT handled by the framework).
			<-ctx.Done()
			fmt.Fprintln(cmd.OutOrStdout(), "ktw-hub: shutting down")
			return nil
		},
	}
}

// managerHub is the connection manager as follow restarts members through it.
type managerHub struct{ mgr cmapi.ConnectionManager }

func (h managerHub) Running(context.Context) ([]string, error) {
	var out []string
	for _, st := range h.mgr.All() {
		if st.State == cmapi.ConnReady {
			out = append(out, st.Name)
		}
	}
	return out, nil
}

func (h managerHub) Restart(ctx context.Context, member string) (bool, error) {
	rctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return h.mgr.Restart(rctx, member)
}

// hubShutdownBudget is how long a stopping hub may take to clean up before it
// removes its lockfile and ends regardless.
const hubShutdownBudget = 15 * time.Second

// needsCatalogReload reports whether the boot-time cache hydration changed
// the tier-1 cache the catalog was just built from: only a re-probed member
// (a record saved) does. A member found fresh or whose probe failed leaves the
// cache as the catalog read it.
func needsCatalogReload(probed int) bool { return probed > 0 }

// spawnSpecOf is what the connection manager needs of a Univers.yaml member:
// one mapping, for the manager built at boot and for the membership watch (the
// watch's own copy had lost the restart policy).
func spawnSpecOf(s appcore.SidecarSpec) cmapi.SpawnSpec {
	if s.IsFolder() {
		// Nothing to spawn, whatever binary the entry names: the hub reads
		// the folder itself, and connmgr refuses to start it.
		return cmapi.SpawnSpec{Name: s.Name, Transport: s.Transport, Folder: folderOf(s)}
	}
	return cmapi.SpawnSpec{
		Name:             s.Name,
		Transport:        s.Transport,
		Binary:           s.Binary,
		Args:             s.Args,
		Env:              s.Env,
		Folder:           folderOf(s),
		IdleTimeout:      s.IdleTimeout,
		Pin:              s.Pin,
		Lifecycle:        s.Lifecycle,
		MaxRestarts:      s.MaxRestarts,
		MaxRestartWindow: s.MaxRestartWindow,
		GracefulShutdown: s.GracefulShutdown,
		ReadyTimeout:     s.ReadyTimeout,
	}
}

// withVersionCheck refuses a member whose binary is not the version its
// universe pins: the pin is its store path
// (SidecarSpec.PinnedVersion), the version is what the binary reported to the
// manifest probe (the tier-1 cache). A skew is refused, never logged and run:
// KTSP has no version negotiation on the member path. A member not wired from
// the store (a dev build), or whose version is not known yet, is not checked.
func withVersionCheck(spec cmapi.SpawnSpec, s appcore.SidecarSpec, cache *manifestlocal.Cache) cmapi.SpawnSpec {
	if why := versionRefusal(s, cache); why != "" {
		spec.Refused = why
	}
	return spec
}

// versionRefusal says why s is refused ("" when it is not).
func versionRefusal(s appcore.SidecarSpec, cache *manifestlocal.Cache) string {
	pinned := s.PinnedVersion()
	if pinned == "" || s.IsFolder() || s.Binary == "" || cache == nil {
		return ""
	}
	m, _, err := cache.Load(s.Name, s.Binary)
	if err != nil || m == nil || m.Version == "" || m.Version == "dev" {
		return ""
	}
	if strings.TrimPrefix(m.Version, "v") == strings.TrimPrefix(pinned, "v") {
		return ""
	}
	return fmt.Sprintf("%s reports version %s and the universe pins %s (%s): not started - reinstall that version or repin the member",
		s.Name, m.Version, pinned, s.Binary)
}

// folderOf is a folder member's content root ("" for a member that runs).
func folderOf(s appcore.SidecarSpec) string {
	if s.IsFolder() {
		return s.ContentRoot
	}
	return ""
}

// universeMembers is the universe's membership as the hub reads it: the
// members Univers.yaml declares, then the contributions folder of every
// attached project as a FOLDER member named by its provider token
// (pcapi.ProjectMembers: the project's `contributions` capability, its
// defaults). Never written back: the projects are attached, not declared.
// Every read of the membership in the hub goes through it, so an attach or a
// detach is a membership change the watch sees.
func universeMembers() (*appcore.UniverseSidecars, error) {
	reg, _, err := universeMembership()
	return reg, err
}

// universeMembership is universeMembers with the members refused: each
// refused project's token and why it is not served (a finding naming the
// token and the owners it collides with).
func universeMembership() (*appcore.UniverseSidecars, map[string]string, error) {
	reg, err := appcore.LoadUniverseSidecars()
	if err != nil {
		return nil, nil, err
	}
	out, refused, served := membershipOf(reg, attachedProjectRoots())
	// Each member holds the object entries pushed for its project's token
	// beside its folder (projectMembers), told whether the universe trusts
	// its project; a detached project's are dropped.
	projectMembers.keepAll(served, projectTrusted)
	return out, refused, nil
}

// projectTrusted reports whether the universe trusts the attached project at
// root (projects.yaml trusted, cenv.ProjectTrusted); a test sets it.
var projectTrusted = func(root string) bool {
	return cenv.ProjectTrusted(application.Env().HomeDir(), root)
}

// attachedProjectRoots are the roots of the projects attached to the
// universe, in the order attached (cenv.AttachedProjects); a test sets it.
var attachedProjectRoots = func() []string {
	return cenv.AttachedProjects(application.Env().HomeDir())
}

// membershipOf is reg with the folder member of every project at
// projectRoots appended (pcapi.ProjectMembers, in the order attached), the
// projects refused keyed by token — several refused under one token have
// their findings joined, none lost — and the project members served.
func membershipOf(reg *appcore.UniverseSidecars, projectRoots []string) (*appcore.UniverseSidecars, map[string]string, []pcapi.ProjectMember) {
	names := make([]string, 0, len(reg.Sidecars))
	for _, s := range reg.Sidecars {
		names = append(names, s.Name)
	}
	members, refusedMembers := pcapi.ProjectMembers(projectRoots, names)
	var refused map[string]string
	for _, r := range refusedMembers {
		slog.Warn("hub: the project's contributions are not served: "+r.Why, "project", r.Project)
		if refused == nil {
			refused = map[string]string{}
		}
		if prev, ok := refused[r.Token]; ok {
			refused[r.Token] = prev + "; " + r.Why
		} else {
			refused[r.Token] = r.Why
		}
	}
	if len(members) == 0 {
		return reg, refused, nil
	}
	out := *reg
	out.Sidecars = append([]appcore.SidecarSpec(nil), reg.Sidecars...)
	for _, m := range members {
		out.Sidecars = append(out.Sidecars, appcore.SidecarSpec{Name: m.Token, ContentRoot: m.Folder})
	}
	return &out, refused, members
}

// recordSource is what a member's record is taken from, and keyed by: its
// binary, or a folder member's root (absolute, cleaned) — the hub reads the
// folder itself. A removed folder reads stale as a missing binary does.
func recordSource(s appcore.SidecarSpec) string {
	if !s.IsFolder() {
		return s.Binary
	}
	root := filepath.Clean(s.ContentRoot)
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if runtime.GOOS == "windows" {
		// One folder, one key, however the membership spells its case.
		root = strings.ToLower(root)
	}
	return root
}

// folderManifest builds a folder member's manifest in process
// (manifestlocal.ProvidersManifest over memberProviders: the folder read as a
// pack, against the kinds of the universe, and for the project's member the
// object entries pushed to it).
func folderManifest(name, root string, kinds contribapi.Kinds) (*mbapi.Manifest, error) {
	ctx := context.Background()
	ps, err := memberProviders(ctx, name, root, kinds)
	if err != nil {
		return nil, err
	}
	return manifestlocal.ProvidersManifest(ctx, name, ps, kinds)
}

// folderProviders are a folder member's providers (manifestlocal.FolderProviders).
func folderProviders(name, root string, kinds contribapi.Kinds) ([]contribapi.ContributionProvider, error) {
	return manifestlocal.FolderProviders(name, root, kinds)
}

// folderMemberProviders are the providers of the folder member named host
// among sidecars; ok is false when host is no folder member (it runs: the
// hub asks its process). The kinds are the universe's, from cache.
func folderMemberProviders(host string, sidecars []appcore.SidecarSpec, cache *manifestlocal.Cache) ([]contribapi.ContributionProvider, bool, error) {
	for _, s := range sidecars {
		if s.Name != host || !s.IsFolder() {
			continue
		}
		ps, err := memberProviders(context.Background(), s.Name, recordSource(s), universeKinds(cache, sidecars))
		return ps, true, err
	}
	return nil, false, nil
}

// openFromProviders opens one layer of the entry (type, id) through the
// providers that publish it — a folder member's, in process.
func openFromProviders(ctx context.Context, providers []contribapi.ContributionProvider, typ, id, layer string) ([]byte, string, error) {
	for _, p := range providers {
		if p.Type() != typ {
			continue
		}
		rc, ref, err := p.Open(ctx, id, layer)
		if err != nil {
			continue // another provider of the kind (a companion) may hold it
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return nil, "", err
		}
		return b, ref.MediaType, nil
	}
	return nil, "", contribapi.ErrContributionNotFound{URI: contribapi.ResourceURI{Type: typ, ID: id}.String()}
}

// universeKinds are the kinds a folder member's files may be: every kind compiled
// into the hub (builtin.Kinds, declared under the system provider so a member
// cannot redeclare them)
// and every kind a member of the universe declares, read from their records.
func universeKinds(cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) contribapi.Kinds {
	var decls []contribapi.Contribution
	if sys, err := builtin.Kinds(); err == nil {
		for _, n := range sys.Names() {
			if d, ok := sys.Get(n); ok {
				c := contribapi.Contribution{Provider: contribapi.SystemProvider, Type: contribapi.ResourceTypeContributionType, ID: n}
				if b, err := json.Marshal(d); err == nil {
					c.SetData(b)
					decls = append(decls, c)
				}
			}
		}
	}
	for _, s := range sidecars {
		src := recordSource(s)
		if src == "" {
			continue
		}
		m, _, err := cache.Load(s.Name, src)
		if err != nil || m == nil {
			continue
		}
		for _, c := range m.Contributions {
			if c.Type == contribapi.ResourceTypeContributionType {
				decls = append(decls, c)
			}
		}
	}
	return contribapi.KindsFrom(decls)
}

// bootSteps are the hub boot's moves around the tier-1 cache, as bootHydration
// orders them.
type bootSteps struct {
	hydrate func(members []appcore.SidecarSpec) (probed int) // probe and save the missing / stale records
	refresh func()                                           // reload the catalog from the cache (+ Phase 2)
	ready   func() error                                     // write the lockfile: clients may connect
	after   func()                                           // start the watches that compare the cache against its sources
}

// bootHydration orders the boot's cache hydration around the lockfile.
//
// A member with NO record is probed first: the catalog has nothing of it to
// serve. Then the hub is ready. A member whose record is only STALE (its
// executable rebuilt, its served folder edited) is re-probed after — the
// probe of a fresh executable waits for the OS to scan it, 11-29 s measured,
// all of it time every client used to wait for the lockfile — while the
// catalog serves the previous record; the catalog reloads (contributions-
// changed) when the re-probe lands, and the watches start once it has, so
// they never probe the same stale record a second time.
//
// The background half runs on its own goroutine; bootHydration returns as soon
// as the hub is ready.
func bootHydration(cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec, s bootSteps) error {
	missing, recorded := splitByRecord(cache, sidecars)
	if len(missing) > 0 && needsCatalogReload(s.hydrate(missing)) {
		s.refresh()
	}
	if err := s.ready(); err != nil {
		return err
	}
	go func() {
		if len(recorded) > 0 && needsCatalogReload(s.hydrate(recorded)) {
			s.refresh()
		}
		s.after()
	}()
	return nil
}

// splitByRecord parts the members into those the tier-1 cache holds no record
// of (for the binary the universe declares) and those it does, fresh or not.
// The hub itself and a member without a binary are never probed (hydrateCache
// skips them); they go with the recorded ones.
func splitByRecord(cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) (missing, recorded []appcore.SidecarSpec) {
	for _, s := range sidecars {
		if recordSource(s) == "" || s.Name == hubProvider {
			recorded = append(recorded, s)
			continue
		}
		if m, _, err := cache.Load(s.Name, recordSource(s)); err != nil || m == nil {
			missing = append(missing, s)
			continue
		}
		recorded = append(recorded, s)
	}
	return missing, recorded
}

// catalogReloader exposes the optional Reload method on the published
// Catalog. Used by the cache watcher in `serve` to prompt a re-read
// after on-disk changes. Mirrors the interface used by `catalog reload`
// and `catalog refresh`.
type catalogReloader interface {
	Reload() error
}

// catalogPublisherSetter exposes the optional SetEventPublisher method
// on the published Catalog. Used by `serve` to attach the NATS-backed
// contributions-changed publisher once the embedded NATS server is up.
// Duck-typed so MemoryCatalog + test stubs don't need to implement it.
type catalogPublisherSetter interface {
	SetEventPublisher(pub contribapi.ContributionsEventPublisher)
}

// catalogValidationConfigSetter exposes the optional SetValidationConfig
// method on the published Catalog. Used by applyHubPhase2 to install the
// central ContributionsConfig before Reindex runs its schema-validation
// checkpoint. Duck-typed so MemoryCatalog + test stubs don't need it.
type catalogValidationConfigSetter interface {
	SetValidationConfig(cfg contribapi.ContributionsConfig)
}

// catalogMemberSwapper exposes the optional SetMembers method on the
// published Catalog. Used by the Univers.yaml watcher in `serve` to
// propagate membership changes to the in-memory catalog without
// restarting the hub. The connmgr counterpart is connmgrlocal.MemberSwapper.
type catalogMemberSwapper interface {
	SetMembers(members []manifestlocal.Member) error
}

// hydrateCache probes each member for its manifest when its per-sidecar
// cache folder is missing or stale (exe_size or exe_mtime drifted vs.
// the metadata.yaml stored at last save). Probes are exec-and-kill
// (`<binary> resources list` → JSON → save → exit) — NO long-running
// NATS subscription. The result is that ktw-hub starts up with a
// fully-populated catalog WITHOUT running any sidecar past its probe
// lifetime.
//
// Probes run in parallel — each sidecar boot is independent (~3-5s),
// and serializing them blocks the hub's lockfile write for tens of
// seconds. Parallel keeps activation latency bounded by the slowest
// single sidecar.
//
// Returns: (hydrated = cache folder already fresh; probed = freshly
// re-probed and saved; errs = probe failures, skipped).
//
// Replaces the prior `hydrateInventory` (2026-06-05 cache redesign).
// The single-flat-dir freshness check (compare binary mtime to newest
// inventory file mtime) is replaced by `Cache.IsStale` which reads
// metadata.yaml and compares exe_size + exe_mtime directly — no more
// content-addressed-filename lex-sort hazard.
func hydrateCache(
	cache *manifestlocal.Cache,
	sidecars []appcore.SidecarSpec,
	errSink io.Writer,
) (hydrated, probed, errs int) {
	type result struct {
		name     string
		binary   string
		status   string // "fresh" / "probed" / "err"
		err      error
		manifest *mbapi.Manifest
		digests  manifestlocal.SourceDigests // the key the probe was run for
	}
	// System meta-model dedup: every binary embeds the identical `system:/`
	// entries; the hub's own manifest carries them (saveHubOwnManifest), so
	// EVERY member is probed without them, always (e26). The record then
	// depends on nothing but the member — not on the universe's order, not on
	// which universe probed it last — and the shared cache holds one copy.
	results := make(chan result, len(sidecars))
	launched := 0
	for _, s := range sidecars {
		// The hub is never probed as a member: its record is built in process,
		// with the system set (saveHubOwnManifest).
		if s.Binary == "" || s.IsFolder() || s.Name == hubProvider {
			continue // a folder member is read in the second pass, below
		}
		launched++
		go func(spec appcore.SidecarSpec) {
			name, binary := spec.Name, spec.Binary
			want := sourceDigestsIn(name, sidecars)
			// The reads half of the key, taken before any probe below: an edit
			// landing while a probe runs must read as a change next time.
			want.ReadRoots, want.Reads = cache.ReadsBeforeProbe(name, binary)
			fresh, freshErr := cacheIsFresh(cache, name, binary, want)
			if freshErr != nil {
				results <- result{name: name, binary: binary, status: "err", err: fmt.Errorf("cache check: %w", freshErr)}
				return
			}
			if fresh {
				// The stat's one gap (blind audit M2, OD-2): a whole-second
				// stamp on a same-size binary cannot tell a rebuild from the
				// record. The recorded provider digests break the tie, on a
				// probe of the declared binary; the manifest that probe
				// returns is the one saved when they moved, so the tie costs
				// one probe, never two. A binary whose stamp is finer than a
				// second never comes here — the stat decided.
				recorded, meta, lerr := cache.Load(name, binary)
				if lerr == nil && recorded != nil && cache.StatUndecided(meta, want) {
					m, runErr := probeManifest(binary, spec.ContentRoot, false)
					if runErr != nil {
						results <- result{name: name, binary: binary, status: "err", err: fmt.Errorf("tie-break probe %s: %w", binary, runErr)}
						return
					}
					if moved, why := cache.ProvidersMoved(recorded.Providers, m.Providers); moved {
						fmt.Fprintf(errSink, "ktw-hub: %s: stat undecided (whole-second stamp), %s — re-probed\n", name, why)
						results <- result{name: name, binary: binary, status: "probed", manifest: m, digests: want}
						return
					}
				}
				results <- result{name: name, binary: binary, status: "fresh"}
				return
			}
			m, runErr := probeManifest(binary, spec.ContentRoot, false)
			if runErr != nil {
				results <- result{name: name, binary: binary, status: "err", err: fmt.Errorf("probe %s: %w", binary, runErr)}
				return
			}
			results <- result{name: name, binary: binary, status: "probed", manifest: m, digests: want}
		}(s)
	}
	for i := 0; i < launched; i++ {
		r := <-results
		switch r.status {
		case "fresh":
			hydrated++
		case "probed":
			if err := cache.Save(r.name, r.manifest, r.binary, r.digests); err != nil {
				fmt.Fprintf(errSink, "ktw-hub: save cache %s: %v\n", r.name, err)
				errs++
				continue
			}
			probed++
		case "err":
			fmt.Fprintf(errSink, "ktw-hub: %s: %v\n", r.name, r.err)
			errs++
		}
	}
	// Second pass: the folder members, read by the hub itself (no process),
	// once every binary's record is saved — a folder publishes entries of the
	// kinds the universe's members declare.
	var kinds *contribapi.Kinds
	for _, spec := range sidecars {
		if !spec.IsFolder() || spec.Name == hubProvider {
			continue
		}
		name, root := spec.Name, recordSource(spec)
		want := sourceDigestsIn(name, sidecars)
		if fresh, err := cacheIsFresh(cache, name, root, want); err == nil && fresh {
			hydrated++
			continue
		}
		if kinds == nil {
			k := universeKinds(cache, sidecars)
			kinds = &k
		}
		m, err := folderManifest(name, root, *kinds)
		if err != nil {
			fmt.Fprintf(errSink, "ktw-hub: %s: read folder %s: %v\n", name, root, err)
			errs++
			continue
		}
		if err := cache.Save(name, m, root, want); err != nil {
			fmt.Fprintf(errSink, "ktw-hub: save cache %s: %v\n", name, err)
			errs++
			continue
		}
		probed++
	}
	return
}

// watchOverwrites polls and, whenever any member's cached manifest is stale
// AGAINST WHAT IT WAS BUILT FROM, re-runs the digest-aware hydrate so the
// cache + in-memory catalog reflect edited overrides or served content. Blocks
// until ctx is cancelled. Gated by the caller on allow_overwrites — never runs
// in prod.
//
// The comparison is against the RECORD (each member's metadata.yaml digests),
// never against a baseline taken when the watch started. A watch-time baseline
// cannot see a change made between the index being built and the watch
// starting: the index holds the old content while the watcher believes the new
// content is what it has always been, for as long as the process lives
// (concepts §10). The record has no such window, and it is the same key the
// boot-time hydrate and `catalog refresh` already use — one mechanism.
func watchOverwrites(ctx context.Context, cache *manifestlocal.Cache, cacheDir, root string, errSink io.Writer) {
	_ = root // the root is what the members' recorded digests fingerprint; the record is the key
	const poll = 2 * time.Second
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			reg, err := universeMembers()
			if err != nil {
				fmt.Fprintf(errSink, "ktw-hub: overwrites watch — reload registry: %v\n", err)
				continue
			}
			if len(staleRecords(cache, reg.Sidecars)) == 0 {
				continue
			}
			hydrateCache(cache, reg.Sidecars, errSink)
			cat, err := application.UseService[mbapi.Catalog](ctx)
			if err != nil {
				continue
			}
			if r, ok := cat.(catalogReloader); ok {
				_ = r.Reload()
			}
			if schemaSvc, schemaErr := application.UseService[schemaapi.SchemaService](ctx); schemaErr == nil && schemaSvc != nil {
				applyHubPhase2(ctx, cat, schemaSvc)
			}
		}
	}
}

// staleRecords is one round of the override watch: the members whose record no
// longer matches its sources, read from the records' metadata alone - an idle
// round reads a few lines per member, never a manifest.
func staleRecords(cache *manifestlocal.Cache, sidecars []appcore.SidecarSpec) []string {
	return staleMembers(sidecars, func(name, binary string) (bool, error) {
		return recordIsFresh(cache, name, binary, sourceDigestsIn(name, sidecars))
	})
}

// staleMembers returns the members whose cached manifest is stale against its
// own record, as `fresh` reports it. A member with no binary is not a member
// of the cache; a freshness error counts as stale — the safe direction, since
// the cost of a wrong "fresh" is serving old content for the life of the
// process and the cost of a wrong "stale" is one probe.
func staleMembers(sidecars []appcore.SidecarSpec, fresh func(name, binary string) (bool, error)) []string {
	var out []string
	for _, sc := range sidecars {
		if recordSource(sc) == "" {
			continue
		}
		ok, err := fresh(sc.Name, recordSource(sc))
		if err != nil || !ok {
			out = append(out, sc.Name)
		}
	}
	return out
}

// universeCache is the tier-1 cache as this hub's universe reads it: rooted at
// the host's shared `<CacheDir>/sidecars`, a record's declared read roots
// resolved in this universe, and a record that depends on the universe kept
// apart from every other universe's. Every hub site that
// opens the cache opens it through here, so they all read the same record.
func universeCache(root string) *manifestlocal.Cache {
	env := application.Env()
	return manifestlocal.NewUniverseCache(root, env.UniverseDir(), env.GlobalHomeDir())
}

// overwritesUniverseDir is the universe folder whose `overwrites/` tree the
// staleness key folds; a test sets it.
var overwritesUniverseDir = func() string { return application.Env().UniverseDir() }

// overwritesDigestFor returns the gate-aware override-tree fingerprint for a
// sidecar: the digest of `<universe>/overwrites/<name>` when the universe opts
// in (allow_overwrites), else "". Folded into the cache staleness key so an
// override edit (which leaves the binary's stat unchanged) still invalidates the
// cached manifest — the recurring agent-mode staleness bug. An attached
// project's overrides are no part of it: they come as override entries through
// the project member's push.
func overwritesDigestFor(name string) string {
	universe := overwritesUniverseDir()
	if !cenv.AllowOverwrites(universe) {
		return ""
	}
	return manifestlocal.DigestDir(cenv.OverwritesDir(universe, name))
}

// contentRootOf returns the folder a member serves, or "" for one that serves
// none. Read from the declared membership — never recovered by parsing the
// member's spawn arguments.
func contentRootOf(name string) string {
	reg, err := universeMembers()
	if err != nil {
		return ""
	}
	if spec := reg.FindByName(name); spec != nil {
		return spec.ContentRoot
	}
	return ""
}

// declaredMembers is the universe's membership as declared now — re-read on
// every poll round so a member added while the hub runs is watched too.
func declaredMembers() []appcore.SidecarSpec {
	reg, err := universeMembers()
	if err != nil {
		return nil
	}
	return reg.Sidecars
}

// watchMembers polls every declared member with a binary and re-probes the
// ones whose sources moved, as `moved` decides (a folder digest or an exe
// stat against the record). The hub itself is never a member of this watch:
// its record is built in process by the running executable, and a rebuilt
// hub binary is served by a restart, not a re-probe. Blocks until ctx is
// cancelled. The interval, the membership, the decision and the refresh are
// parameters so a test drives one round in milliseconds with no application.
func watchMembers(
	ctx context.Context,
	interval time.Duration,
	members func() []appcore.SidecarSpec,
	moved func(appcore.SidecarSpec) (bool, string),
	refresh func(name string) error,
	out, errSink io.Writer,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, s := range members() {
				if recordSource(s) == "" || s.Name == hubProvider {
					continue
				}
				did, what := moved(s)
				if !did {
					continue
				}
				if err := refresh(s.Name); err != nil {
					fmt.Fprintf(errSink, "ktw-hub: %s for %s but re-probe failed: %v\n", what, s.Name, err)
					continue
				}
				fmt.Fprintf(out, "ktw-hub: %s for %s — re-probed\n", what, s.Name)
			}
		}
	}
}

// exeStatWatch notices a compiled member's rebuild for one os.Stat per member
// per round. What each stat is compared against is the RECORD's stat — the
// size and mtime metadata.yaml says the cached manifest was taken from —
// seeded from the record the first time a member is seen and again after
// every change, never a baseline taken when the watch started: a baseline
// cannot see a rebuild made between the record being written and the first
// round, and would hide it for the life of the process (watcher_content.go).
// A member with no record yet is hydration's, not the watch's.
type exeStatWatch struct {
	cache *manifestlocal.Cache
	seen  map[string]exeStat // member → the record's stat, as last read
}

type exeStat struct {
	size  int64
	mtime time.Time
}

func newExeStatWatch(cache *manifestlocal.Cache) *exeStatWatch {
	return &exeStatWatch{cache: cache, seen: map[string]exeStat{}}
}

// moved reports whether the member's declared binary no longer matches the
// stat its record was taken from. After a change the seed is dropped, so the
// next round re-reads the record — the new one after a successful re-probe,
// the old one after a failed probe (a binary still being written), in which
// case the member reads as moved again and the probe is retried.
func (w *exeStatWatch) moved(name, binary string) bool {
	info, err := os.Stat(binary)
	if err != nil {
		return false // absent or unreadable: nothing to probe; hydration reports it
	}
	rec, ok := w.seen[name]
	if !ok {
		meta, lerr := w.cache.LoadMetadata(name, binary)
		if lerr != nil || meta == nil {
			return false // no record (or another binary's): hydration owns that case
		}
		rec = exeStat{size: meta.ExeSize, mtime: meta.ExeModTime}
		w.seen[name] = rec
	}
	if info.Size() == rec.size && info.ModTime().Equal(rec.mtime) {
		return false
	}
	delete(w.seen, name)
	return true
}

// servedContentMoved reports whether a member's served folder differs from what
// its CACHED manifest was built from.
//
// The comparison is against the cache's own record, not against a baseline this
// process took when it started watching. That distinction is the whole
// correctness argument: a baseline taken at watch time cannot see an edit made
// between the cache being written and the watch beginning, and that edit would
// then be invisible forever — the cache holds the old content and the watcher
// believes the new content is what it has always been. Comparing against what
// the cache was built from has no such window.
//
// It is also one mechanism rather than two: exactly the check the operator's
// `catalog refresh` and the boot-time hydration already make.
func servedContentMoved(name string) bool {
	cache := universeCache(filepath.Join(application.Env().CacheDir(), "sidecars"))
	want := sourceDigestsFor(name)
	meta, err := cache.LoadMetadata(name, want.Binary)
	if err != nil || meta == nil {
		return false // nothing cached yet; hydration owns that case
	}
	stale, _ := cache.IsStale(meta, want)
	return stale
}

// sourceDigestsFor collects every source whose changes the member's executable
// cannot show, plus the system setting of its record — the staleness key over
// the universe's declared membership.
func sourceDigestsFor(name string) manifestlocal.SourceDigests {
	var sidecars []appcore.SidecarSpec
	if reg, err := universeMembers(); err == nil {
		sidecars = reg.Sidecars
	}
	d := sourceDigestsIn(name, sidecars)
	if d.Binary == "" && name != hubProvider {
		// A name the membership does not bind to a binary (the host binary
		// beside ktw-hub): the same resolution `catalog refresh` probes with,
		// so the key it checks is the key the record was saved under.
		d.Binary = resolveSidecarBinary(name)
	}
	return d
}

// sourceDigestsIn is sourceDigestsFor over an explicit member list: the one a
// hydration round is working through, so the key it checks, the probe it runs
// and the record it saves all come from the same list.
//
// The served-folder digest is computed by the aggregator ITSELF, from the root
// the membership declares. It does not ask the member — asking would mean
// contacting a contributor to find out whether anything changed, which is the
// one thing discovery must never do.
//
// The key starts with the binary the membership DECLARES for the member (the
// running executable for the hub itself): a record taken from another path
// is another binary's record, whatever universe wrote it, and reads stale
// with a reason naming both paths (blind audit H1).
func sourceDigestsIn(name string, sidecars []appcore.SidecarSpec) manifestlocal.SourceDigests {
	d := manifestlocal.SourceDigests{
		Overwrites:    overwritesDigestFor(name),
		IncludeSystem: carriesSystemSet(name),
	}
	if name == hubProvider {
		d.Binary, _ = hubOwnExecutable() // "" → IsStale says so; Save reports the error
		return d
	}
	for _, s := range sidecars {
		if s.Name != name {
			continue
		}
		d.Binary = recordSource(s)
		if s.ContentRoot != "" {
			d.Content = manifestlocal.DigestDir(s.ContentRoot)
		}
		break
	}
	return d
}

// carriesSystemSet answers whether a member's record carries the system set:
// true for the hub's own manifest, which is the one carrier (e26), false for
// every probed member, whatever universe it belongs to. It stays part of the
// staleness key (metadata.yaml `include_system`) because records written before
// e26 exist in shared caches: a member once chosen as its universe's carrier was
// saved WITH the set, and would read fresh — a second copy beside the hub's —
// if the key did not refuse it. The answer no longer depends on the membership.
func carriesSystemSet(name string) bool { return name == hubProvider }

// probeManifest is the exec-and-kill manifest probe. A variable so the
// hydration tests can stand in for a compiled member.
var probeManifest = appcore.RunResourcesList

// cacheIsFresh returns true when a record of `name` taken from `binary` — the
// one the universe declares — exists AND that binary's current size + mtime
// still match what was recorded at the last Save, AND the record was built
// from the sources and with the system setting `want` names. Another
// universe's record of the same name at another binary is not this one:
// Load addresses the record by (name, binary) and IsStale compares the
// declared binary before anything else (blind audit H1). A missing binary
// file errors (Univers.yaml is wrong); a missing or refused record → not
// fresh (will probe).
func cacheIsFresh(cache *manifestlocal.Cache, name, binary string, want manifestlocal.SourceDigests) (bool, error) {
	if _, err := os.Stat(binary); err != nil {
		return false, fmt.Errorf("stat binary: %w", err)
	}
	_, meta, err := cache.Load(name, binary)
	if err != nil {
		// Hash mismatch, another binary's record, or a partial-write race
		// — not fresh (the caller will re-probe and overwrite the pair).
		return false, nil
	}
	if meta == nil {
		return false, nil // no record → must probe
	}
	stale, _ := cache.IsStale(meta, want)
	return !stale, nil
}

// recordIsFresh is cacheIsFresh read from the record's metadata alone: the
// question a watch round asks every interval, for every member. A manifest can
// be megabytes; decoding each member's every round kept an idle hub busy. A
// record the round finds stale is re-probed by hydration, which loads it whole.
func recordIsFresh(cache *manifestlocal.Cache, name, binary string, want manifestlocal.SourceDigests) (bool, error) {
	if _, err := os.Stat(binary); err != nil {
		return false, fmt.Errorf("stat binary: %w", err)
	}
	meta, err := cache.LoadMetadata(name, binary)
	if err != nil || meta == nil {
		return false, nil // no record, another binary's, or half of a pair: re-probe
	}
	stale, _ := cache.IsStale(meta, want)
	return !stale, nil
}

const longHelp = `ktw-hub is the KTSP bus host. It embeds a NATS server (desktop / dev mode)
or connects to an external NATS cluster (cloud), and provides the messaging
backbone, connection manager, sidecar catalog, and shared-state services
(rooms / sessions / journal / presence) for ONE active universe.

ktw-hub is not user-facing day-to-day. ktw-gui activates a universe by
spawning ktw-hub via the hubclient library; ktw-agents (CLI) does the same
for command-line work. Headless deploys (systemd / k8s) start ktw-hub
directly with 'ktw-hub serve'.

See:
  modules/bus/decisions/OD-003-which-binary-hosts-the-bus.cwm.md
  modules/bus/decisions/OD-002-how-are-the-bus-s-subjects-streams-and-buckets.cwm.md
  modules/bus/decisions/OD-001-what-carries-the-messages-between-the-hub-the.cwm.md`

// hubCache is the hub's tier-1 cache of its members' records, in the
// universe's cache folder. Composed here, at the hub's wiring: the catalog's
// read path names no place on the machine (F-030).
func hubCache() *manifestlocal.Cache {
	return universeCache(filepath.Join(application.Env().CacheDir(), "sidecars"))
}
