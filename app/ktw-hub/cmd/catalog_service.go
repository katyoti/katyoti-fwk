// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// catalogListDefaultLimit is the page size when a caller passes Limit == 0.
// Every reply is one page: a reply that grew with the universe was the one
// unbounded path left on the wire (blind audit M5), and the client loop
// (manifestremote.Catalog.Index) follows NextCursor for everyone.
const catalogListDefaultLimit = 500

// catalogServiceImpl is the bus-friendly adapter over the in-process
// `mbapi.Catalog`. Every method delegates to the underlying Catalog
// resolved from the application's service container — meaning the
// reads share the SAME in-memory state the inventory watcher refreshes,
// and the SAME state the hub's hydration loop populated at boot.
//
// This is what lets ktw-gui (and any other bus client) ask "what skills
// exist?" without spawning a single sidecar.
type catalogServiceImpl struct {
	app *application.Application
	// cat, when set, is the catalog every read uses instead of the
	// application's — the unit-test seam; production leaves it nil.
	cat mbapi.Catalog
	// members and cache, when set, are the membership and the tier-1 cache a
	// folder member is read from — the unit-test seam; production leaves
	// them nil (the active universe's Univers.yaml and cache).
	members func() ([]appcore.SidecarSpec, error)
	cache   *manifestlocal.Cache
}

// folderMember is the providers of host when it is a folder member of the
// universe — read by the hub from its folder, no process asked.
func (s *catalogServiceImpl) folderMember(host string) ([]contribapi.ContributionProvider, bool, error) {
	members := s.members
	if members == nil {
		members = func() ([]appcore.SidecarSpec, error) {
			reg, err := universeMembers()
			if err != nil {
				return nil, err
			}
			return reg.Sidecars, nil
		}
	}
	sidecars, err := members()
	if err != nil {
		return nil, false, err
	}
	cache := s.cache
	if cache == nil {
		cache = hubCache()
	}
	return folderMemberProviders(host, sidecars, cache)
}

func newCatalogService(ctx context.Context) (mbapi.CatalogService, error) {
	app := application.AppFromContext(ctx)
	if app == nil {
		return nil, fmt.Errorf("catalog service factory: no Application in ctx")
	}
	return &catalogServiceImpl{app: app}, nil
}

func (s *catalogServiceImpl) catalog() (mbapi.Catalog, error) {
	if s.cat != nil {
		return s.cat, nil
	}
	return application.AppUseService[mbapi.Catalog](s.app)
}

// GetManifest returns one provider's cached manifest, BODY-FREE: every
// every layer above the inline band is dropped (its hash retained) so the response fits
// well under the NATS MaxPayload limit regardless of pack size. This is
// the INDEX plane — consumers fetch bodies on demand via GetBody. There
// is no full-body manifest path over the bus (no dual mode); the hub's
// on-disk cache stays full-body for GetBody to serve from.
func (s *catalogServiceImpl) GetManifest(ctx context.Context, in mbapi.GetManifestInput) (mbapi.GetManifestOutput, error) {
	cat, err := s.catalog()
	if err != nil {
		return mbapi.GetManifestOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	m, err := cat.Get(ctx, in.Provider)
	slog.Debug("catalog: GetManifest", "provider", in.Provider, "found", err == nil)
	if err != nil {
		if errors.Is(err, mbapi.ErrSidecarNotInCatalog) {
			// A member with no loaded manifest is a universe that answers less
			// than it declares; say which members ARE loaded so the gap is
			// visible in the hub's log, not only in a client's error.
			names, _ := cat.Names(ctx)
			loaded, _ := cat.List(ctx)
			held := make([]string, 0, len(loaded))
			for _, lm := range loaded {
				held = append(held, lm.Provider)
			}
			slog.Warn("catalog: GetManifest for a member with no loaded manifest",
				"provider", in.Provider, "members", names, "loaded", held)
			return mbapi.GetManifestOutput{}, ktsp.ErrSlotNotFound(in.Provider)
		}
		return mbapi.GetManifestOutput{}, err
	}
	out := stripManifestBodies(m)
	if in.SlotsOnly {
		// The callable lane only: a consumer that holds the paged index asks
		// for slots without receiving every entry again, unpaged.
		out.Contributions = nil
	}
	return fitManifestReply(out, in.SlotType, manifestReplyBudget), nil
}

// manifestReplyBudget is the largest manifest GetManifest answers in one
// reply: half the bus's message limit, leaving the envelope room.
const manifestReplyBudget = natsembedded.MaxPayload / 2

// fitManifestReply narrows m to the slot type asked, or — when m is larger
// than budget — answers its slot types by name for the caller to ask each
// one. A provider whose methods do not fit one message (a connector's generated
// API methods) would otherwise have its reply dropped by the bus and the
// caller wait out its timeout. m is a copy the caller owns.
func fitManifestReply(m *mbapi.Manifest, slotType string, budget int) mbapi.GetManifestOutput {
	if slotType != "" {
		m.SlotTypes = slotTypeNamed(m.SlotTypes, slotType)
		return mbapi.GetManifestOutput{Manifest: m}
	}
	if raw, err := json.Marshal(m); err == nil && len(raw) > budget {
		m.SlotTypes = slotTypeNames(m.SlotTypes)
		slog.Info("catalog: GetManifest answers the slot types by name: too large for one reply",
			"provider", m.Provider, "bytes", len(raw), "budget", budget)
		return mbapi.GetManifestOutput{Manifest: m, SplitSlotTypes: true}
	}
	return mbapi.GetManifestOutput{Manifest: m}
}

// slotTypeNamed keeps the one slot type named, on its declared or kebab form.
func slotTypeNamed(sts []mbapi.SlotType, name string) []mbapi.SlotType {
	for _, st := range sts {
		if st.Type == name || corereflect.ToKebab(st.Type) == name {
			return []mbapi.SlotType{st}
		}
	}
	return nil
}

// slotTypeNames is the slot types without their methods.
func slotTypeNames(sts []mbapi.SlotType) []mbapi.SlotType {
	out := make([]mbapi.SlotType, len(sts))
	for i, st := range sts {
		out[i] = mbapi.SlotType{Type: st.Type, Version: st.Version}
	}
	return out
}

// stripManifestBodies returns a shallow copy of the manifest whose
// Contributions carry no Body (BodyHash retained). The catalog hands out
// shared manifest pointers from its in-memory snapshot — mutating in
// place would corrupt the cache, so we copy the slice and clear Body on
// each (copied) entry.
func stripManifestBodies(m *mbapi.Manifest) *mbapi.Manifest {
	if m == nil {
		return nil
	}
	out := *m
	out.Contributions = make([]contribapi.Contribution, len(m.Contributions))
	for i, e := range m.Contributions {
		e.DropInline()
		out.Contributions[i] = e
	}
	return &out
}

func (s *catalogServiceImpl) ListNames(ctx context.Context, _ mbapi.ListNamesInput) (mbapi.ListNamesOutput, error) {
	cat, err := s.catalog()
	if err != nil {
		return mbapi.ListNamesOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	ns, err := cat.Names(ctx)
	if err != nil {
		return mbapi.ListNamesOutput{}, err
	}
	out := mbapi.ListNamesOutput{Names: ns, Hashes: manifestDigests(ctx, cat, ns), Unavailable: unavailableMembers(cat)}
	if s.members == nil {
		if _, refused, err := universeMembership(); err == nil {
			out.Refused = refused
		}
	}
	return out, nil
}

// ListOverrides gathers the Overrides of every cached manifest, from one
// tree when asked.
func (s *catalogServiceImpl) ListOverrides(ctx context.Context, in mbapi.ListOverridesInput) (mbapi.ListOverridesOutput, error) {
	cat, err := s.catalog()
	if err != nil {
		return mbapi.ListOverridesOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	ms, err := cat.List(ctx)
	if err != nil {
		return mbapi.ListOverridesOutput{}, err
	}
	out := mbapi.ListOverridesOutput{Overrides: []contribapi.Override{}}
	for _, m := range ms {
		if m == nil {
			continue
		}
		for _, o := range m.Overrides {
			if in.From == "" || o.From == in.From {
				out.Overrides = append(out.Overrides, o)
			}
		}
	}
	return out, nil
}

// unavailableMembers names the members whose declared binary is not on disk:
// their entries are served from the record, but no producer can be reached
// for them. Only a membership that declares binaries (the disk-backed
// catalog) can answer; a memory view declares none and reports none.
func unavailableMembers(cat mbapi.Catalog) []string {
	if c, ok := cat.(interface{ UnavailableMembers() []string }); ok {
		return c.UnavailableMembers()
	}
	return nil
}

// manifestDigests reads each cached member's manifest hash — the index digest
// a consumer's view is compared against on reconnect. A member with no
// inventory entry has no hash and is left out, so "absent" and "empty" stay
// distinguishable on the wire.
func manifestDigests(ctx context.Context, cat mbapi.Catalog, names []string) map[string]string {
	out := make(map[string]string, len(names))
	for _, name := range names {
		m, err := cat.Get(ctx, name)
		if err != nil || m == nil || m.Hash == "" {
			continue
		}
		out[name] = m.Hash
	}
	return out
}

// Translations answers the translation lane: the bundle of every loaded
// manifest in the language asked (mbapi.TranslationBundle).
func (s *catalogServiceImpl) Translations(ctx context.Context, in mbapi.TranslationsInput) (mbapi.TranslationsOutput, error) {
	if !contribapi.IsLanguageCode(in.Language) {
		return mbapi.TranslationsOutput{}, fmt.Errorf("language %q is not a two-letter language code", in.Language)
	}
	cat, err := s.catalog()
	if err != nil {
		return mbapi.TranslationsOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	manifests, err := cat.List(ctx)
	if err != nil {
		return mbapi.TranslationsOutput{}, err
	}
	return mbapi.TranslationsOutput{Language: in.Language, Texts: mbapi.TranslationBundle(manifests, in.Language)}, nil
}

// ListContributions answers the index lane: one page of every contribution
// matching the filter, in index form (DropInline — inline-band layers keep
// their bytes, every larger layer keeps its descriptor only).
func (s *catalogServiceImpl) ListContributions(ctx context.Context, in mbapi.CatalogListContributionsInput) (mbapi.CatalogListContributionsOutput, error) {
	page, pg, err := s.listPage(ctx, in.Filter, in.Limit, in.Offset, in.Cursor, false)
	if err != nil {
		return mbapi.CatalogListContributionsOutput{}, err
	}
	return mbapi.CatalogListContributionsOutput{Contributions: page, Pagination: pg}, nil
}

// ListResources is the compatibility adapter over ListContributions' code
// path (compat: removed after the next tagged release following 2026-09-25 —
// manifest/api doc.go, "Compatibility names"). Its only difference is
// IncludeBody: the paginated default is "no body" (saves NATS bytes); the
// unpaginated default is "with body" (legacy callers expect it).
func (s *catalogServiceImpl) ListResources(ctx context.Context, in mbapi.CatalogListResourcesInput) (mbapi.CatalogListResourcesOutput, error) {
	paginated := in.Limit > 0 || in.Cursor != "" || in.Offset > 0
	wantBody := in.IncludeBody || !paginated
	page, pg, err := s.listPage(ctx, in.Filter, in.Limit, in.Offset, in.Cursor, wantBody)
	if err != nil {
		return mbapi.CatalogListResourcesOutput{}, err
	}
	return mbapi.CatalogListResourcesOutput{Contributions: page, Pagination: pg}, nil
}

// listPage is the one index code path behind ListContributions and the
// compatibility ListResources.
//
// Every reply is one page (CatalogListContributionsInput godoc): Limit <= 0
// means catalogListDefaultLimit. The cursor is `<offset>@<index digest>`,
// the digest being a hash over every loaded manifest's hash in member order:
// a cursor cut from an index that has since reloaded restarts at offset 0
// rather than skipping or repeating entries across the two (blind audit L4).
// A bare-offset cursor from an older client is honoured without the check.
//
// wantBody false reduces each entry to its index form.
func (s *catalogServiceImpl) listPage(ctx context.Context, filter contribapi.ListFilter, limit, offsetIn int, cursor string, wantBody bool) ([]contribapi.Contribution, mbapi.Pagination, error) {
	cat, err := s.catalog()
	if err != nil {
		return nil, mbapi.Pagination{}, fmt.Errorf("resolve catalog: %w", err)
	}
	manifests, err := cat.List(ctx)
	if err != nil {
		return nil, mbapi.Pagination{}, err
	}
	digest := indexDigest(manifests)
	entries := filterEntries(manifests, filter)
	total := len(entries)

	offset, err := resolveOffset(cursor, offsetIn, digest)
	if err != nil {
		return nil, mbapi.Pagination{}, fmt.Errorf("decode cursor: %w", err)
	}
	if limit <= 0 {
		limit = catalogListDefaultLimit
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	page := entries[offset:end]
	hasMore := end < total

	if !wantBody {
		page = stripBodies(page)
	}

	next := ""
	if hasMore {
		next = strconv.Itoa(end) + cursorSep + digest
	}
	return page, mbapi.Pagination{
		Total:      total,
		PageSize:   len(page),
		Offset:     offset,
		HasMore:    hasMore,
		NextCursor: next,
	}, nil
}

// cursorSep separates the offset from the index digest in a cursor.
const cursorSep = "@"

// indexDigest names the index a page was cut from: a hash over every loaded
// manifest's hash, in member order. Two loads of the same records give the
// same digest; a reload that changed any member changes it.
func indexDigest(manifests []*mbapi.Manifest) string {
	h := sha256.New()
	for _, m := range manifests {
		if m == nil {
			continue
		}
		h.Write([]byte(m.Provider))
		h.Write([]byte{0})
		h.Write([]byte(m.Hash))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// resolveOffset reads Cursor with precedence over Offset (matches CRUD's
// hybrid pagination — see crudapi.ListInput godoc). A cursor stamped with a
// digest other than the current index's restarts at 0: the offsets it counted
// belong to an index that no longer exists.
func resolveOffset(cursor string, fallback int, digest string) (int, error) {
	if cursor == "" {
		if fallback < 0 {
			return 0, nil
		}
		return fallback, nil
	}
	offsetText, stamp, stamped := strings.Cut(cursor, cursorSep)
	n, err := strconv.Atoi(offsetText)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor %q", cursor)
	}
	if stamped && stamp != digest {
		return 0, nil
	}
	return n, nil
}

// stripBodies returns a copy of the slice with each entry's Body
// cleared. The catalog hands out shared Contribution values from its
// in-memory snapshot — mutating in place would corrupt the cache, so
// we copy each entry before nilling Body.
func stripBodies(in []contribapi.Contribution) []contribapi.Contribution {
	out := make([]contribapi.Contribution, len(in))
	for i, e := range in {
		e.DropInline()
		out[i] = e
	}
	return out
}

// GetBody serves the CONTENT plane: one contribution's body by URI,
// resolved straight from the hub's full-body manifests. The hub disk
// cache stays full-body (per the approved fork defaults), so this never
// re-fetches — it's a bounded read of a single body. The optional Hash
// is verified against the located body to catch index/content drift.
func (s *catalogServiceImpl) GetBody(ctx context.Context, in mbapi.GetBodyInput) (mbapi.GetBodyOutput, error) {
	cat, err := s.catalog()
	if err != nil {
		return mbapi.GetBodyOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	if in.URI == "" {
		return mbapi.GetBodyOutput{}, fmt.Errorf("GetBody: empty URI")
	}
	body, err := cat.GetBody(ctx, in.URI)
	if err != nil {
		if errors.Is(err, mbapi.ErrSidecarNotInCatalog) || errors.Is(err, contribapi.ErrContributionNotFound{}) {
			return mbapi.GetBodyOutput{}, ktsp.ErrSlotNotFound(in.URI)
		}
		return mbapi.GetBodyOutput{}, err
	}
	// The caller addressed the body by hash: the bytes located by URI must be
	// the ones that address names, or the caller's index and this catalog
	// disagree — refused as a mismatch, never served as if they agreed.
	if in.Hash != "" && len(body) > 0 {
		if err := contribapi.VerifyContent(in.URI, contribapi.LayerData, in.Hash, body); err != nil {
			return mbapi.GetBodyOutput{}, fmt.Errorf("CatalogService/GetBody: %w", err)
		}
	}
	return mbapi.GetBodyOutput{Content: body}, nil
}

// Read serves the READABLE-CONTENT plane: the raw content + media type a
// consumer would render for a URI. The hub parses the URI's provider,
// lazily spawns it via the ConnectionManager (Acquire/Release), and
// forwards the read to its ResourceService.Read over the bus. This is the
// path ktw-gui's Resources view uses — it can't address sidecars
// directly, so the hub mediates.
func (s *catalogServiceImpl) Read(ctx context.Context, in mbapi.CatalogReadInput) (mbapi.CatalogReadOutput, error) {
	if in.URI == "" {
		return mbapi.CatalogReadOutput{}, ktsp.ErrInvalidRequest("CatalogService/Read: empty URI")
	}
	parsed, err := contribapi.ParseResourceURI(in.URI)
	if err != nil {
		return mbapi.CatalogReadOutput{}, ktsp.ErrInvalidRequest(fmt.Sprintf("CatalogService/Read: bad URI %q: %v", in.URI, err))
	}
	// Route to the binary that actually HOSTS the contribution, not blindly
	// to the URI's provider token. Layer providers (e.g. the ktw-gocode
	// persona pack bundled into ktw-agents) advertise a provider token that
	// has NO standalone binary — the content lives inside another binary's
	// manifest. hostFor resolves that from the catalog; for ordinary
	// per-binary contributions host == provider, so this is a no-op there.
	// The hosting binary's ResourceService.Read serves the original URI (its
	// in-process provider is registered under the URI's provider token).
	host := s.hostFor(ctx, parsed, parsed.Provider)
	// A folder member runs no process: the hub reads its folder itself.
	if providers, isFolder, ferr := s.folderMember(host); ferr != nil {
		return mbapi.CatalogReadOutput{}, fmt.Errorf("CatalogService/Read: folder member %s: %w", host, ferr)
	} else if isFolder {
		layer := contribapi.LayerSource
		if cat, cerr := s.catalog(); cerr == nil {
			if resolver, ok := cat.(mbapi.ContributionResolver); ok {
				if entry, found := resolver.ContributionAt(in.URI); found {
					layer = mbapi.ReadLayer(entry)
				}
			}
		}
		content, mediaType, oerr := openFromProviders(ctx, providers, parsed.Type, parsed.ID, layer)
		if oerr != nil {
			return mbapi.CatalogReadOutput{}, fmt.Errorf("CatalogService/Read: %w", oerr)
		}
		return mbapi.CatalogReadOutput{Content: content, MediaType: mediaType}, nil
	}
	mgr, err := application.AppUseService[cmapi.ConnectionManager](s.app)
	if err != nil {
		return mbapi.CatalogReadOutput{}, fmt.Errorf("CatalogService/Read: resolve ConnectionManager: %w", err)
	}
	sender, err := mgr.Acquire(ctx, host)
	if err != nil {
		if errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
			return mbapi.CatalogReadOutput{}, ktsp.ErrSlotNotFound(host)
		}
		return mbapi.CatalogReadOutput{}, fmt.Errorf("CatalogService/Read: acquire %s: %w", host, err)
	}
	defer mgr.Release(host)

	// The ResourceService is published on the KTSP slot "resources" with
	// kebab method names. The connmgr sender is already bound to this one
	// provider, so we use the LOCAL ktsp.Sender path form `resources/read`
	// (NOT `<provider>:resources/read`: the stdio session's ParsePath splits
	// on '/' and would read the whole `<provider>:resources` as the slot →
	// "Slot not found"). For bus transports the connmgr wrapper qualifies it.
	raw, err := sender.Call(ctx, "resources/read", mbapi.ContributionReadInput{URI: in.URI})
	if err != nil {
		return mbapi.CatalogReadOutput{}, fmt.Errorf("CatalogService/Read: %s ResourceService.Read: %w", parsed.Provider, err)
	}
	var out mbapi.ContributionReadOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return mbapi.CatalogReadOutput{}, fmt.Errorf("CatalogService/Read: decode %s ResourceService.Read output: %w", parsed.Provider, err)
	}
	return mbapi.CatalogReadOutput{Content: out.Content, MediaType: out.MediaType}, nil
}

// readServes reports whether the producer's Read — which answers with ONE
// fixed layer, mbapi.ReadLayer — yields the bytes of `layer`: that
// layer itself, or any other layer with the same content address. A kind
// whose `source` and `data` are one JSON file (capability-panel) declares two
// layers with one hash; asking which layer is the "default" was the wrong
// question and refused the second one.
func readServes(entry contribapi.Contribution, layer string) bool {
	want := entry.Layers[layer].Hash
	served := entry.Layers[mbapi.ReadLayer(entry)].Hash
	return want != "" && want == served
}

// OpenLayer implements the content lane over the wire: one named layer of one
// contribution, band-aware, answered by contribapi.OpenLayerFrom — the one
// open every reach shares.
//
// The order is the two-plane model: a layer resident in the aggregator's
// manifest (tier 1) is served from there and contacts nobody; a bulk-band layer
// is refused with its address, because no message payload can carry it; any
// other declared layer is fetched from the producer — through Read when Read
// answers with these very bytes (readServes), else through the producer's own
// OpenLayer by name — and checked against its address. A layer the entry does
// not declare is refused before any producer is contacted.
func (s *catalogServiceImpl) OpenLayer(ctx context.Context, in mbapi.OpenLayerInput) (mbapi.OpenLayerOutput, error) {
	if in.URI == "" {
		return mbapi.OpenLayerOutput{}, ktsp.ErrInvalidRequest("CatalogService/OpenLayer: empty URI")
	}
	cat, err := s.catalog()
	if err != nil {
		return mbapi.OpenLayerOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	resolver, ok := cat.(mbapi.ContributionResolver)
	if !ok {
		return mbapi.OpenLayerOutput{}, fmt.Errorf("CatalogService/OpenLayer: catalog cannot address entries")
	}
	entry, found := resolver.ContributionAt(in.URI)
	if !found {
		return mbapi.OpenLayerOutput{}, ktsp.ErrSlotNotFound(in.URI)
	}
	layer := in.Layer
	if layer == "" {
		kinds, err := mbapi.KindsAt(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
		if err != nil {
			return mbapi.OpenLayerOutput{}, err
		}
		layer = kinds.DefaultLayer(entry.Type)
	}
	rc, ref, err := contribapi.OpenLayerFrom(entry, layer, contribapi.LayerLanes{
		// Not carried here: the producer has it.
		Fetch: func(contribapi.LayerRef) ([]byte, string, error) {
			if readServes(entry, layer) {
				read, err := s.Read(ctx, mbapi.CatalogReadInput{URI: in.URI})
				if err != nil {
					return nil, "", err
				}
				return read.Content, read.MediaType, nil
			}
			// A folder member's layer, read from its folder by the hub.
			if parsed, perr := contribapi.ParseResourceURI(in.URI); perr == nil {
				host := s.hostFor(ctx, parsed, parsed.Provider)
				if providers, isFolder, ferr := s.folderMember(host); ferr != nil {
					return nil, "", ferr
				} else if isFolder {
					return openFromProviders(ctx, providers, parsed.Type, parsed.ID, layer)
				}
			}
			var opened mbapi.OpenLayerOutput
			if err := s.callProducer(ctx, in.URI, "resources/open-layer", mbapi.OpenLayerInput{URI: in.URI, Layer: layer}, &opened); err != nil {
				return nil, "", fmt.Errorf("CatalogService/OpenLayer: %w", err)
			}
			if rerr := mbapi.RefusedErr(in.URI, layer, opened); rerr != nil {
				return nil, "", rerr
			}
			return opened.Content, opened.Ref.MediaType, nil
		},
	})
	if err != nil {
		// The wire's two refusal shapes: a refusal (bulk, not carried)
		// travels as Refused with the layer's address; an undeclared layer is
		// a bad request. Both texts are the typed errors' own.
		if text, ok := mbapi.Refusal(err); ok {
			return mbapi.OpenLayerOutput{Ref: ref, Refused: text}, nil
		}
		var undeclared contribapi.ErrLayerNotDeclared
		if errors.As(err, &undeclared) {
			return mbapi.OpenLayerOutput{}, ktsp.ErrInvalidRequest("CatalogService/OpenLayer: " + undeclared.Error())
		}
		return mbapi.OpenLayerOutput{}, err
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		return mbapi.OpenLayerOutput{}, err
	}
	return mbapi.OpenLayerOutput{Ref: ref, Content: content}, nil
}

// callProducer routes one ResourceService call to the binary that carries the
// contribution (hostFor), through the connection manager, and decodes the
// answer into out. Read and OpenLayer share it; nothing else contacts a
// producer.
func (s *catalogServiceImpl) callProducer(ctx context.Context, uri, method string, in any, out any) error {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return ktsp.ErrInvalidRequest(fmt.Sprintf("bad URI %q: %v", uri, err))
	}
	mgr, err := application.AppUseService[cmapi.ConnectionManager](s.app)
	if err != nil {
		return fmt.Errorf("resolve ConnectionManager: %w", err)
	}
	host := s.hostFor(ctx, parsed, parsed.Provider)
	sender, err := mgr.Acquire(ctx, host)
	if err != nil {
		if errors.Is(err, cmapi.ErrSidecarNotInUniverse) {
			return ktsp.ErrSlotNotFound(host)
		}
		return fmt.Errorf("acquire %s: %w", host, err)
	}
	defer mgr.Release(host)
	raw, err := sender.Call(ctx, method, in)
	if err != nil {
		return fmt.Errorf("%s %s: %w", parsed.Provider, method, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s output: %w", parsed.Provider, method, err)
	}
	return nil
}

// hostFor returns the binary (catalog member) whose manifest actually carries
// the given contribution — the binary that must be Acquired to Read it. A
// layer provider's contributions live inside another binary's manifest (its
// effective Provider differs from the carrying manifest's Provider), and the
// URI's provider token may have no standalone binary at all. We find the
// carrying manifest by matching effective provider/type/id (mirroring the
// catalog's findBody) and return that manifest's own Provider — the acquirable
// binary name. Falls back to `fallback` (the URI provider) when the catalog
// doesn't place the contribution, so ordinary per-binary reads are unchanged.
func (s *catalogServiceImpl) hostFor(_ context.Context, uri contribapi.ResourceURI, fallback string) string {
	cat, err := s.catalog()
	if err != nil {
		return fallback
	}
	// Resolved from the catalog's index rather than by walking every manifest.
	// This runs on EVERY read, so a scan here costs more than anywhere else in
	// the system.
	if h, ok := cat.(interface {
		HostOf(contribapi.ResourceURI) string
	}); ok {
		if host := h.HostOf(uri); host != "" {
			return host
		}
	}
	return fallback
}

func (s *catalogServiceImpl) ListURIs(ctx context.Context, in mbapi.CatalogListURIsInput) (mbapi.CatalogListURIsOutput, error) {
	cat, err := s.catalog()
	if err != nil {
		return mbapi.CatalogListURIsOutput{}, fmt.Errorf("resolve catalog: %w", err)
	}
	entries, err := filterEntriesAcrossManifests(ctx, cat, in.Filter)
	if err != nil {
		return mbapi.CatalogListURIsOutput{}, err
	}
	uris := make([]contribapi.ResourceURI, 0, len(entries))
	for _, e := range entries {
		uris = append(uris, e.TypedURI())
	}
	return mbapi.CatalogListURIsOutput{URIs: uris}, nil
}

// filterEntriesAcrossManifests walks every cached manifest and accumulates
// entries matching the filter. Server-side filter — the cheap path the
// CatalogReader interface depends on.
func filterEntriesAcrossManifests(ctx context.Context, cat mbapi.Catalog, filter contribapi.ListFilter) ([]contribapi.Contribution, error) {
	manifests, err := cat.List(ctx)
	if err != nil {
		return nil, err
	}
	return filterEntries(manifests, filter), nil
}

// filterEntries accumulates the entries of the given manifests (member order)
// that match the filter. One URI, one entry: when two members carry the same
// URI the first in member order is the one every by-URI read resolves to
// (the catalog's host index), so a listing answers with that copy and not
// with both.
func filterEntries(manifests []*mbapi.Manifest, filter contribapi.ListFilter) []contribapi.Contribution {
	out := make([]contribapi.Contribution, 0)
	seen := map[string]struct{}{}
	for _, m := range manifests {
		if m == nil {
			continue
		}
		for _, e := range m.Contributions {
			if !filter.IsEmpty() && !filter.Matches(e.TypedURI()) {
				continue
			}
			uri := e.ResolvedURI()
			if _, dup := seen[uri]; dup {
				continue
			}
			seen[uri] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}

// catalogServiceExtension publishes CatalogService as a KTSP slot on
// the hub. Subject form: `ktw-hub:/catalog/<method>`. Clients (ktw-gui,
// agent system-prompt builder) read the universe-wide catalog over the
// bus instead of spawning sidecars for enumeration.
//
// Wired separately from catalogExtension because the latter publishes
// the in-process Catalog (no KTSP capability); this one wraps it in a
// bus-shape and adds the auto-published slot.
func catalogServiceExtension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[mbapi.CatalogService](
			newCatalogService,
			appcore.WithServiceDef(mbapi.CatalogServiceDef),
			appcore.WithKTSPContainer(),
			// CLI surface is not generated — operators use `ktw-hub catalog`
			// (the existing Cobra group); this is a wire contract for
			// remote bus clients.
			appcore.WithKTSPSlotTags(map[string]string{"cli": "false"}),
		))
	}
}
