// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"context"
	"fmt"
	"sync"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// Snapshot is the answer HELD, which this package's doc has promised since it
// was written and nothing implemented (found 2026-09-25).
//
// THE COST IT REMOVES. `Catalog` holds no cache by design: what it serves is
// what the aggregator said. But `GetBody` has to locate the entry first, and
// locating it meant `Index()`, which refetches the WHOLE paginated index over
// the bus. So a consumer reading N bodies paid N full index fetches plus N body
// fetches — measured on the GUI's task gallery: 63 tasks, ~127 round trips,
// four seconds, every time the panel drew.
//
// A snapshot fetches the index ONCE and answers locating questions from memory.
// The bodies still come from the aggregator, because they are not in the index
// and pretending otherwise is what the two-plane split exists to prevent — but
// one body now costs one call rather than one call plus a re-read of the world.
//
// IT IS EXPLICITLY A SNAPSHOT. It does not watch for changes and does not
// invalidate: it is the answer as of the moment it was taken, for a consumer
// doing one piece of work. A consumer that needs the current answer takes
// another. Making it self-refreshing would make it a cache, and a cache with
// no invalidation protocol is the bug this replaces wearing a better name.
type Snapshot struct {
	sender ktsapi.Sender
	index  []contribapi.Contribution
	byURI  map[string]int

	mu     sync.Mutex
	bodies map[string][]byte
}

// TakeSnapshot fetches the index once and returns a reader over it.
func TakeSnapshot(ctx context.Context, sender ktsapi.Sender) (*Snapshot, error) {
	c := New(sender)
	index, err := c.Index(ctx)
	if err != nil {
		return nil, err
	}
	s := &Snapshot{
		sender: sender,
		index:  index,
		byURI:  make(map[string]int, len(index)),
		bodies: map[string][]byte{},
	}
	for i, e := range index {
		// FIRST WINS on a duplicate URI, matching the aggregate's own read
		// order. A second entry under one URI is a catalog defect, reported
		// there; a snapshot that silently preferred the last would make the
		// two disagree about which one that is.
		if _, dup := s.byURI[e.ResolvedURI()]; !dup {
			s.byURI[e.ResolvedURI()] = i
		}
	}
	return s, nil
}

// Index returns the snapshot's entries. No call is made: this is what the
// snapshot IS.
func (s *Snapshot) Index(_ context.Context) ([]contribapi.Contribution, error) {
	return s.index, nil
}

// GetContributionsByType filters the held index.
func (s *Snapshot) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, e := range s.index {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

// List reconstructs one manifest per provider from the held index.
//
// The entries are what the aggregate carries; the per-provider descriptor
// fields it does not hold are left zero rather than invented, because a
// manifest assembled from an index is an INDEX view of that provider, not the
// provider's own statement about itself.
func (s *Snapshot) List(_ context.Context) ([]*mbapi.Manifest, error) {
	order := []string{}
	byProvider := map[string]*mbapi.Manifest{}
	for _, e := range s.index {
		m, ok := byProvider[e.Provider]
		if !ok {
			m = &mbapi.Manifest{Provider: e.Provider}
			byProvider[e.Provider] = m
			order = append(order, e.Provider)
		}
		m.Contributions = append(m.Contributions, e)
	}
	out := make([]*mbapi.Manifest, 0, len(order))
	for _, p := range order {
		out = append(out, byProvider[p])
	}
	return out, nil
}

// GetBody serves the entry's data layer, fetching it once when the index does
// not carry it.
//
// MEMOISED PER SNAPSHOT: asking for one body twice costs one call. That is not
// a cache across time — the snapshot is a moment — it is not asking the same
// question twice within one piece of work.
func (s *Snapshot) GetBody(ctx context.Context, uri string) ([]byte, error) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, fmt.Errorf("snapshot GetBody: parse %q: %w", uri, err)
	}
	key := parsed.String()
	i, ok := s.byURI[key]
	if !ok {
		return nil, fmt.Errorf("%w: %w", mbapi.ErrSidecarNotInCatalog, contribapi.ErrContributionNotFound{URI: uri})
	}
	entry := s.index[i]
	if b := entry.Data(); len(b) > 0 {
		return b, nil
	}
	if entry.DataHash() == "" {
		return nil, nil // no data layer — a data-type keeps its content in source
	}
	s.mu.Lock()
	if b, hit := s.bodies[key]; hit {
		s.mu.Unlock()
		return b, nil
	}
	s.mu.Unlock()

	body, err := New(s.sender).fetchBody(ctx, uri, entry.DataHash())
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.bodies[key] = body
	s.mu.Unlock()
	return body, nil
}

var _ mbapi.CatalogReader = (*Snapshot)(nil)
