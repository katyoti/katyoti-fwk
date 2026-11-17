// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// query.go — the one surface for asking what exists.
//
// There is a single list operation and a single read operation. Both take the
// same `Q`, so choosing "my own contributions" or "the whole universe" is a
// field, not a different function to import. That is the point: the surfaces
// this replaces differed by which package you reached for, and reaching for the
// wrong one returned only the caller's own contributions with no error.
//
// See modules/contributions/concepts.md section 7.

// Reach says WHERE to look.
//
// It is mandatory. The zero value is not a reach, so a call site cannot leave
// it out and be quietly served one topology when it meant the other.
type Reach string

const (
	// ReachSelf is this binary's own contribution providers, read in
	// process. No aggregator, no bus — a standalone binary answers it with
	// nothing else running.
	ReachSelf Reach = "self"

	// ReachUniverse is the aggregated view, read through the aggregation
	// point. What it contains depends on what the aggregator has; that it
	// goes through the aggregator is what the reach guarantees.
	ReachUniverse Reach = "universe"
)

// Include is tri-state because a visibility default depends on the reach it is
// asked at. The zero value means "whatever this reach implies".
type Include int8

const (
	IncludeDefault Include = iota
	IncludeYes
	IncludeNo
)

// resolve applies the reach-dependent default.
func (i Include) resolve(def bool) bool {
	switch i {
	case IncludeYes:
		return true
	case IncludeNo:
		return false
	default:
		return def
	}
}

// Filter says WHAT to include, along the visibility axis. Visibility is a
// property of the contribution (carried by its provider name); reach is
// topology. They are orthogonal — every combination is meaningful.
type Filter struct {
	// Providers, when non-empty, restricts results to these provider names.
	Providers []string

	// IncludeSystem covers the bootstrap metadata every binary embeds
	// identically — kind declarations, well-known data-types. Defaults to
	// true at either reach: a consumer asking for a kind normally wants the
	// system-provided entries of it too.
	IncludeSystem Include

	// IncludeLocal covers binary-private contributions. Defaults to true at
	// ReachSelf and false at ReachUniverse, because a local contribution by
	// definition does not leave its binary — asking the universe for one is
	// asking for something that cannot be there.
	IncludeLocal Include
}

// Q is one question: which reach, which kind, through which aggregate,
// filtered how.
type Q struct {
	// Reach is required.
	Reach Reach

	// Kind is the contribution kind to list ("skill", "task", …). Empty
	// means every kind.
	Kind string

	// Catalog is the aggregation point for ReachUniverse. Nil resolves it
	// from the application in ctx. Ignored at ReachSelf, which never
	// consults an aggregate.
	//
	// A READER: anything that can answer the three read questions can be
	// queried, including a remote one that holds nothing but a Sender.
	Catalog CatalogReader

	Filter Filter
}

// ErrReachUnset is returned for a Q that names no reach.
var ErrReachUnset = errors.New("contributions: Q.Reach is unset — say self or universe")

// Query is the only list surface.
//
// Returns full envelopes. At ReachUniverse on a two-plane index the bodies are
// absent (Body nil, BodyHash set) — use QueryTyped, or Open, to materialise
// them; the index itself never carries them.
func Query(ctx context.Context, q Q) ([]contribapi.Contribution, error) {
	var (
		entries []contribapi.Contribution
		err     error
	)
	switch q.Reach {
	case ReachSelf:
		entries, err = querySelf(ctx, q.Kind)
	case ReachUniverse:
		entries, err = queryUniverse(ctx, q)
	default:
		return nil, ErrReachUnset
	}
	if err != nil {
		return nil, err
	}
	return q.Filter.Apply(entries, q.Reach), nil
}

// QueryTyped is Query plus body decoding, with per-entry provider attribution.
//
// Bodies absent from the index are materialised one at a time through the
// aggregate's content lane (content-store hit, else one bounded fetch). A body
// that fails to decode is an error, not a skipped entry: schema drift that
// silently shortens a list is harder to find than one that stops it.
func QueryTyped[T any](ctx context.Context, q Q) ([]Attributed[T], error) {
	entries, err := Query(ctx, q)
	if err != nil {
		return nil, err
	}
	bodies, err := resolveBodies(ctx, q, entries)
	if err != nil {
		return nil, err
	}
	out := make([]Attributed[T], 0, len(entries))
	for i, c := range entries {
		body := bodies[i]
		// A kind with no data layer (a data-type IS its DSL) decodes to the
		// zero T: it is not an error to ask a typed question of an entry whose
		// content is elsewhere. It used to be one, via a fallback that handed
		// YAML to a JSON decoder.
		var v T
		if len(body) > 0 {
			var derr error
			if v, derr = decodeCached[T](c.DataHash(), body); derr != nil {
				return nil, fmt.Errorf("contributions: decode %s: %w", c.ResolvedURI(), derr)
			}
		}
		out = append(out, Attributed[T]{
			Provider: c.Provider, Body: v,
			URI: c.ResolvedURI(), Meta: c.ResolvedMetadata(),
			Valid: c.Valid, ValidationErrors: c.ValidationErrors,
		})
	}
	return out, nil
}

// GetOne resolves ONE contribution by URI and decodes its body.
//
// The point is what it does NOT do: listing a kind and scanning it to find one
// entry materialises every body of that kind, which is how asking for a single
// skill's instructions came to cost as much as constructing a whole agent.
func GetOne[T any](ctx context.Context, q Q, uri string) (Attributed[T], error) {
	var zero Attributed[T]
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return zero, fmt.Errorf("contributions: %q: %w", uri, err)
	}
	entries, err := oneOrKind(ctx, q, parsed, uri)
	if err != nil {
		return zero, err
	}
	for _, c := range entries {
		if c.ID != parsed.ID {
			continue
		}
		body := c.Data()
		if len(body) == 0 && c.DataHash() != "" && q.Reach == ReachUniverse {
			cat, cerr := q.aggregate(ctx)
			if cerr != nil {
				return zero, cerr
			}
			if body, err = cat.GetBody(ctx, c.ResolvedURI()); err != nil {
				return zero, fmt.Errorf("contributions: resolve body %s: %w", uri, err)
			}
		}
		var v T
		if len(body) > 0 {
			var derr error
			if v, derr = decodeCached[T](c.DataHash(), body); derr != nil {
				return zero, fmt.Errorf("contributions: decode %s: %w", uri, derr)
			}
		}
		return Attributed[T]{
			Provider: c.Provider, Body: v,
			URI: c.ResolvedURI(), Meta: c.ResolvedMetadata(),
			Valid: c.Valid, ValidationErrors: c.ValidationErrors,
		}, nil
	}
	return zero, fmt.Errorf("contributions: %s: %w", uri, ErrNotInView)
}

// Get resolves ONE contribution's envelope by URI, without decoding anything.
//
// The envelope fetch: identity, metadata, layer descriptors and the verdict.
// A consumer that wants to SHOW an entry — the GUI detail pane — asks this and
// then opens the layers it wants. It used to reach for GetOne[json.RawMessage]
// and decode a body it never read, which failed on every kind whose content is
// not JSON.
func Get(ctx context.Context, q Q, uri string) (contribapi.Contribution, error) {
	parsed, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return contribapi.Contribution{}, fmt.Errorf("contributions: %q: %w", uri, err)
	}
	entries, err := oneOrKind(ctx, q, parsed, uri)
	if err != nil {
		return contribapi.Contribution{}, err
	}
	for _, c := range entries {
		if c.ID == parsed.ID {
			return c, nil
		}
	}
	return contribapi.Contribution{}, fmt.Errorf("contributions: %s: %w", uri, ErrNotInView)
}

// ErrNotInView is returned by GetOne and Get when the URI is not in the queried view.
var ErrNotInView = errors.New("not in this view")

// ContributionResolver is implemented by a catalog that can place one URI
// without walking its contents. Optional: a catalog that does not implement it
// is still correct, just linear.
type ContributionResolver interface {
	ContributionAt(uri string) (contribapi.Contribution, bool)
}

// oneOrKind returns just the addressed entry when the aggregate can place it,
// and otherwise falls back to listing the kind and letting the caller pick.
// The fallback is what keeps every Catalog implementation usable; the fast
// path is what keeps asking for one skill from costing a whole kind.
func oneOrKind(ctx context.Context, q Q, parsed contribapi.ResourceURI, uri string) ([]contribapi.Contribution, error) {
	if q.Reach == ReachUniverse {
		cat, err := q.aggregate(ctx)
		if err != nil {
			return nil, err
		}
		if r, ok := cat.(ContributionResolver); ok {
			c, found := r.ContributionAt(uri)
			if !found {
				return nil, nil
			}
			return q.Filter.Apply([]contribapi.Contribution{c}, q.Reach), nil
		}
	}
	q.Kind = parsed.Type
	q.Filter.Providers = []string{parsed.Provider}
	return Query(ctx, q)
}

// ── reaches ─────────────────────────────────────────────────────────────────

// querySelf reads the binary's own view — ResourceService, resolved in
// process. No aggregate, no bus: this is the reach a standalone binary can
// always answer.
//
// It goes through the service rather than walking the raw providers so the
// self reach and the wire surface are the same view: deduped, ordered, with
// each entry's schema resolved and its body hashed. A second provider walk
// would answer the same question with weaker envelopes, and the two would
// drift.
//
// Visibility is asked for wide open here and narrowed by Filter.Apply, so the
// defaults live in exactly one place.
func querySelf(ctx context.Context, kind string) ([]contribapi.Contribution, error) {
	svc, err := application.UseService[ResourceService](ctx)
	if err != nil {
		return nil, fmt.Errorf("contributions: self reach needs this binary's ResourceService: %w", err)
	}
	yes := true
	in := ContributionListInput{WithBody: true, IncludeLocal: true, IncludeSystem: &yes}
	if kind != "" {
		in.Filter = ContributionListFilter{Types: []string{kind}}
	}
	out, err := svc.List(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("contributions: list self: %w", err)
	}
	return out.Contributions, nil
}

// queryUniverse goes through the aggregation point. A consumer cannot reach
// this path and be served only its own providers: the aggregate is a different
// object from the provider list, and which one answers is decided by the reach
// the caller named, never inferred.
func queryUniverse(ctx context.Context, q Q) ([]contribapi.Contribution, error) {
	cat, err := q.aggregate(ctx)
	if err != nil {
		return nil, err
	}
	if q.Kind != "" {
		return cat.GetContributionsByType(ctx, q.Kind)
	}
	manifests, err := cat.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []contribapi.Contribution
	for _, m := range manifests {
		if m != nil {
			out = append(out, m.Contributions...)
		}
	}
	return out, nil
}

// selfResourceService resolves this binary's own contribution surface.
func selfResourceService(ctx context.Context) (ResourceService, error) {
	svc, err := application.UseService[ResourceService](ctx)
	if err != nil {
		return nil, fmt.Errorf("contributions: self reach needs this binary's ResourceService: %w", err)
	}
	return svc, nil
}

// newBytesReadCloser wraps bytes as a reader that needs no closing.
func newBytesReadCloser(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}

// aggregate returns the explicit reader, else resolves one from ctx.
//
// Returns a CatalogReader, not a Catalog: reading needs three methods and the
// rest of Catalog is the aggregator's write protocol. Narrowing here is what
// lets a light consumer — one holding a Sender rather than an in-process
// catalog — be queried by the same surface as everything else.
func (q Q) aggregate(ctx context.Context) (CatalogReader, error) {
	if q.Catalog != nil {
		return q.Catalog, nil
	}
	cat, err := application.UseService[Catalog](ctx)
	if err != nil {
		return nil, fmt.Errorf("contributions: universe reach needs an aggregate catalog: %w", err)
	}
	return cat, nil
}

// ── visibility ──────────────────────────────────────────────────────────────

// Apply narrows entries along the visibility axis. Exported so the wire
// surface (ResourceService.List) decides visibility here too, rather than
// keeping a second copy of the same two rules.
func (f Filter) Apply(entries []contribapi.Contribution, reach Reach) []contribapi.Contribution {
	system := f.IncludeSystem.resolve(true)
	local := f.IncludeLocal.resolve(reach == ReachSelf)

	var allow map[string]bool
	if len(f.Providers) > 0 {
		allow = make(map[string]bool, len(f.Providers))
		for _, p := range f.Providers {
			allow[p] = true
		}
	}
	if system && local && allow == nil {
		return entries
	}

	out := make([]contribapi.Contribution, 0, len(entries))
	for _, c := range entries {
		switch {
		case !system && c.Provider == contribapi.SystemProvider:
		case !local && !contribapi.IsBusVisible(c.Provider):
		case allow != nil && !allow[c.Provider]:
		default:
			out = append(out, c)
		}
	}
	return out
}

// KindSchemas returns the data-type each kind declares, keyed by kind name.
//
// One query for every kind, rather than a lookup per contribution: kinds are
// few and contributions are many, so the shape that scales is the one that asks
// once. A consumer showing a schema column for a thousand entries makes one
// call, not a thousand.
//
// This is where a contribution's schema comes from now. It used to be a field
// on the envelope, written by the producer and re-written by the aggregator —
// a value with two writers, which is a contradiction rather than a contract. A
// kind declares its schema; an entry of that kind has it by being that kind.
func KindSchemas(ctx context.Context, q Q) (map[string]string, error) {
	q.Kind = contribapi.ResourceTypeContributionType
	entries, err := Query(ctx, q)
	if err != nil {
		return nil, err
	}
	return contribapi.KindsFrom(entries).Schemas(), nil
}

// KindsAt returns the kind registry of a view — every declaration it holds,
// resolved once. The one call behind KindSchemas, KindOf and the default layer
// of Open; a consumer with several questions about kinds asks this once.
// KindsHolder is the optional side-interface of a catalog that already holds
// the kind registry (the hub's aggregate builds it once per reload); KindsAt
// prefers it to re-deriving the registry from a query on every call.
type KindsHolder interface {
	Kinds() contribapi.Kinds
}

func KindsAt(ctx context.Context, q Q) (contribapi.Kinds, error) {
	if q.Reach == ReachUniverse {
		if h, ok := q.Catalog.(KindsHolder); ok {
			return h.Kinds(), nil
		}
	}
	q.Kind = contribapi.ResourceTypeContributionType
	entries, err := Query(ctx, q)
	if err != nil {
		return contribapi.Kinds{}, err
	}
	return contribapi.KindsFrom(entries), nil
}

// SchemaFor resolves the data-type governing a contribution, from the kind
// declarations in `kinds`.
//
// The meta-model is the base case: its kinds cannot name their own schema
// without circularity — a `contribution-type` describing `contribution-type` —
// so they resolve to the system data-type of the same name, which is the point
// at which the chain terminates.
func SchemaFor(kinds map[string]string, kind string) string {
	if s := kinds[kind]; s != "" {
		return s
	}
	if contribapi.IsMetaKind(kind) {
		return contribapi.Contribution{
			Provider: contribapi.SystemProvider,
			Type:     contribapi.ResourceTypeDataType,
			ID:       kind,
		}.ResolvedURI()
	}
	return ""
}

// bodyFetchConcurrency bounds how many bodies are materialised at once.
//
// MEASURED, not guessed: the GUI's task gallery reads 63 bodies, and doing it
// one at a time cost four seconds of round trips — the panel's whole latency.
// Eight is enough to collapse that to a handful of batches while staying well
// inside what one aggregator answers comfortably; unbounded would turn a large
// universe into a thundering herd against a single service.
const bodyFetchConcurrency = 8

// resolveBodies returns one body per entry, in entry order, fetching the
// absent ones CONCURRENTLY.
//
// Order is preserved by writing into a pre-sized slice rather than appending,
// so the result does not depend on which fetch finished first — a listing that
// reorders between runs is a listing nobody can diff.
func resolveBodies(ctx context.Context, q Q, entries []contribapi.Contribution) ([][]byte, error) {
	bodies := make([][]byte, len(entries))
	var missing []int
	for i, c := range entries {
		bodies[i] = c.Data()
		if len(bodies[i]) == 0 && c.DataHash() != "" && q.Reach == ReachUniverse {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		return bodies, nil
	}
	cat, err := q.aggregate(ctx)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, bodyFetchConcurrency)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	for _, idx := range missing {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if ctx.Err() != nil {
				return
			}
			fetched, ferr := cat.GetBody(ctx, entries[i].ResolvedURI())
			if ferr != nil {
				// THE FIRST ERROR WINS and stops the rest. A body that fails
				// to resolve is an error, not a skipped entry — a list that
				// silently shortens is harder to find than one that stops.
				once.Do(func() {
					firstErr = fmt.Errorf("contributions: resolve body %s: %w", entries[i].ResolvedURI(), ferr)
					cancel()
				})
				return
			}
			bodies[i] = fetched
		}(idx)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return bodies, nil
}
