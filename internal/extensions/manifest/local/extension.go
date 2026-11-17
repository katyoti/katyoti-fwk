// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	"io/fs"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecar"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcommands "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/commands"
	manifestcore "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// Extension wires the manifest extension:
//   - registers `ContributionProvidersPoint` (the EP every contributor goes through)
//   - publishes `ResourceService` (so other code can `UseService[ResourceService](ctx)`)
//   - contributes the `<binary> contributions list/get` Cobra commands
//
// Every binary that imports this extension (directly or transitively) gets
// all three for free. Idempotent across multiple inclusions — RegisterPoint
// guards duplicates at the EM layer.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("manifest", ".points", func(app *application.Application) error {
			if err := application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]()); err != nil {
				return err
			}
			if err := application.RegisterPoint(app, em.NewListPoint[mbapi.ReadRoot]()); err != nil {
				return err
			}
			return application.RegisterPoint(app, em.NewListPoint[mbapi.RefreshHook]())
		})

		// Publish ResourceService. Identity is resolved at first-Use time
		// from the Cobra root (which appcore.NewAppExt populates from
		// cfg.AppName / cfg.Version).
		//
		// WithKTSPContainer exposes the service as KTSP slot "resources"
		// (ResourceServiceDef.Name) so the hub can reach a layer the
		// aggregate does not carry (`resources/open-layer`); discovery
		// itself is the `sc resources manifest` exec probe. CLI is
		// suppressed (cli=false) because users invoke the hand-written
		// `<binary> sc resources` Cobra group directly — not generated
		// `resources-list` etc.
		init.AddExtension(appcore.PublishSimpleService[mbapi.ResourceService](
			func(ctx context.Context) (mbapi.ResourceService, error) {
				// Use the ctx-based resolver — it respects the re-entrant
				// resolver injected during service creation, avoiding the
				// SM mutex re-entry that AppUseNamedService would cause
				// when AutoKTSPContainer resolves us at AppInit phase
				// (which holds sm.mu). See project_autoktsp_lazy_resolve.md.
				cr, err := application.UseNamedService[appcore.CobraCommand](ctx, "root")
				if err != nil {
					return nil, fmt.Errorf("manifest: cobra root unavailable: %w", err)
				}
				provider := cr.Cmd.Use
				version := cr.Cmd.Version
				if version == "" {
					version = "dev"
				}
				return manifestcore.NewResourceService(provider, version), nil
			},
			appcore.WithServiceDef(mbapi.ResourceServiceDef),
			appcore.WithKTSPContainer(),
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))

		// Publish a self-only `mbapi.Catalog` containing the binary's own
		// manifest. Lets any consumer (`skills list` CLI, system prompt
		// builder, …) resolve a catalog uniformly via `UseService[Catalog]`
		// whether or not the binary can see beyond itself.
		//
		// A binary that also wants the universe wires
		// manifest/universe.UniverseCatalogExtension, which registers a concrete instance at
		// AppInit and so deterministically replaces this one. Without it, a
		// binary answers for itself — which is what a standalone sidecar
		// needs and what `ReachSelf` names.
		init.AddExtension(appcore.PublishSimpleService[mbapi.Catalog](
			func(ctx context.Context) (mbapi.Catalog, error) {
				cr, err := application.UseNamedService[appcore.CobraCommand](ctx, "root")
				if err != nil {
					return nil, fmt.Errorf("manifest: cobra root unavailable: %w", err)
				}
				provider := cr.Cmd.Use
				version := cr.Cmd.Version
				if version == "" {
					version = "dev"
				}
				m, err := manifestcore.NewBuilder(provider, version).Build(ctx)
				if err != nil {
					return nil, fmt.Errorf("manifest: build self manifest: %w", err)
				}
				return manifestcore.NewMemoryCatalog([]*mbapi.Manifest{m}, []string{provider}), nil
			},
		))

		// Contribute the `contributions` command group automatically. Binary
		// identity is resolved from cmd.Root() at run-time — the command
		// works in any binary that imports this extension. Lives under
		// `sc resources` per the command-surface decision (the
		// canonical layout has only domain / sc / llm at root). On
		// binaries WITHOUT `SidecarGroupsExt()` (ktw-hub, ktw-test),
		// SCCommandExt errors gracefully and the command is absent —
		// matches today's behavior for those binaries.
		init.AddExtension(appcore.SCCommandExt(manifestcommands.NewResourcesCommand()))

		// Contribute the canonical ReferenceBody + ConfigBody data-type
		// Contributions shipped by this extension. The hub's catalog
		// Phase-2 wire-up registers them with SchemaService at boot.
		// Empty provider → resolved to the binary's app name at
		// Contribute time (per the URI grammar the
		// `<provider>` token in a URI is always a BINARY name, never
		// an extension/library namespace).
		manifestcontrib.ContributeDataTypeResources(
			"",
			loader.NewEmbedFSLoader(dataTypesFS, "."),
			"embed/data-types",
		)(init)
	}
}

// ContributeResources returns an Extension that adds one ContributionProvider
// to the point. Use it from a binary's bootstrap.
//
// Examples:
//
//	// A folder of skills (flat IDs):
//	ContributeResources(&local.manifestcontrib.EmbedFSResourceProvider{
//	    ProviderName:        "ktw-code",
//	    ResourceType:        "skill",
//	    Loader:              loader.NewEmbedFSLoader(efs, "."),
//	    Root:                "embed/skills",
//	    KeepFolderStructure: false,
//	    FileExtensions:      []string{"md"},
//	})
//
//	// A folder of references (tree IDs):
//	ContributeResources(&local.manifestcontrib.EmbedFSResourceProvider{
//	    ProviderName:        "ktw-docs",
//	    ResourceType:        "reference",
//	    Loader:              loader.NewEmbedFSLoader(efs, "."),
//	    Root:                "embed/references",
//	    KeepFolderStructure: true,
//	})
func ContributeResources(p contribapi.ContributionProvider) application.Extension {
	return func(init *application.ApplicationInit) {
		key := ".resource." + p.Provider() + "." + p.Type()
		init.AddContribute("manifest", key, func(app *application.Application) error {
			return application.Contribute(app, p)
		})
	}
}

// ContributeRefreshHook returns an Extension that registers one RefreshHook on
// ContributionRefreshHooksPoint. The contribution-change subscriber fans out to
// it (kind-filtered) after refreshing the catalog index. Use it from a binary's
// bootstrap so a consumer (SkillManager, CWM type store, …) reacts to hot
// contribution changes without restart.
func ContributeRefreshHook(hook mbapi.RefreshHook) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".refresh-hook."+hook.Name, func(app *application.Application) error {
			return application.Contribute(app, hook)
		})
	}
}

// Cache is the public alias for the per-sidecar manifest cache type.
// Callers in `app/ktw-hub/` reference this when they need to type a
// helper function's parameter (e.g. `func hydrateCache(c *manifestlocal.Cache, …)`)
// without importing the restricted `internal/extensions/manifest/internal/core`
// package.
type Cache = manifestcore.Cache

// NewCache exposes the per-sidecar manifest cache for hub binaries.
// The internal/core package is restricted; this is the public
// constructor. Replaces the prior flat-dir `NewInventory` (2026-06-05
// redesign — see `cache.go` for the new layout).
func NewCache(root string) *manifestcore.Cache {
	return manifestcore.NewCache(root)
}

// NewUniverseCache is the cache as one universe's hub reads it: the read roots
// a record declares resolve against this universe's folder and the global
// home, and a record that depends on the universe (a universe read root, an
// override tree) is kept apart from every other universe's. See
// manifestcore.Cache.ForUniverse.
func NewUniverseCache(root, universeDir, globalDir string) *manifestcore.Cache {
	return manifestcore.NewCache(root).ForUniverse(universeDir, globalDir)
}

// DeclareReadRoot declares a folder this binary reads contributions from at
// run time, relative to an anchor (mbapi.ReadRootUniverse or ReadRootGlobal).
// The builder records every declaration in Manifest.ReadRoots, so the
// aggregator fingerprints the folder itself and re-probes the binary when it
// changes. Declare the folder whether it exists or not: one created later is
// then noticed too.
func DeclareReadRoot(anchor, path string) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".read-root."+anchor+"."+path, func(app *application.Application) error {
			return application.Contribute(app, mbapi.ReadRoot{Anchor: anchor, Path: path})
		})
	}
}

// DigestDir fingerprints a directory tree. Hub binaries pass the result to
// Cache.Save / Cache.IsStale to detect edits the exe stat cannot see — an
// override tree, or a folder a generic host serves. Returns "" for a
// missing/empty dir (the prod-safe value).
func DigestDir(dir string) string {
	return manifestcore.DigestDir(dir)
}

// SourceDigests is the staleness key for sources a binary READS rather than
// carries — an override tree, or a folder a generic host serves. See
// manifestcore.SourceDigests.
type SourceDigests = manifestcore.SourceDigests

// ContentWatchInterval is how often a served folder is re-fingerprinted.
const ContentWatchInterval = manifestcore.ContentWatchInterval

// LoadManifestsFromCache was deliberately removed (2026-06-05 layering
// rule): the on-disk cache is owned by ktw-hub exclusively. Every
// other consumer — ktw-agents, ktw-gui, future bus clients — fetches
// manifests via `ktw-hub:/CatalogService/GetManifest({provider})` over
// the bus. Direct disk reads coupled this binary to the cache's
// file-format and offered no protection against the hub's mid-write
// races; the bus-only surface eliminates both. `FetchUniverseIndex` in
// manifest/universe (universe.go) is that surface, usable by any binary that
// imports it on purpose.

// NewBuilder exposes the manifest builder for binaries that need to
// produce their OWN descriptor in-process (e.g. manifest/universe adding this
// binary's contributions to the aggregated view alongside cached sidecar
// manifests, without spawning itself via `RunEndpointExtensions`).
//
// `provider` is the binary name (e.g., "ktw-agents"); `version` is
// whatever your `appcore.AppConfig.Version` resolves to. The
// `Builder.Build(ctx)` call walks every contributed ContributionProvider
// via the EM — same data source the disk-backed catalog's refresh
// path uses — so the resulting manifest matches what `<binary>
// resources list --type X` returns.
func NewBuilder(provider, version string) *manifestcore.Builder {
	return manifestcore.NewBuilder(provider, version)
}

// WatchCache watches the per-sidecar cache root for filesystem changes
// (folder create / rewrite via atomic rename) and invokes `onChange`
// (debounced) on every event. Blocks until ctx is cancelled. Returns
// the first non-cancel error encountered.
//
// Hub binaries call this in a goroutine to keep the in-memory Catalog
// synchronised with the on-disk cache — e.g. when `catalog refresh`
// rewrites a sidecar's folder or a developer manually edits the cache.
// Internally delegates to the underlying directory watcher; the same
// semantics as the prior `WatchInventory` but pointed at the new
// per-sidecar layout root.
func WatchCache(ctx context.Context, root string, onChange func()) error {
	return manifestcore.WatchInventory(ctx, root, onChange)
}

// WatchUniverseSidecars watches the universe's `Univers.yaml`
// file for content changes and invokes `onChange` (debounced) on each
// rewrite. Handles atomic-save patterns (write-temp-then-rename) by
// watching the parent directory and filtering to the target path.
//
// Hub binaries call this in a goroutine to keep the membership of
// the Catalog and ConnectionManager synchronised with the on-disk
// `sidecars:` list. The callback typically reloads the registry, then
// calls `Catalog.SetMembers` + `connmgr.SetMembers` with the new names.
func WatchUniverseSidecars(ctx context.Context, path string, onChange func()) error {
	return manifestcore.WatchUniverseSidecars(ctx, path, onChange)
}

// Member is one entry of a catalog's membership — a name and the binary the
// universe declares for it, which together address its tier-1 record. See
// manifestcore.Member.
type Member = manifestcore.Member

// Record is one tier-1 record as Cache.Records lists it: the member name and
// the loaded pair, or the error that refused it. See manifestcore.Record.
type Record = manifestcore.Record

// NewCatalog exposes the in-memory Catalog implementation for hub
// binaries. Members are typically the sidecars of Univers.yaml, each with
// its declared binary.
func NewCatalog(cache *manifestcore.Cache, members []Member) (mbapi.Catalog, error) {
	return manifestcore.NewCatalog(cache, members)
}

// NewMemoryCatalog builds an in-memory `Catalog` from a slice of
// pre-built manifests, without requiring an `Inventory` (disk cache).
// Suited for binaries like ktw-agents that gather descriptors
// in-process via `RunEndpointExtensions` and have no need for
// cross-process cache sharing. `extraMembers` lists provider names
// that should appear in `Names()` even when their manifest hasn't
// been loaded yet (matches the Univers.yaml-membership semantics).
//
// The returned `*MemoryCatalog` exposes a `Replace()` method for
// atomic content swaps (useful when discovery refreshes mid-process).
func NewMemoryCatalog(manifests []*mbapi.Manifest, extraMembers []string) *manifestcore.MemoryCatalog {
	return manifestcore.NewMemoryCatalog(manifests, extraMembers)
}

// MemoryCatalog re-exports the in-memory aggregate catalog so consumers
// outside the restricted internal/core package can name the concrete type —
// needed to call its non-interface methods (Replace / Put / SetContentLane)
// for event-driven incremental refresh. Construct via NewMemoryCatalog.
type MemoryCatalog = manifestcore.MemoryCatalog

// ContentStore re-exports the content-addressed body store (the CONTENT
// plane of the two-plane cache). Construct via NewContentStore.
type ContentStore = manifestcore.ContentStore

// BodyFetcher re-exports the per-hash bus fetch callback consumed by
// MemoryCatalog.SetContentLane.
type BodyFetcher = manifestcore.BodyFetcher

// NewContentStore constructs a content-addressed body store rooted at
// `root` (typically `<CacheDir>/contributions`). One file per sha256;
// identical bodies dedup. Safe to `rm -rf` — misses are repaired by the
// per-hash bus fetch wired into MemoryCatalog.SetContentLane.
func NewContentStore(root string) *manifestcore.ContentStore {
	return manifestcore.NewContentStore(root)
}

// ContributeConfigResources returns an Extension that contributes the three
// config-related ResourceProviders (config-file / config-program / env) on
// behalf of the binary. The provider name is resolved at Contribute-time
// from the Cobra root, so the helper works in any binary wired via
// `appcore.NewAppExt(...)`.
//
// Pass the same `embed.FS` that's handed to `appcore.SidecarExt(efs)` —
// the file-default detection (`ConfigFileBody.HasDefault`) reads
// `embed/config/<name>` from it.
//
// Wire it once per binary, immediately after `appcore.SidecarExt(efs)`:
//
//	appcore.SidecarExt(efs),
//	manifestlocal.Extension(),
//	manifestlocal.ContributeConfigResources(efs),
func ContributeConfigResources(efs fs.FS) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddContribute("manifest", ".config-resources", func(app *application.Application) error {
			cr, err := application.AppUseNamedService[appcore.CobraCommand](app, "root")
			if err != nil {
				return nil // no root command (test fixture) — skip silently
			}
			for _, p := range sidecar.ConfigResourceProviders(cr.Cmd.Use, efs) {
				if err := application.Contribute(app, p); err != nil {
					return fmt.Errorf("manifest: contribute %s/%s: %w", p.Provider(), p.Type(), err)
				}
			}
			return nil
		})
	}
}

// === Validation, for the moments outside the aggregator ===
//
// The check is ONE function. A binary's own test suite, the packaging tool and
// the aggregator all call it; only the caller and the moment differ. A second
// implementation would be a second place for "valid" to mean something
// slightly different, which is the failure this shape exists to prevent.

// BodyValidator checks a body against a data-type URI. Nil means no validator
// is available — which ValidateContribution records as a finding rather than
// treating as a pass, so a binary that ships no parser says so instead of
// silently reporting everything as fine.
type BodyValidator = manifestcore.BodyValidator

// ValidationOutcome is one contribution's verdict.
type ValidationOutcome = manifestcore.ValidationOutcome

// ValidateContribution runs the framework check for one contribution, against
// the schema its KIND declares and at the severity its KIND declares — both
// resolved by the caller from the kind registry (contribapi.Kinds), because a
// kind's declaration is the only place either is stated.
func ValidateContribution(
	ctx context.Context,
	severity string,
	c contribapi.Contribution,
	schema string,
	validate BodyValidator,
) ValidationOutcome {
	return manifestcore.ValidateContribution(ctx, severity, c, schema, validate)
}

// === The object store, for the producer side ===
//
// Reading a bulk layer is the catalog's business and stays behind it. WRITING
// one is packaging's, and packaging lives outside this extension — so the store
// is republished here rather than reimplemented there. Two implementations of
// "content addressed by hash" would be two places for an address to mean
// something slightly different, and the address is the only thing holding the
// bulk plane together.

// BlobStore holds content addressed by hash. Every method takes a hash, never
// a URI: a URI can be re-pointed, its content's address cannot.
type BlobStore = manifestcore.BlobStore

// NewFileBlobStore roots a store at a directory — the filesystem backend, and
// the one a local pack build writes into.
func NewFileBlobStore(root string) *manifestcore.FileBlobStore {
	return manifestcore.NewFileBlobStore(root)
}

// ReclaimBlobs evicts content the index no longer references, and returns how
// many addresses stopped resolving.
//
// Mark from the aggregated view, sweep by hash. By hash because several
// contributions may share one blob, so a contribution disappearing says nothing
// on its own about whether its content is still in use; from the WHOLE view
// because that is the only place the question has an answer.
//
// An empty index does nothing: "not built yet" and "nothing is referenced" look
// identical and have opposite right answers.
func ReclaimBlobs(ctx context.Context, entries []contribapi.Contribution, store BlobStore) (int, error) {
	return manifestcore.Reclaim(ctx, entries, store)
}
