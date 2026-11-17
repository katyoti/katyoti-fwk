// SPDX-License-Identifier: Apache-2.0

// Package universe is the CONSUMER side of the contribution catalog: the
// universe view a binary obtains from the aggregator (ktw-hub) — the paged
// body-free index, the content, layer and bulk lanes, and the change
// subscription that keeps the view current.
//
// It is its own package so that linking it is a decision. A pure sidecar
// publishes (manifest/local + the producers) and answers ReachSelf; only a
// binary that consumes the universe on purpose (ktw-agents, ktw-compose)
// imports this package, and with it the hub client lane in manifest/remote.
// The split and the guard that holds it: modules/contributions/packages.md
// and internal/test/arch/sidecar_footprint_test.go (e27).
package universe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcore "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
)

// universe.go — the aggregation point, reachable by any binary.
//
// A universe view is assembled once, by the aggregator, and served over a
// protocol. This is what a CONSUMER does to obtain it: page the body-free
// index, add its own contributions, wire a content lane, and keep the whole
// thing current. None of that is specific to one binary, so it no longer lives
// in one binary's bootstrap.
//
// Discovery contacts no contributor. Everything here talks to the aggregator
// and to a local content-addressed store; no sidecar is spawned, woken or
// called to find out what exists.
//
// Cost: this package and manifest/remote (the catalog client). Every binary
// that serves KTSP already carries the hub client, the bus sender and the
// environment resolver; a binary that does not import this package links
// neither (e27 moved it out of manifest/local, which every sidecar links).

// UniverseCatalogExtension publishes the aggregated universe view as the
// binary's `mbapi.Catalog`, replacing the self-only view Extension() publishes.
//
// Wire it in any binary that needs `ReachUniverse`. Without it a binary still
// answers `ReachSelf` with nothing else running — which is why the two reaches
// are separate.
//
// # Connecting is the binary's policy, not this extension's
//
// The view is fetched through whatever `*hubclient.HubClient` the application
// already provides. A binary that registers one decides for itself whether
// resolving it may START a universe — ktw-agents does, because bringing the
// universe up is what its run commands are for. A binary that registers none
// falls back to reusing an already-running aggregator and never spawns
// anything.
//
// # Nothing is fetched until something asks
//
// The catalog is published as a lazy service, so a binary that never queries
// the universe never opens a connection. `--help` costs nothing. A binary that
// wants the view built during startup resolves the service in its own init
// hook, which is an explicit choice rather than a cost every binary pays.
//
// Degrades rather than fails: with no reachable aggregator the view is this
// binary's own contributions. An unreachable universe is an operational
// condition, not a reason for a binary not to start.
func UniverseCatalogExtension(opts ...UniverseOption) application.Extension {
	var cfg universeOptions
	for _, o := range opts {
		o(&cfg)
	}
	return func(init *application.ApplicationInit) {
		lazy := newLazyCatalog(buildUniverseCatalog)
		// AppInit, registered as a concrete instance: that is what makes this
		// deterministically replace the self-only view Extension() publishes,
		// rather than depending on which factory registered last. Phases run
		// in sequence, so every Bootstrap consumer sees the universe view —
		// and because it is lazy, registering it costs nothing.
		init.AddAppInit("universe-catalog", ".catalog", func(app *application.Application) error {
			application.PutService[mbapi.Catalog](app, "default", lazy)
			return nil
		})
		if cfg.watchChanges {
			init.AddBootstrap("universe-catalog", ".refresh", func(app *application.Application) error {
				return subscribeToChanges(app, lazy)
			})
		}
	}
}

// UniverseOption configures UniverseCatalogExtension.
type UniverseOption func(*universeOptions)

type universeOptions struct{ watchChanges bool }

// WithChangeSubscription keeps the local view current by subscribing to the
// aggregator's change announcements.
//
// Opt-in, because subscribing means holding a connection for the process
// lifetime — right for a long-running consumer, wrong for a CLI that runs one
// command and exits.
//
// The announcement is advisory: it says "re-check this provider", never "here
// is the new content". Correctness therefore never depends on delivery — a
// lost signal costs latency, a duplicate costs one comparison, and no trust is
// placed in the sender's payload.
func WithChangeSubscription() UniverseOption {
	return func(o *universeOptions) { o.watchChanges = true }
}

// buildUniverseCatalog assembles the view: the aggregator's body-free index for
// every other provider, this binary's own contributions built in process, and a
// content lane for reading bodies later.
func buildUniverseCatalog(ctx context.Context) (mbapi.Catalog, error) {
	selfName, version := selfIdentity(ctx)

	var members []string
	if reg, err := appcore.LoadUniverseSidecars(); err == nil {
		members = make([]string, 0, len(reg.Sidecars))
		for _, s := range reg.Sidecars {
			if s.Binary != "" {
				members = append(members, s.Name)
			}
		}
	} else {
		slog.Warn("universe catalog: load Univers.yaml failed", "error", err)
	}

	hub := connectAggregator(ctx)
	manifests, err := FetchUniverseIndex(ctx, hub)
	if err != nil {
		// No fallback to reading the aggregator's cache directly: its surface
		// is the only way to the universe view, and a local copy read behind
		// its back is how the two diverge silently.
		slog.Error("universe catalog: aggregator unreachable; view is this binary's own contributions only",
			"error", err, "hint", "ensure ktw-hub is running for this universe")
		manifests = nil
	}
	// Drop the aggregator's copy of THIS binary: the in-process build below is
	// always current, while a cached copy may predate the running binary.
	kept := manifests[:0]
	for _, m := range manifests {
		if m != nil && m.Provider != selfName {
			kept = append(kept, m)
		}
	}
	manifests = kept

	own := application.UseExtensions[contribapi.ContributionProvider](ctx)
	self, berr := manifestcore.NewBuilder(selfName, version).Build(ctx)
	if berr != nil {
		slog.Warn("universe catalog: own manifest build failed", "error", berr)
	} else if self != nil {
		// The in-process build is the binary's SELF view, local entries
		// included; this catalog is the universe view, where a `local:`
		// entry of any member — this one too — does not exist. Narrowed
		// once here, the same way the aggregator narrows a loaded record.
		self.Contributions = mbapi.Filter{}.Apply(self.Contributions, mbapi.ReachUniverse)
		manifestcore.ReHash(self)
		manifests = append(manifests, self)
	}

	cat := manifestcore.NewMemoryCatalog(manifests, members)
	// The content lane: a content-addressed store (no aggregator call) plus
	// one bounded per-hash fetch on a miss. The index is body-free, so listing
	// the universe never moves content.
	cat.SetContentLane(
		manifestcore.NewContentStore(application.Env().GetCacheSubDir("contributions")),
		OwnFirstBodies(selfName, own, HubBodyFetcher(hub)))
	// The layer lane: a named layer the index does not carry (a skill's
	// instructions, a data-type's source) through the aggregator's OpenLayer,
	// which forwards to the producer when it does not hold it either.
	cat.SetLayerLane(OwnFirstLayers(selfName, own, HubLayerFetcher(hub)))
	// The bulk lane: content past what any message payload can carry, fetched
	// out of band by hash. A filesystem store minting `file://` locators is
	// what local development runs against; an object store minting `https://`
	// ones replaces it without changing a line on the consumer side, because
	// the locator is resolved by scheme rather than assumed to be a path.
	cat.SetBulkLane(bulkLane(ctx))
	slog.Info("universe catalog: built", "manifests", len(manifests), "members", len(members))
	return cat, nil
}

// connectAggregator returns a connection to the aggregator, or nil.
//
// An application-provided client wins: registering one is how a binary states
// its own policy, including whether resolving it may start a universe. With
// none registered, reuse a running aggregator and never spawn — a consumer
// asking what exists must not bring infrastructure up as a side effect.
func connectAggregator(ctx context.Context) *hubclient.HubClient {
	if hc, err := application.UseService[*hubclient.HubClient](ctx); err == nil && hc != nil {
		return hc
	}
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{
		DataDir: application.Env().DataDir(),
		Mode:    hubclient.ModeReuse,
	})
	if err != nil {
		return nil
	}
	return hc
}

// subscribeToChanges wires the change announcement to an incremental refresh of
// the local view, then fans out to every contributed RefreshHook.
// Subscribing is itself what forces the view to be built: a binary that asked
// to stay current has, by that request, said it intends to use the view.
func subscribeToChanges(app *application.Application, lazy *lazyCatalog) error {
	appCtx := application.WithApplication(context.Background(), app)
	mem := lazy.Concrete(appCtx)
	hub := connectAggregator(appCtx)
	if hub == nil {
		return nil
	}

	var self *mbapi.Manifest
	selfName, version := selfIdentity(appCtx)
	if m, berr := manifestcore.NewBuilder(selfName, version).Build(appCtx); berr == nil {
		self = m
	}
	var members []string
	if reg, rerr := appcore.LoadUniverseSidecars(); rerr == nil {
		for _, s := range reg.Sidecars {
			if s.Binary != "" {
				members = append(members, s.Name)
			}
		}
	}

	tenantID, universeID := busScope()
	hooks := application.UseExtensions[mbapi.RefreshHook](appCtx)
	fetchAll := func(c context.Context) ([]*mbapi.Manifest, error) {
		return FetchUniverseIndex(c, hub)
	}
	sub, subErr := manifestlocal.SubscribeContributionsChanged(
		context.Background(), hub.MessageBus(), tenantID, universeID, 100*time.Millisecond,
		func(ev contribapi.ContributionsChangedEvent) {
			RefreshOnContributionsChanged(appCtx, fetchAll, mem, members, self, hooks, ev)
		})
	if subErr != nil {
		slog.Warn("universe catalog: contributions-changed subscribe failed", "error", subErr)
		return nil
	}
	slog.Info("universe catalog: subscribed to contributions-changed",
		"tenant", tenantID, "universe", universeID)
	// Events published while the link was down never arrive. On every
	// reconnect the view is checked against the aggregator's digests and
	// rebuilt when one moved — a lost event costs one check, never a stale
	// view for the life of the process (G15, the consumer half).
	digests := func(c context.Context) (map[string]string, error) {
		sender, err := hubSender(hub)
		if err != nil {
			return nil, err
		}
		return manifestremote.New(sender).Digests(c)
	}
	hub.OnReconnect(func() {
		if _, err := ResyncView(appCtx, digests, fetchAll, mem, members, self, hooks); err != nil {
			slog.Warn("universe catalog: resync after reconnect failed; keeping live view", "error", err)
		}
	})
	// Rooted for the process lifetime; the connection is owned by the
	// ApplicationScope container and closed at exit, which drains the
	// subscription server-side.
	_ = sub
	return nil
}

// selfIdentity reads this binary's provider name and version off the cobra
// root — the same source the self-only catalog uses, so the two agree.
func selfIdentity(ctx context.Context) (name, version string) {
	cr, err := application.UseNamedService[appcore.CobraCommand](ctx, "root")
	if err != nil {
		return "", "dev"
	}
	version = cr.Cmd.Version
	if version == "" {
		version = "dev"
	}
	return cr.Cmd.Use, version
}

// busScope resolves the tenant and universe tokens every bus subject is scoped
// by. Publisher and subscriber MUST resolve these the same way: divergent
// tokens produce subjects that never match, and the failure is silent.
func busScope() (tenantID, universeID string) {
	tenantID = os.Getenv("KTW_TENANT_ID")
	if tenantID == "" {
		tenantID = "local"
	}
	universeID = "default"
	if m, err := cenv.LoadUniverseManifest(application.Env().UniverseDir()); err == nil && m != nil && m.Name != "" {
		universeID = m.Name
	}
	return tenantID, universeID
}

// hubSender builds a tenant/universe-scoped sender for the aggregator's
// catalog surface.
func hubSender(hc *hubclient.HubClient) (*messagebus.Sender, error) {
	if hc == nil {
		return nil, fmt.Errorf("universe catalog: no connection (universe unreachable)")
	}
	tenantID, universeID := busScope()
	return messagebus.NewSender(hc.MessageBus(), tenantID, universeID), nil
}

// asksSlots reports whether the view asks a provider's slots: a member's,
// or any provider's when the membership is unknown (nil). A layer provider
// (the built-in system data types, the universe's own contributions) has no
// manifest at the aggregator.
func asksSlots(members map[string]bool, provider string) bool {
	return members == nil || members[provider]
}

// FetchUniverseIndex pages the aggregator's body-free index and regroups it
// into per-provider manifests.
//
// Body-free is what makes this affordable: each page carries identity, meta and
// content addresses but no content, so learning what the universe offers costs
// in proportion to the number of contributions rather than their size. Bodies
// are resolved later, one hash at a time, and only when a consumer reads one.
func FetchUniverseIndex(ctx context.Context, hc *hubclient.HubClient) ([]*mbapi.Manifest, error) {
	sender, err := hubSender(hc)
	if err != nil {
		return nil, err
	}

	// The cursor loop lives in manifest/remote, once. This used to be a second
	// copy of it — and a paging bug is invisible until a universe crosses a
	// page boundary, which is the worst moment to find one.
	index, err := manifestremote.New(sender).Index(ctx)
	if err != nil {
		return nil, err
	}

	// Group by the contribution's OWN provider, not by the manifest that
	// carried it: a layer provider's contributions live inside another
	// binary's manifest, and attributing them to the carrier gives the same
	// logical entry two URIs, which breaks dedupe.
	byProvider := make(map[string][]contribapi.Contribution)
	for _, e := range index {
		if e.Provider != "" {
			byProvider[e.Provider] = append(byProvider[e.Provider], e)
		}
	}
	// The members, asked once: unknown (nil) when the aggregator cannot say,
	// and then every provider is asked as before.
	var members map[string]bool
	if names, err := manifestremote.New(sender).Providers(ctx); err == nil {
		members = make(map[string]bool, len(names))
		for _, n := range names {
			members[n] = true
		}
	}
	out := make([]*mbapi.Manifest, 0, len(byProvider))
	for prov, entries := range byProvider {
		m := &mbapi.Manifest{Provider: prov, Contributions: entries}
		// Only a member carries slots: a layer provider (the built-in system
		// data types, the universe's own contributions) has no manifest at the
		// aggregator - asking it warned on every view built.
		if !asksSlots(members, prov) {
			out = append(out, m)
			continue
		}
		// The index is contributions only. The method registry lives on the
		// manifest, and without it a consumer has every skill but no callable
		// tool — so fetch each provider's manifest once for its slots.
		gm, cerr := manifestremote.New(sender).ManifestSlots(ctx, prov)
		var rpcErr *ktsp.RPCError
		if cerr != nil && !(errors.As(cerr, &rpcErr) && rpcErr.Code == ktsp.CodeSlotNotFound) {
			// A member whose methods cannot be read is a member nothing can
			// call through this view: say so, never drop it silently.
			slog.Warn("universe catalog: a provider's slots are unreadable; its methods are missing from this view",
				"provider", prov, "error", cerr)
		}
		if cerr == nil && gm != nil {
			m.SlotTypes = gm.SlotTypes
			m.Slots = gm.Slots
			m.Transports = gm.Transports
			// The producer identity rows (digest, mutable, declared): what an
			// inspector of this view reports about where an entry came from.
			m.Providers = gm.Providers
			if m.Version == "" {
				m.Version = gm.Version
			}
			// The aggregator's hash for this provider is what the view was
			// built from; ResyncView compares it on reconnect.
			m.Hash = gm.Hash
		}
		out = append(out, m)
	}
	return out, nil
}

// ResyncView compares the aggregator's per-provider manifest hashes with the
// ones the view was built from and, when any differs or is missing, rebuilds
// the whole view through the same path a whole-catalog event takes (hooks
// included). It reports whether it rebuilt. Concepts §10: notification is
// latency, digest comparison is the truth — the hub already polls its sources
// against the record; this is the consumer's side of the same rule.
//
// The local build (selfManifest) is authoritative and never compared. A
// member whose manifest the view holds without a hash (a layer-only carrier)
// reads as stale, which costs one refetch rather than one missed change.
func ResyncView(
	appCtx context.Context,
	digests func(context.Context) (map[string]string, error),
	fetchAll func(context.Context) ([]*mbapi.Manifest, error),
	cat *manifestcore.MemoryCatalog,
	allNames []string,
	selfManifest *mbapi.Manifest,
	hooks []mbapi.RefreshHook,
) (bool, error) {
	if cat == nil || digests == nil {
		return false, nil
	}
	have, err := digests(appCtx)
	if err != nil {
		return false, err
	}
	if !viewIsStale(appCtx, cat, have, selfManifest) {
		return false, nil
	}
	RefreshOnContributionsChanged(appCtx, fetchAll, cat, allNames, selfManifest, hooks,
		contribapi.ContributionsChangedEvent{KindsChanged: []string{"*"}})
	return true, nil
}

// viewIsStale is the decision ResyncView makes: any provider the aggregator
// reports with a hash the view does not hold under that provider.
func viewIsStale(ctx context.Context, cat *manifestcore.MemoryCatalog, have map[string]string, selfManifest *mbapi.Manifest) bool {
	for prov, hash := range have {
		if hash == "" || (selfManifest != nil && prov == selfManifest.Provider) {
			continue
		}
		m, err := cat.Get(ctx, prov)
		if err != nil || m == nil || m.Hash != hash {
			return true
		}
	}
	return false
}

// HubBodyFetcher resolves ONE contribution body through the aggregator, by
// hash. Wired into a consumer catalog's content lane and reached only on a
// content-store miss.
func HubBodyFetcher(hc *hubclient.HubClient) manifestcore.BodyFetcher {
	return func(ctx context.Context, hash, uri string) ([]byte, error) {
		sender, err := hubSender(hc)
		if err != nil {
			return nil, err
		}
		raw, sendErr := sender.Call(ctx, manifestremote.GetBodyPath,
			mbapi.GetBodyInput{Hash: hash, URI: uri})
		if sendErr != nil {
			return nil, fmt.Errorf("universe GetBody %s: %w", uri, sendErr)
		}
		var resp mbapi.GetBodyOutput
		if err := json.Unmarshal(raw, &resp); err != nil {
			return nil, fmt.Errorf("decode universe GetBody %s: %w", uri, err)
		}
		return resp.Content, nil
	}
}

// HubLayerFetcher resolves ONE named layer through the aggregator's
// OpenLayer (CatalogService/OpenLayer on the wire). Wired into a consumer
// catalog's layer lane and reached only when neither the index nor the
// content store holds the bytes.
func HubLayerFetcher(hc *hubclient.HubClient) manifestcore.LayerFetcher {
	return func(ctx context.Context, uri, layer string) ([]byte, contribapi.LayerRef, error) {
		sender, err := hubSender(hc)
		if err != nil {
			return nil, contribapi.LayerRef{}, err
		}
		rc, ref, err := manifestremote.New(sender).OpenLayer(ctx, uri, layer)
		if err != nil {
			return nil, contribapi.LayerRef{}, err
		}
		defer rc.Close()
		body, err := io.ReadAll(rc)
		if err != nil {
			return nil, ref, fmt.Errorf("read %s layer %q: %w", uri, layer, err)
		}
		return body, ref, nil
	}
}

// OwnFirstLayers answers a layer of a contribution THIS binary publishes from
// its own providers, in process, and every other provider's through next (the
// aggregator). Its own contributions are built in process into the view; their
// content is read the same way, so a binary reads its own skill with no
// universe running.
func OwnFirstLayers(self string, own []contribapi.ContributionProvider, next manifestcore.LayerFetcher) manifestcore.LayerFetcher {
	return func(ctx context.Context, uri, layer string) ([]byte, contribapi.LayerRef, error) {
		if body, ref, mine, err := ownLayer(ctx, self, own, uri, layer); mine {
			return body, ref, err
		}
		return next(ctx, uri, layer)
	}
}

// OwnFirstBodies is OwnFirstLayers for the body lane (the data layer).
func OwnFirstBodies(self string, own []contribapi.ContributionProvider, next manifestcore.BodyFetcher) manifestcore.BodyFetcher {
	return func(ctx context.Context, hash, uri string) ([]byte, error) {
		if body, _, mine, err := ownLayer(ctx, self, own, uri, contribapi.LayerData); mine {
			return body, err
		}
		return next(ctx, hash, uri)
	}
}

// ownLayer opens one layer of uri through the providers of self; mine is false
// when uri is another provider's.
func ownLayer(ctx context.Context, self string, own []contribapi.ContributionProvider, uri, layer string) (body []byte, ref contribapi.LayerRef, mine bool, err error) {
	u, perr := contribapi.ParseResourceURI(uri)
	if perr != nil || u.Provider != self {
		return nil, ref, false, nil
	}
	for _, p := range own {
		if p.Provider() != self || p.Type() != u.Type {
			continue
		}
		rc, r, oerr := p.Open(ctx, u.ID, layer)
		if errors.Is(oerr, contribapi.ErrContributionNotFound{}) {
			continue // this provider does not enumerate the id — try the next
		}
		if oerr != nil {
			return nil, r, true, oerr
		}
		defer rc.Close()
		b, rerr := io.ReadAll(rc)
		return b, r, true, rerr
	}
	return nil, ref, true, mbapi.ErrContributionNotFound{URI: uri}
}

// RefreshOnContributionsChanged applies one change announcement: re-resolve
// what changed in the local view, then fan out to every contributed RefreshHook
// whose kinds match.
//
//   - a named provider patches only that provider's entry, leaving the rest of
//     the index untouched;
//   - an empty provider means universe-wide, and re-pulls the whole body-free
//     index. That stays affordable precisely because it carries no content.
//
// This binary's OWN manifest is built in process and never served by the
// aggregator, so a self-provider event re-applies the captured build rather
// than a possibly-stale remote copy.
//
// A failed re-fetch keeps the live view: it is never wiped because a refresh
// did not land.
//
// Dependencies are injected so the logic is testable without a live universe.
func RefreshOnContributionsChanged(
	appCtx context.Context,
	fetchAll func(context.Context) ([]*mbapi.Manifest, error),
	cat *manifestcore.MemoryCatalog,
	allNames []string,
	selfManifest *mbapi.Manifest,
	hooks []mbapi.RefreshHook,
	ev contribapi.ContributionsChangedEvent,
) {
	switch {
	case cat == nil:
		// No local index to patch — still fan out below.
	case ev.Provider != "" && selfManifest != nil && ev.Provider == selfManifest.Provider:
		cat.Put(selfManifest.Provider, selfManifest)
	case fetchAll == nil:
		slog.Warn("contributions-changed: no fetcher; skipping index refresh",
			"provider", ev.Provider, "kinds", ev.KindsChanged)
	default:
		manifests, err := fetchAll(appCtx)
		if err != nil {
			slog.Warn("contributions-changed: refetch failed; keeping live index",
				"error", err, "provider", ev.Provider)
			break
		}
		if ev.Provider != "" {
			patched := false
			for _, m := range manifests {
				if m != nil && m.Provider == ev.Provider {
					cat.Put(m.Provider, m)
					patched = true
					break
				}
			}
			if !patched {
				slog.Debug("contributions-changed: provider absent from refetched index (removed?)",
					"provider", ev.Provider)
			}
		} else {
			if selfManifest != nil {
				manifests = append(manifests, selfManifest)
			}
			cat.Replace(manifests, allNames)
		}
	}

	for _, h := range hooks {
		if h.Refresh == nil || !h.Matches(ev.KindsChanged) {
			continue
		}
		if err := h.Refresh(appCtx, ev); err != nil {
			slog.Warn("contributions-changed: refresh hook failed",
				"hook", h.Name, "provider", ev.Provider, "kinds", ev.KindsChanged, "error", err)
		}
	}
}

// bulkLane builds the object store the catalog serves large layers from.
//
// A local cache in front of wherever the content was published. The cache lives
// under CacheDir because that is exactly what it is — safe to delete, refilled
// by reading — and it starts empty: installing a pack must not download the
// content it references.
//
// Origins come from whoever knows where things were published, resolved as an
// ordinary service. A binary with no marketplaces registers none and reads only
// what it holds.
func bulkLane(ctx context.Context) manifestcore.BlobStore {
	cache := manifestcore.NewFileBlobStore(application.Env().GetCacheSubDir("blobs"))

	origins, err := application.UseService[mbapi.BlobOrigins](ctx)
	if err != nil || origins == nil {
		return cache
	}
	roots := origins.BlobRoots()
	if len(roots) == 0 {
		return cache
	}
	stores := make([]manifestcore.BlobStore, 0, len(roots))
	for _, r := range roots {
		stores = append(stores, manifestcore.NewFileBlobStore(r))
	}
	slog.Info("universe catalog: bulk lane", "origins", len(stores))
	return manifestcore.NewLayeredBlobStore(cache, stores...)
}
