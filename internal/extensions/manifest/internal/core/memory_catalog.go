// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"sort"
	"sync"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// BodyFetcher fetches ONE contribution body over the bus by content
// hash — the per-hash RPC behind the CONTENT plane. Injected into the
// MemoryCatalog so the manifest package takes no transport dependency.
// Returns (nil, nil) when the contribution exists but has no body.
type BodyFetcher func(ctx context.Context, hash, uri string) ([]byte, error)

// LayerFetcher resolves ONE named layer of one contribution the view does not
// carry — through the aggregator's OpenLayer, which forwards to the producer
// when the aggregate does not hold it either. The body lane (BodyFetcher) can
// only answer the data layer; a consumer opening `instructions` or `source`
// needs this lane, or it gets the data layer under another layer's name and
// fails the hash check (ms-contribution-ext e9, 2026-09-22).
type LayerFetcher func(ctx context.Context, uri, layer string) ([]byte, contribapi.LayerRef, error)

// MemoryCatalog is an in-memory `mbapi.Catalog` populated from an
// initial set of manifests. Unlike `CatalogImpl` it has no disk
// inventory backing — useful for binaries (like ktw-agents) that
// gather descriptors via in-process discovery (`RunEndpointExtensions`)
// and don't need cross-process cache sharing.
//
// The membership list is derived from the provided manifests'
// `Provider` field plus any explicitly-declared additional names
// passed at construction time (so a sidecar listed in
// `Univers.yaml` but currently uncached still shows up in
// `Names()`, matching the disk-backed catalog's semantics).
//
// `Replace(manifests)` swaps the contents under the mutex — useful
// when discovery refreshes.
type MemoryCatalog struct {
	mu      sync.RWMutex
	members []string                   // declared membership (provider names)
	by      map[string]*mbapi.Manifest // provider name â†’ manifest

	// Two-plane CONTENT lane (optional). When the catalog holds a
	// body-free index, GetBody resolves bodies via the content store
	// (zero-bus hit) and, on a miss, the per-hash BodyFetcher (one
	// bounded bus call). Both nil for a test stub holding full bodies —
	// GetBody then returns the inline body directly.
	content *ContentStore
	fetch   BodyFetcher
	// layers answers a named, non-resident, non-bulk layer by name. Optional:
	// without it OpenLayer falls back to the body lane by hash, which is right
	// only for a layer whose bytes the body lane already holds.
	layers LayerFetcher

	// blobs serves the BULK band: layers too large for any message payload,
	// fetched out of band from an object store by hash. Optional — without
	// it a bulk layer reports that its content is unreachable rather than
	// pretending the control plane could carry it.
	blobs BlobStore

	// index maps a contribution's URI to its entry, rebuilt whenever the
	// manifest set changes.
	//
	// Without it, resolving ONE contribution walks every manifest and every
	// contribution in the universe — and the typed read path calls that
	// resolution once per entry, so listing a kind costs (entries × matches)
	// comparisons under a global lock. The index is derived state, so it is
	// rebuilt rather than maintained: correctness comes from rebuilding at the
	// same moments the manifests change.
	index map[string]*contribapi.Contribution

	// collisions records every URI more than one member carries, as
	// CatalogImpl does: the first member in declared order answers.
	collisions []URICollision

	// overrideFindings are the carried override entries the last rebuild
	// could not apply, as CatalogImpl records them.
	overrideFindings []string
}

// reindex rebuilds the URI index in MEMBER ORDER, so a URI two members carry
// resolves to the same entry on every rebuild — the first member's — and the
// others are recorded as a URICollision (blind audit M4); then it applies the
// override entries the members carry, as CatalogImpl does. Caller must hold
// c.mu for writing.
func (c *MemoryCatalog) reindex() {
	n := 0
	for _, m := range c.by {
		if m != nil {
			n += len(m.Contributions)
		}
	}
	idx := make(map[string]*contribapi.Contribution, n)
	loc := make(map[string]entryLoc, n)
	host := make(map[string]string, n)
	var collisions []URICollision
	at := map[string]int{}
	for _, name := range c.members {
		m := c.by[name]
		if m == nil {
			continue
		}
		for i := range m.Contributions {
			uri := uriOf(m, &m.Contributions[i])
			if h, taken := host[uri]; taken {
				if h == m.Provider {
					continue
				}
				k, seen := at[uri]
				if !seen {
					k = len(collisions)
					at[uri] = k
					collisions = append(collisions, URICollision{URI: uri, Host: h})
				}
				collisions[k].Shadowed = append(collisions[k].Shadowed, m.Provider)
				continue
			}
			host[uri] = m.Provider
			idx[uri] = &m.Contributions[i]
			loc[uri] = entryLoc{m: m, i: i}
		}
	}
	c.index = idx
	c.collisions = collisions
	ordered := make([]*mbapi.Manifest, 0, len(c.members))
	for _, name := range c.members {
		ordered = append(ordered, c.by[name])
	}
	c.overrideFindings = applyCarriedOverrideEntries(ordered, loc, func() contribapi.Kinds {
		var all []contribapi.Contribution
		for _, m := range ordered {
			if m != nil {
				all = append(all, m.Contributions...)
			}
		}
		return contribapi.KindsFrom(all)
	})
}

// OverrideFindings are the carried override entries the last rebuild could
// not apply, each saying why — as CatalogImpl.OverrideFindings.
func (c *MemoryCatalog) OverrideFindings() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]string(nil), c.overrideFindings...)
}

// URICollisions reports every URI more than one member carries, as of the
// last rebuild. Empty when every URI has one carrier.
func (c *MemoryCatalog) URICollisions() []URICollision {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]URICollision(nil), c.collisions...)
}

// SetContentLane wires the CONTENT plane: a content-addressed store
// (zero-bus hits) plus a per-hash BodyFetcher (one bounded bus call on
// a store miss). Call once after construction on the consumer side.
// Leaving it unset keeps GetBody serving inline bodies (test stub /
// full-body catalogs).
func (c *MemoryCatalog) SetContentLane(store *ContentStore, fetch BodyFetcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.content = store
	c.fetch = fetch
}

// SetLayerLane wires the fetch for a named layer the view does not carry.
// Fetched bytes are checked against the layer's address and cached in the
// content store by hash, so a second open of the same layer costs nothing.
func (c *MemoryCatalog) SetLayerLane(fetch LayerFetcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.layers = fetch
}

// NewMemoryCatalog builds an in-memory catalog from a slice of manifests.
// `extraMembers` lists provider names that should appear in `Names()`
// even without a cached manifest (so callers see the full Univers.yaml
// membership, not just successfully-probed sidecars). Duplicates are
// collapsed; iteration order matches the order names first appear in
// (extraMembers first, then manifests).
func NewMemoryCatalog(manifests []*mbapi.Manifest, extraMembers []string) *MemoryCatalog {
	seen := map[string]struct{}{}
	members := make([]string, 0, len(manifests)+len(extraMembers))
	for _, n := range extraMembers {
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		members = append(members, n)
	}
	by := make(map[string]*mbapi.Manifest, len(manifests))
	for _, m := range manifests {
		if m == nil || m.Provider == "" {
			continue
		}
		by[m.Provider] = m
		if _, ok := seen[m.Provider]; !ok {
			seen[m.Provider] = struct{}{}
			members = append(members, m.Provider)
		}
	}
	c := &MemoryCatalog{members: members, by: by}
	c.reindex()
	return c
}

// Replace atomically swaps the catalog's contents. Members list is
// recomputed from the new manifests plus any extras passed at the
// last constructor call (extras are NOT preserved across Replace —
// pass them again if needed).
func (c *MemoryCatalog) Replace(manifests []*mbapi.Manifest, extraMembers []string) {
	next := NewMemoryCatalog(manifests, extraMembers)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.members = next.members
	c.by = next.by
	c.index = next.index
	c.collisions = next.collisions
	c.overrideFindings = next.overrideFindings
}

// Put inserts or replaces a single provider's manifest under the mutex —
// the per-provider patch used by event-driven incremental refresh (a
// `ContributionsChangedEvent` scoped to one provider). A nil manifest or an
// empty Provider is ignored. A new provider is appended to the membership list;
// an existing one keeps its membership position. The CONTENT lane (store +
// fetcher) is untouched — bodies stay content-addressed and self-heal on the
// next Read because the new manifest's BodyHash drives the lookup.
func (c *MemoryCatalog) Put(provider string, m *mbapi.Manifest) {
	if provider == "" || m == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.by[provider] = m
	defer c.reindex()
	for _, n := range c.members {
		if n == provider {
			return // already a declared member — keep its position
		}
	}
	c.members = append(c.members, provider)
}

// Get returns the manifest for the named provider or
// ErrSidecarNotInCatalog if it isn't loaded. Matches `CatalogImpl.Get`
// semantics so consumers can swap implementations transparently.
func (c *MemoryCatalog) Get(_ context.Context, name string) (*mbapi.Manifest, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	m, ok := c.by[name]
	if !ok {
		return nil, mbapi.ErrSidecarNotInCatalog
	}
	return m, nil
}

// List returns every loaded manifest, sorted by provider name for
// deterministic iteration.
func (c *MemoryCatalog) List(_ context.Context) ([]*mbapi.Manifest, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*mbapi.Manifest, 0, len(c.by))
	for _, m := range c.by {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Provider < out[j].Provider })
	return out, nil
}

// Names returns the full membership in declared order (including
// providers that don't have a loaded manifest).
func (c *MemoryCatalog) Names(_ context.Context) ([]string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, len(c.members))
	copy(out, c.members)
	return out, nil
}

// GetContributionsByType walks every loaded manifest and aggregates resource
// entries with the matching type. Output is sorted by (provider, id)
// for deterministic iteration — matches `CatalogImpl.GetContributionsByType`.
func (c *MemoryCatalog) GetContributionsByType(_ context.Context, resourceType string) ([]contribapi.Contribution, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []contribapi.Contribution
	for _, m := range c.by {
		for i, e := range m.Contributions {
			if e.Type != resourceType {
				continue
			}
			// One URI, one entry: a copy another member shadows is the one
			// the index does not point at, and a listing answers with the
			// same entry a by-URI read does.
			if c.index[uriOf(m, &m.Contributions[i])] != &m.Contributions[i] {
				continue
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// GetBody implements Catalog. Resolves the body for one contribution URI
// via the two-plane CONTENT lane:
//
//  1. Locate the index entry, take its data layer's hash. If the entry
//     already carries the data inline (full-body catalog / test stub),
//     return it.
//  2. Content-store hit by hash → return (zero bus).
//  3. Miss → one bounded per-hash bus fetch via BodyFetcher, VERIFIED
//     against the hash (contribapi.ErrContentMismatch otherwise), stored,
//     returned.
//
// Returns (nil, nil) for a contribution with no data layer. A URI the view
// does not hold is ErrSidecarNotInCatalog wrapping
// contribapi.ErrContributionNotFound.
func (c *MemoryCatalog) GetBody(ctx context.Context, uri string) ([]byte, error) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, fmt.Errorf("memory catalog GetBody: parse %q: %w", uri, err)
	}

	c.mu.RLock()
	entry, found := c.findEntry(parsed)
	store := c.content
	fetch := c.fetch
	c.mu.RUnlock()

	if !found {
		return nil, notInCatalog(uri)
	}
	// Inline data present (full-body catalog / stub) — done. The data layer
	// only: a kind that keeps its content in the source layer answers nil
	// here, and is read through OpenLayer by naming that layer.
	if b := entry.Data(); len(b) > 0 {
		return b, nil
	}
	hash := entry.DataHash()
	if hash == "" {
		// No data layer addressed — the contribution genuinely has none.
		return nil, nil
	}
	// Content-store hit (zero bus).
	if store != nil {
		if b, ok := store.Get(hash); ok {
			return b, nil
		}
	}
	// Miss → one bounded per-hash fetch over the bus, verified against the
	// address before it is stored or served.
	if fetch == nil {
		return nil, fmt.Errorf("memory catalog GetBody: %s: %w", uri, contribapi.ErrLayerNotCarried{URI: uri, Layer: contribapi.LayerData})
	}
	body, err := fetchVerified(ctx, fetch, store, hash, uri)
	if err != nil {
		return nil, fmt.Errorf("memory catalog GetBody: %w", err)
	}
	return body, nil
}

// findEntry locates the contribution matching the URI across every
// loaded manifest (matching the effective provider). Caller must hold
// c.mu (read).
func (c *MemoryCatalog) findEntry(uri contribapi.ResourceURI) (contribapi.Contribution, bool) {
	if e, ok := c.index[uri.String()]; ok && e != nil {
		return *e, true
	}
	return contribapi.Contribution{}, false
}

// SetSchema implements Catalog. MemoryCatalog ignores the schema — it's
// a test stub for in-memory aggregation. Returns nil so the bootstrap
// protocol's call chain works in tests.
func (c *MemoryCatalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }

// Reindex implements Catalog. MemoryCatalog is the test stub; reindex
// is a no-op.
func (c *MemoryCatalog) Reindex(_ context.Context, _ []string) error { return nil }

// MarkReady implements Catalog. MemoryCatalog is always-ready; the call
// is a no-op.
func (c *MemoryCatalog) MarkReady(_ context.Context) error { return nil }

// Ready implements Catalog. MemoryCatalog is always-ready (test stub).
func (c *MemoryCatalog) Ready() bool { return true }

// Compile-time assertion.
var _ mbapi.Catalog = (*MemoryCatalog)(nil)

// ContributionAt resolves one contribution by URI through the index, without
// materialising its kind. Satisfies mbapi.ContributionResolver.
func (c *MemoryCatalog) ContributionAt(uri string) (contribapi.Contribution, bool) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return contribapi.Contribution{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.findEntry(parsed)
}
