// SPDX-License-Identifier: Apache-2.0

// Package local registers CWM as an L4 published service via
// appcore. Imports + composes the cwm/internal/* sub-services
// (via internal/core.Service) and wires KTSP + HTTP + config.
//
// The single public surface is `Extension()`. Consumers (e.g.
// ktw-agents bootstrap, future ktw-cwm sidecar binary) add it
// to their application via `application.Extension(...)`.
//
// The workspace folder (`.katyoti`) is resolved by the
// application bootstrap layer — see
// `api.DiscoverWorkspaceRoot`. This package accepts the
// resolved root via `core.CWMServiceConfig.WorkspaceRoot`.
package local

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/contrib"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storage"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/storeset"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// defaultHTTPPort is the canonical cwm port per
// modules/cwm/tools.md (kb uses 8896; cwm gets 8895).
const defaultHTTPPort = 8895

// Options configures Extension at build time. Optional knobs
// that depend on caller-owned values — embedder, HTTP port
// override — live here so the standard Extension() signature
// stays argument-free.
type Options struct {

	// HTTPPort overrides the default 8895. 0 → use default.
	HTTPPort int

	// WorkspaceRoot is the absolute path to the workspace folder
	// (`.katyoti`). Used to fill `core.CWMServiceConfig.WorkspaceRoot`
	// when `CWMService.yaml` does not set it. Tests typically use
	// this static path.
	WorkspaceRoot string

	// WorkspaceRootProvider lazily resolves the workspace folder at
	// service-construction time. Used by the ktw-agents bootstrap
	// to defer discovery until after Cobra has parsed
	// `--workspace-dir`. Has higher priority than the static
	// `WorkspaceRoot` field when both are set. Receives no
	// arguments; reads its own state (CLI flag, env var) and
	// returns the resolved path.
	WorkspaceRootProvider func() (string, error)

	// TypesFS is the embedded FS rooted at a directory containing
	// `_kinds/` + `_edge_kinds/` (e.g. `cwmembed.FS`). Becomes
	// the floor of the TypeRegistry layering chain — the
	// workspace's own `<root>/types/` dir layers on top and wins
	// on collision. nil = no embedded source (substrate tests).
	TypesFS fs.FS

	// ObjectTypeValidation is the validation mode when `CWMService.yaml` sets
	// none ("off" | "warn" | "error"). Empty leaves CWM's default: error. The
	// test harness sets warn, so a test that exercises one extension is not
	// refused for the types of the packs it does not load.
	ObjectTypeValidation string

	// ServedElsewhere, when it answers true, says another process serves
	// this binary's workspace (the universe's hub has a serving member): the
	// service built here is its TYPE only — it opens nothing, takes no lock,
	// writes no first-run file, attaches no observer and runs no bootstrap
	// hook — and every verb reaches the serving process over the hub
	// . nil: never.
	ServedElsewhere func(ctx stdctx.Context) bool
}

// Extension returns the application.Extension that registers
// CWM. Call once per application; subsequent calls return
// fresh Extensions (each register cycle is independent).
//
// Defaults: port 8895, validation in error mode.
func Extension() application.Extension {
	return ExtensionWith(Options{})
}

// ExtensionWith is the customizable variant — used by the
// ktw-agents binary to inject an Ollama / OpenAI embedder, or
// by tests that want a stub embedder.
func ExtensionWith(opts Options) application.Extension {
	port := opts.HTTPPort
	if port <= 0 {
		port = defaultHTTPPort
	}
	return func(init *application.ApplicationInit) {
		// One gate per application: the service is usually built DURING boot
		// (the KTSP container resolves every published service at AppInit,
		// before it publishes its endpoint), and the bootstrap hooks this
		// binary serves wait for the Bootstrap phase, when every slot of this
		// binary is published.
		boot := &bootGate{}
		init.AddBootstrap("cwm", ".bootstrap-hooks", func(*application.Application) error {
			boot.open()
			return nil
		})
		// The bootstrap hooks another sidecar serves run once this process
		// serves its universe: their action calls CWM back over the bus, and
		// before that nothing answers it. A one-shot command never runs them —
		// the serving process does, each time it starts.
		init.AddContribute("cwm", ".bootstrap-serving", func(app *application.Application) error {
			if err := application.Contribute(app, ktsapi.ServingHook{Name: "cwm-bootstrap", Start: runOthersBootstrap}); err != nil {
				slog.Warn("cwm: no serving hooks in this binary — the bootstrap hooks other sidecars serve do not run", "err", err)
			}
			return nil
		})
		init.AddExtension(appcore.PublishService[api.CWMService, core.CWMServiceConfig](
			func(ctx stdctx.Context, cfg *core.CWMServiceConfig) (api.CWMService, error) {
				// A process started to describe itself (the hub's manifest
				// probe) needs the service's TYPE — its slot and method
				// signatures — never the workspace: it opens nothing, takes no
				// lock, writes no first-run file and runs no bootstrap hook. A
				// probe that opened the workspace failed whenever the serving
				// process held the lock, and raced it when it did not.
				if appcore.ManifestProbeProcess() {
					return describeOnlyService(), nil
				}
				// A CLIENT of its running universe opens nothing either: its
				// workspace is the serving process's (ServedElsewhere).
				if opts.ServedElsewhere != nil && opts.ServedElsewhere(ctx) {
					slog.Debug("cwm: the workspace is served by another process; this one is its client and opens nothing")
					return describeOnlyService(), nil
				}
				if cfg.WorkspaceRoot == "" {
					switch {
					case opts.WorkspaceRootProvider != nil:
						root, err := opts.WorkspaceRootProvider()
						if err != nil {
							return nil, fmt.Errorf("resolve workspace root: %w", err)
						}
						cfg.WorkspaceRoot = root
					case opts.WorkspaceRoot != "":
						cfg.WorkspaceRoot = opts.WorkspaceRoot
					}
				}
				// A project ATTACHED to a universe (its `.katyoti/project.yaml`) is
				// the one CWM gives a project object when it has none — never a
				// universe folder CWM serves for itself.
				members := attachedMembers(cfg.WorkspaceRoot)
				attached := len(members) > 0
				cfg.WorkspaceRoot, cfg.ProjectRoot = resolveWorkspaceRoots(cfg.WorkspaceRoot, cfg.ProjectRoot)
				if cfg.ObjectTypeValidation == "" {
					cfg.ObjectTypeValidation = opts.ObjectTypeValidation
				}
				// Every log line this core writes carries the universe and the
				// project id (identity-and-index.md §1): read from the root
				// project object, before the index exists. ONE place.
				stampDefaultLogger(application.Env().UniverseName(), projectUUIDAt(cfg.ProjectRoot))
				// The store's in-binary FLOOR is this host's own type FS
				// (opts.TypesFS — workspace/inbox + CWM-core kinds). Method
				// packs are NO LONGER layered in-process: each pack sidecar
				// contributes its types to the catalog (ContributeCWMTypes),
				// and the store consumes them via cfg.CatalogTypes below
				// (migrate-cwm-type-packs-to-sidecars). TypeSourcesPoint is
				// retired.
				if opts.TypesFS != nil && len(cfg.TypeSources) == 0 {
					cfg.TypeSources = []fs.FS{opts.TypesFS}
				}
				// The host's OWN CWM types are overridable from the workspace
				// overlay too (per-sidecar, like skills/tasks). Other sidecars'
				// overrides arrive via cfg.CatalogTypes (their provider applies
				// the overlay), but the host's embedded types are in the FLOOR —
				// and the catalog Merge is floor-wins, so an embedded host type
				// would shadow its own override. Appending the overlay after the
				// embedded floor makes it win (LoadTypesLayered is last-wins).
				// Gated by allow_overwrites; nil (gate off) is skipped. A
				// project's overrides of the host's types are override entries
				// the catalog applies (CatalogTypes.Overridden), never a tree
				// layered here.
				if prov, ok := application.SelfProviderFromContext(ctx); ok {
					cfg.TypeSources = append(cfg.TypeSources,
						cwmcontrib.FloorOverlays(application.Env().UniverseDir(), prov)...)
				}
				// Plumb the application-level EventBus so the CWM
				// events service can publish on workspace mutations.
				// Resolved lazily here (at factory-fire time, which
				// happens after `ktspInfraExt` has registered the
				// MemoryEventBus). A missing bus makes events
				// publishing a no-op — fine for tests.
				if cfg.EventBus == nil {
					if bus, err := application.UseService[eventbus.EventBus](ctx); err == nil {
						cfg.EventBus = bus
					}
				}
				// Plumb the NATS MessageBus alongside so CWM lifecycle
				// events also fan out to cross-sidecar subscribers via
				// EventService (`internal/extensions/events/`). A
				// missing bus silently skips the NATS publish — the
				// in-process publish above keeps in-process
				// subscribers working.
				if cfg.MessageBus == nil {
					if mb, err := application.UseService[mbapi.MessageBus](ctx); err == nil {
						cfg.MessageBus = mb
					}
				}
				// The minter and its number blocks live with the tenant's
				// stores: a project at a git repository's top
				// level mints its numbers from them.
				if cfg.MinterDir == "" && application.Env().TenantDataDir() != "" {
					cfg.MinterDir = filepath.Join(application.Env().TenantDataDir(), "cwm")
				}
				// A migration keeps what it removes from the project in the
				// universe's data folder, never beside the universe config.
				if cfg.BackupDir == "" && application.Env().DataDir() != "" {
					cfg.BackupDir = application.Env().GetDataSubDir("migration-backups")
				}
				// Scope tokens for the NATS subjects the events
				// service publishes on. MUST come from the canonical
				// resolver — hardcoded defaults here drift from the
				// subjects subscribers build (the bus stamps the real
				// universe name on served contexts) and events then
				// silently never match.
				if m, err := cenv.LoadUniverseManifest(application.Env().UniverseDir()); err == nil && m != nil {
					cfg.UniverseSolution = m.Solution
				}
				if cfg.TenantID == "" || cfg.UniverseID == "" {
					tenantID, universeID := cenv.ResolveTenantUniverse(application.Env().UniverseDir())
					if cfg.TenantID == "" {
						cfg.TenantID = tenantID
					}
					if cfg.UniverseID == "" {
						cfg.UniverseID = universeID
					}
				}
				// Consume CWM types contributed by ANY sidecar from the
				// universe-wide aggregate catalog, layered onto the in-binary
				// floor (cfg.TypeSources) with FLOOR-WINS. This realizes "any
				// sidecar can contribute CWM types" — the store reads them like
				// skills/tasks. The floor guarantees the store still opens when
				// the catalog is unavailable or a pack sidecar is down (degraded
				// mode — types from absent sidecars simply don't register, and
				// creates of those kinds fail with type-not-found).
				if cat, cerr := application.UseService[mbapi.Catalog](ctx); cerr == nil {
					cfg.CatalogTypes = LoadCWMTypesFromCatalog(ctx, cat)
					// Policies arrive ON the capability declarations themselves
					// (`attach: {types}`), resolved above with every other
					// capability — there is no second kind to read.
				} else {
					slog.Warn("cwm: aggregate catalog unavailable; opening on in-binary floor only (CWM types from other sidecars absent)", "error", cerr)
				}
				// A CLEAN context carrying only the application, for everything
				// that calls CWM back from outside this factory: the factory's own
				// ctx holds the resolution stack of this very service, so CWM
				// resolved through it reads as a cycle.
				appCtx := application.WithApplication(stdctx.Background(), application.AppFromContext(ctx))
				observers := application.UseExtensions[api.EventObserver](ctx)
				// Each store, once open and before it is served:
				//   - the in-process reactions extensions declared (the journal)
				//     are attached HERE, where the workspace is held and the
				//     writes happen, so each is the one writer of what it records;
				//   - an attached project without its project object gets one
				//     now (from the seed the hub left, else the default object),
				//     journaled, before the `bootstrap` point (which fires on
				//     that object) - a new project shows as a project, with its
				//     inbox and planification - and before the store set joins a
				//     member's store under that object's id.
				prepare := func(st *core.Service) {
					st.AttachObservers(appCtx, observers)
					if !attached {
						return
					}
					if created, err := st.EnsureProject(appCtx); err != nil {
						slog.Warn("cwm: the attached project has no project object and none could be created", "root", st.CorpusRoot(), "error", err)
					} else if created {
						slog.Info("cwm: the attached project had no project object - created", "root", st.CorpusRoot())
					}
				}
				// One store per attached member: one member, or
				// none, opens the one store as always; several open one each,
				// joined by the store set behind its one service.
				stores, err := openStores(ctx, cfg, members, prepare)
				if err != nil {
					return nil, err
				}
				svc, err := serve(stores)
				if err != nil {
					for _, st := range stores {
						_ = st.Close()
					}
					return nil, err
				}
				// The `bootstrap` point runs once the service is reachable AND
				// the application has booted (bootGate): a bootstrap action
				// calls CWM back, and the ones run here are slots of this very
				// binary, which run in-process only once its endpoint is
				// published. The others wait for the process to serve
				// (runOthersBootstrap).
				// The match takers (the events extension) are attached once
				// the application has booted — a taker reaches services this
				// very boot publishes — and get every match pending.
				takers := application.UseExtensions[api.MatchTaker](ctx)
				for _, st := range stores {
					st.BeforeClose(boot.wait)
				}
				boot.afterBoot(func() {
					svc.AttachMatchTakers(appCtx, takers)
					for _, e := range svc.RunBootstrapHooks(appCtx, core.BootstrapOwn) {
						slog.Warn("cwm: bootstrap hook failed", "error", e)
					}
				})
				return svc, nil
			},
			appcore.WithServiceDef(api.CWMServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithHTTPServer(port),
			appcore.WithConfigDirs("config", "cwm"),
		))

		// Advertise this binary's CWM node / item / EDGE types as catalog
		// Contributions, read directly from the contributed TypeSource FSs
		// (FS-reading, no live-CWMService dependency — so any sidecar can
		// contribute types, and the store can CONSUME the catalog without a
		// store→catalog→store cycle). Pairs with the system Contributions at
		// `internal/core/contribution/embed/{contribution-type,data-type}/{node,item,edge}-type.yaml`.
		cwmcontrib.ContributeCWMTypes("", opts.TypesFS)(init)

		// Contribute the DSL data-types shipped under `embed/cwm/data-types/`
		// (an object type's fields at the top, a capability's field-group
		// under `capabilities/`). `WithFlatSubfolders()` drops the subfolder
		// so each `<name>.dsl.yaml` becomes `ktw-agents:/data-type/<name>`
		// wherever it sits on disk; an object type's `fields` and a
		// capability's `schema` resolve through the one SchemaService
		// pipeline.
		if opts.TypesFS != nil {
			manifestcontrib.ContributeDataTypeResources(
				"",
				loader.NewEmbedFSLoader(opts.TypesFS, "."),
				"data-types",
				manifestcontrib.WithFlatSubfolders(),
			)(init)
		}

		// Re-resolve CWM types from the aggregate catalog on a
		// contributions-changed event: the snapshot replaces the declarations
		// in force, keyed by URI - a hot-deployed pack's types and a changed
		// declaration are in force without a restart. The store
		// consumes the catalog at construction (cfg.CatalogTypes); this hook
		// keeps it live. Only ktw-agents (which runs the event subscriber +
		// holds the in-process store) actually fans out to this hook.
		manifestlocal.ContributeRefreshHook(mbapi.RefreshHook{
			Name:  "cwm-types",
			Kinds: []string{"edge-type", "object-type", "capability", "aspect", "workflow", "policy", "hook-type", "hook", "event-type", "data-type"},
			Refresh: func(ctx stdctx.Context, _ contribapi.ContributionsChangedEvent) error {
				cat, err := application.UseService[mbapi.Catalog](ctx)
				if err != nil {
					return fmt.Errorf("cwm refresh: catalog unavailable: %w", err)
				}
				svc, err := application.UseService[api.CWMService](ctx)
				if err != nil {
					return fmt.Errorf("cwm refresh: service unavailable: %w", err)
				}
				refresher, ok := svc.(core.CatalogTypeRefresher)
				if !ok {
					return nil // service doesn't support hot type refresh
				}
				refresher.RefreshCatalogTypes(LoadCWMTypesFromCatalog(ctx, cat))
				return nil
			},
		})(init)
	}
}

// LoadCWMTypesFromCatalog resolves every node-type / item-type / edge-type /
// object-type
// Contribution in the universe-wide aggregate catalog into decoded
// registrations, for the store to overlay on its in-binary floor. Bodies are
// JSON (as emitted by ContributeCWMTypes); a body-free index entry is
// materialised via Catalog.GetBody. Per-entry decode failures are logged and
// skipped — one malformed remote type never blocks the rest. The returned set
// includes THIS binary's own types too (identical to the floor → harmless,
// floor-wins on Merge); the point is it ALSO carries other sidecars' types.
func LoadCWMTypesFromCatalog(ctx stdctx.Context, cat mbapi.Catalog) core.CatalogTypeSet {
	d := api.DeclarationsFromCatalog(ctx, cat)
	set := core.CatalogTypeSet{
		Edges: d.Edges, Objects: d.Objects, Capabilities: d.Capabilities, Aspects: d.Aspects,
		Workflows: d.Workflows, Policies: d.Policies, HookTypes: d.HookTypes, Hooks: d.Hooks,
		EventTypes: d.EventTypes, Findings: d.Findings,
		Sources: d.Sources, Members: d.Members, Named: d.Named, Overridden: d.Overridden,
		// What the universe grants each sidecar beyond what it owns: where its
		// gate entries may also refuse writes (Univers.yaml sidecars[].grants).
		Grants: api.Grants(cenv.GateGrantsAt(application.Env().UniverseDir())),
	}
	set.CapabilitySchemas = capabilitySchemas(ctx, set.Capabilities, set.Objects, d.DataTypes)
	return set
}

// capabilitySchemas compiles the schema each capability declares from its
// data-type's DSL to JSON Schema — what CWM checks a stored group against. A
// schema whose data-type is absent from the catalog, or that does not compile,
// is logged and left out: CWM then reports the groups using it as unchecked.
func capabilitySchemas(ctx stdctx.Context, caps []api.Capability, types []api.ObjectType, dataTypes map[string][]byte) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	add := func(owner, uri string) {
		if uri == "" {
			return
		}
		if _, done := out[uri]; done {
			return
		}
		src, ok := dataTypes[uri]
		if !ok {
			slog.Warn("cwm: a schema is not in the catalog", "declared by", owner, "schema", uri)
			return
		}
		compiled, err := schemaparser.CompileJSONSchema(ctx, src)
		if err != nil || len(compiled) == 0 {
			slog.Warn("cwm: a schema does not compile", "declared by", owner, "schema", uri, "error", err)
			return
		}
		out[uri] = compiled
	}
	for _, c := range caps {
		add("capability "+c.Name, c.Schema)
		// The section a type writes under the capability's name, checked
		// when the type is declared.
		add("capability "+c.Name+" (type_schema)", c.TypeSchema)
	}
	// A type's own fields are checked the same way as a capability's group.
	for _, t := range types {
		add("object type "+t.Name, t.Fields)
	}
	return out
}

// ensureFirstRunWorkspace creates the workspace folder + a minimal
// `workspace.yaml` marker when missing. The substrate's
// `storage.New` requires `workspace.yaml` to exist; the bootstrap
// subservice (which creates the workspace + inbox nodes) only runs
// after storage init. This function closes that chicken-and-egg
// gap at the application layer so a fresh `.katyoti/` folder
// boots cleanly.
//
// Idempotent: a no-op when the marker is already present.
func ensureFirstRunWorkspace(root string) error {
	if root == "" {
		return fmt.Errorf("empty workspace root")
	}
	markerPath := storage.NewPaths(root).WorkspaceYAMLPath()
	if _, err := os.Stat(markerPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", markerPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(markerPath), err)
	}
	defaults := &storage.WorkspaceConfig{Version: "1"}
	if err := storage.WriteWorkspace(root, defaults); err != nil {
		return fmt.Errorf("write %s: %w", markerPath, err)
	}
	return nil
}

// projectUUIDAt reads the project id from `<project>/project.cwm.yaml`, "" when
// there is none — a project that has not run `project-init` yet is not a fault.
func projectUUIDAt(projectRoot string) string {
	if projectRoot == "" {
		return ""
	}
	f, err := projectfile.Read(projectRoot)
	if err != nil {
		return ""
	}
	return f.UUID
}

// resolveWorkspaceRoots returns the CWM store root and the corpus root for
// the config root the service was pointed at.
//
// THE CWM WORKSPACE LIVES IN THE PROJECT, NEVER IN THE UNIVERSE. A universe
// folder (`<GlobalHomeDir>/universes/<name>/.katyoti`) holds CONFIG and
// keys — identity, sidecars, `.env`. The content it serves is the repository
// ATTACHED to it (`projects.yaml`, written by attach-project), and that
// repository carries the whole CWM workspace: the committed corpus in
// `modules/` AND the workspace state — node store, index, lock,
// `config/workspace.yaml` — in its own gitignored `.katyoti/`. Keeping the
// state beside the universe config instead would split one workspace across
// two folders, and the coming per-module locks (modules as units of
// concurrency) must sit beside the modules they guard.
//
// So an attached project moves BOTH roots: store root = the project's config
// root, corpus root = the project. These are the roots of the FIRST member;
// a universe attaching several repositories opens one store per member, each
// on that member's own roots (openStores). No
// attached project → the roots given, with the corpus derived by stepping up
// from the config root — the universe-in-its-own-checkout layout (`dev`),
// unchanged. Run from the project folder itself, the config root already IS
// the project's, `projects.yaml` is not there, and the same fallback applies:
// both run modes land the workspace in the project.
func resolveWorkspaceRoots(configRoot, projectRoot string) (string, string) {
	if project := cenv.AttachedProject(configRoot); project != "" {
		return application.EnvOf(project).HomeDir(), project
	}
	if projectRoot == "" {
		projectRoot = deriveProjectRoot(configRoot)
	}
	return configRoot, projectRoot
}

// deriveProjectRoot returns the folder whose `modules/` holds the object
// corpus, given the workspace's CONFIG root.
//
// THE OBJECT CORPUS LIVES BESIDE THE CODE, not under the config root:
// `<project>/modules/`, committed with the software it describes (G2). Node
// storage, the graph and the type store stay under the config root because
// they are recreatable state. Conflating the two hid 474 authored objects from
// the running service while the tree rendered perfectly.
//
// DERIVED SAFELY. It steps up only when the parent's own config root IS the
// root it was given. A bare path that is not a config root — which is what
// every substrate test passes — returns "", so the caller falls back to the
// workspace root and nothing goes hunting for `modules/` in a system temp
// folder.
func deriveProjectRoot(workspaceRoot string) string {
	if workspaceRoot == "" {
		return ""
	}
	parent := filepath.Dir(workspaceRoot)
	if parent == workspaceRoot {
		return ""
	}
	if filepath.Clean(application.EnvOf(parent).HomeDir()) == filepath.Clean(workspaceRoot) {
		return parent
	}
	return ""
}

// attachedMembers are the repositories the universe whose config root is
// configRoot attaches, in attach order: the existing folders only (the one
// reader of projects.yaml, cenv.AttachedProjects).
func attachedMembers(configRoot string) []string {
	var out []string
	for _, p := range cenv.AttachedProjects(configRoot) {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// openStores opens the stores the service serves: one over cfg's roots when
// the universe attaches one member or none; one per member when it attaches
// several - each a copy of cfg on its member's roots (its own workspace
// state, lock and index under the member's config root), sharing the buses,
// the catalog types, the minter and the universe's ids. Each store is
// prepared (prepare: its observers, its project object) as soon as it opens,
// before any is served - a member's store gets the project object the store
// set addresses it by before storeset.New joins it. A store
// that does not open fails the service, naming its member.
func openStores(ctx stdctx.Context, cfg *core.CWMServiceConfig, members []string, prepare func(*core.Service)) ([]*core.Service, error) {
	if prepare == nil {
		prepare = func(*core.Service) {}
	}
	if len(members) < 2 {
		if err := ensureFirstRunWorkspace(cfg.WorkspaceRoot); err != nil {
			return nil, fmt.Errorf("first-run workspace init: %w", err)
		}
		svc, err := core.New(ctx, cfg)
		if err != nil {
			return nil, err
		}
		prepare(svc)
		return []*core.Service{svc}, nil
	}
	var stores []*core.Service
	fail := func(err error) ([]*core.Service, error) {
		for _, st := range stores {
			_ = st.Close()
		}
		return nil, err
	}
	for _, m := range members {
		c := *cfg
		c.WorkspaceRoot, c.ProjectRoot = application.EnvOf(m).HomeDir(), m
		if err := ensureFirstRunWorkspace(c.WorkspaceRoot); err != nil {
			return fail(fmt.Errorf("member %s: first-run workspace init: %w", m, err))
		}
		st, err := core.New(ctx, &c)
		if err != nil {
			return fail(fmt.Errorf("member %s: %w", m, err))
		}
		stores = append(stores, st)
		// A member from its opening: what its observers handle while it is
		// prepared (its project object minted) names its project already.
		st.ServeAsMember()
		prepare(st)
	}
	return stores, nil
}

// servedService is what the factory does with the service it built, beside
// its verbs: the lifecycle each store runs.
type servedService interface {
	api.CWMService
	AttachObservers(stdctx.Context, []api.EventObserver)
	AttachMatchTakers(stdctx.Context, []api.MatchTaker)
	RunBootstrapHooks(stdctx.Context, core.BootstrapPart) []error
	EnsureProject(stdctx.Context) (bool, error)
}

// serve is the one service over the stores: the store itself when there is
// one, the store set's front when there are several.
func serve(stores []*core.Service) (servedService, error) {
	if len(stores) == 1 {
		return stores[0], nil
	}
	set, err := storeset.New(stores...)
	if err != nil {
		return nil, err
	}
	return storeset.NewServed(set)
}

// describeOnlyService is the CWM service of a process that only describes
// itself (appcore.ManifestProbeProcess): the concrete type the KTSP container
// reflects its slot from, with no workspace behind it. It is never called — a
// probe serves nothing — and Close on it releases nothing.
func describeOnlyService() api.CWMService { return &core.Service{} }
