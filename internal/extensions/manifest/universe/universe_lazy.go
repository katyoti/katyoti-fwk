// SPDX-License-Identifier: Apache-2.0

package universe

import (
	"context"
	"fmt"
	"io"
	"sync"

	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcore "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/internal/core"
)

// universe_lazy.go — the universe view exists as a service from startup, but
// is not assembled until something asks for it.
//
// Both halves matter. Registering the service eagerly is what lets it
// deterministically replace the self-only view, with no dependence on which
// registration ran last. Building it lazily is what keeps a binary that never
// queries the universe from opening a connection: `--help` pays nothing.

// lazyCatalog builds its inner catalog on first use.
//
// Every method goes through `resolve`, so there is no way to reach the inner
// catalog without building it — a forwarding method that forgot to would
// silently answer from nothing.
type lazyCatalog struct {
	once  sync.Once
	build func(context.Context) (mbapi.Catalog, error)
	inner mbapi.Catalog
	err   error
}

// newLazyCatalog wraps a builder. A build failure is remembered, not retried:
// the builder already degrades to this binary's own view when the aggregator is
// unreachable, so an error here means something structural, and retrying it on
// every call would turn one failure into a storm.
func newLazyCatalog(build func(context.Context) (mbapi.Catalog, error)) *lazyCatalog {
	return &lazyCatalog{build: build}
}

func (l *lazyCatalog) resolve(ctx context.Context) mbapi.Catalog {
	l.once.Do(func() { l.inner, l.err = l.build(ctx) })
	return l.inner
}

// Concrete returns the built catalog as a *MemoryCatalog when it is one, for
// the refresher, which needs Put and Replace — neither of which the Catalog
// interface exposes.
func (l *lazyCatalog) Concrete(ctx context.Context) *manifestcore.MemoryCatalog {
	if m, ok := l.resolve(ctx).(*manifestcore.MemoryCatalog); ok {
		return m
	}
	return nil
}

func (l *lazyCatalog) Get(ctx context.Context, name string) (*mbapi.Manifest, error) {
	c := l.resolve(ctx)
	if c == nil {
		return nil, l.err
	}
	return c.Get(ctx, name)
}

func (l *lazyCatalog) List(ctx context.Context) ([]*mbapi.Manifest, error) {
	c := l.resolve(ctx)
	if c == nil {
		return nil, l.err
	}
	return c.List(ctx)
}

func (l *lazyCatalog) Names(ctx context.Context) ([]string, error) {
	c := l.resolve(ctx)
	if c == nil {
		return nil, l.err
	}
	return c.Names(ctx)
}

func (l *lazyCatalog) GetContributionsByType(ctx context.Context, t string) ([]contribapi.Contribution, error) {
	c := l.resolve(ctx)
	if c == nil {
		return nil, l.err
	}
	return c.GetContributionsByType(ctx, t)
}

func (l *lazyCatalog) GetBody(ctx context.Context, uri string) ([]byte, error) {
	c := l.resolve(ctx)
	if c == nil {
		return nil, l.err
	}
	return c.GetBody(ctx, uri)
}

func (l *lazyCatalog) SetSchema(s mbapi.CatalogSchemaService) error {
	c := l.resolve(context.Background())
	if c == nil {
		return l.err
	}
	return c.SetSchema(s)
}

func (l *lazyCatalog) Reindex(ctx context.Context, uris []string) error {
	c := l.resolve(ctx)
	if c == nil {
		return l.err
	}
	return c.Reindex(ctx, uris)
}

func (l *lazyCatalog) MarkReady(ctx context.Context) error {
	c := l.resolve(ctx)
	if c == nil {
		return l.err
	}
	return c.MarkReady(ctx)
}

// Ready reports readiness WITHOUT forcing a build: asking whether the view is
// ready must not be the thing that assembles it.
func (l *lazyCatalog) Ready() bool {
	if l.inner == nil {
		return false
	}
	return l.inner.Ready()
}

// ContributionAt satisfies mbapi.ContributionResolver so an addressed read
// stays O(1) through the wrapper instead of falling back to listing a kind.
// OpenLayer implements mbapi.LayerOpener by forwarding to the concrete view:
// the content lane at universe reach requires it, and until 2026-09-22 this
// wrapper did not forward it, so every `mbapi.Open` from an agent process —
// the skills-get-instruction tool first — failed with "this aggregate cannot
// open layers" (ms-contribution-ext e9).
func (l *lazyCatalog) OpenLayer(ctx context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	cat := l.resolve(ctx)
	if cat == nil {
		return nil, contribapi.LayerRef{}, fmt.Errorf("universe view: open %s: view not built: %w", uri, l.err)
	}
	opener, ok := cat.(mbapi.LayerOpener)
	if !ok {
		return nil, contribapi.LayerRef{}, fmt.Errorf("universe view: open %s: the built catalog (%T) cannot open layers", uri, cat)
	}
	return opener.OpenLayer(ctx, uri, layer)
}

var _ mbapi.LayerOpener = (*lazyCatalog)(nil)

func (l *lazyCatalog) ContributionAt(uri string) (contribapi.Contribution, bool) {
	r, ok := l.resolve(context.Background()).(mbapi.ContributionResolver)
	if !ok {
		return contribapi.Contribution{}, false
	}
	return r.ContributionAt(uri)
}

var (
	_ mbapi.Catalog              = (*lazyCatalog)(nil)
	_ mbapi.ContributionResolver = (*lazyCatalog)(nil)
)

// Overrides implements mbapi.OverrideReader: what the hub's manifests say
// override trees served (CatalogService/ListOverrides). Asked of the
// aggregator, which holds every member's manifest — the view built from its
// index carries contributions, not where their layers came from.
func (l *lazyCatalog) Overrides(ctx context.Context, from string) ([]contribapi.Override, error) {
	sender, err := hubSender(connectAggregator(ctx))
	if err != nil {
		return nil, err
	}
	return manifestremote.New(sender).Overrides(ctx, from)
}

var _ mbapi.OverrideReader = (*lazyCatalog)(nil)
