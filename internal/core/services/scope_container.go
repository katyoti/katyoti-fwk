// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
)

// ScopeContainer is a per-scope service cache. Each entry holds a
// `sync.Once` so the factory runs at most once, OUTSIDE the container
// mutex — re-entrant factory calls (factory A's body resolving service
// B in the same scope) never deadlock because the container mutex is
// only held for the brief registry lookup.
//
// Phase E §5a refactor (2026-06-05): the prior shape held the
// container mutex around the factory call, which forced all
// in-process dependency injection to thread a "re-entrant resolver"
// through `context.Context` to escape the lock. Per-entry once removes
// that need — `application.UseService[T](ctx)` works uniformly during
// factory execution and outside it.
type ScopeContainer struct {
	scope   Scope
	mu      sync.Mutex
	entries map[string]*entry
	closed  bool
	// closing is set once Close has begun (draining): a second Close is a
	// no-op, while resolutions still work until closed is set.
	closing bool
	// next numbers entries in the order their value became available
	// (Put, or a factory returning). A factory that resolves a dependency
	// returns AFTER it, so the order is a dependency order; Close walks it
	// backwards.
	next uint64
}

// entry holds one cached service's result + its `sync.Once`. The Once
// guarantees the factory runs exactly once per entry across all
// goroutines. value+err are written inside Once.Do and read after it
// returns; sync.Once provides the necessary happens-before.
type entry struct {
	once  sync.Once
	value any
	err   error
	// seq is the entry's place in the availability order (0 = not yet
	// available). Written and read under the container mutex.
	seq uint64
}

// NewScopeContainer creates a container for the given scope.
func NewScopeContainer(scope Scope) *ScopeContainer {
	return &ScopeContainer{
		scope:   scope,
		entries: make(map[string]*entry),
	}
}

// Scope returns the scope this container belongs to.
func (sc *ScopeContainer) Scope() Scope { return sc.scope }

// Get returns a cached, fully-resolved service, or (nil, false).
// Returns (nil, false) for entries whose factory hasn't run yet OR
// whose factory returned an error — callers that need the
// once-protected create path use GetOrCreate.
func (sc *ScopeContainer) Get(id string) (any, bool) {
	sc.mu.Lock()
	e, ok := sc.entries[id]
	sc.mu.Unlock()
	if !ok || e == nil {
		return nil, false
	}
	// Lock-free read: callers that race a still-running factory get
	// the (zero, false) view rather than blocking. The proper path
	// for callers that NEED the result is GetOrCreate, which waits.
	if e.err != nil {
		return nil, false
	}
	return e.value, e.value != nil
}

// Put stores a pre-resolved service. Used for bootstrap services
// (Application, ConfigService, etc.) that exist before any factory
// has run. Idempotent — overwrites any existing entry's value (a
// later GetOrCreate caller for the same id sees the put value).
func (sc *ScopeContainer) Put(id string, svc any) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	e := sc.entries[id]
	if e == nil {
		e = &entry{}
		sc.entries[id] = e
	}
	// Mark the once as fired so subsequent GetOrCreate calls just
	// return the put value rather than running an alternative
	// factory. The Once never fires its function — the closure is
	// the no-op `func() {}`.
	e.once.Do(func() {})
	e.value = svc
	e.err = nil
	sc.next++
	e.seq = sc.next
}

// GetOrCreate returns a cached service or creates it atomically.
// The factory runs EXACTLY ONCE per (container, id) across all
// goroutines — protected by the per-entry sync.Once, NOT the
// container mutex. This means:
//
//   - Concurrent GetOrCreate calls for the SAME id share the result.
//   - Re-entrant GetOrCreate calls for DIFFERENT ids never deadlock
//     (each id has its own once).
//   - Re-entrant GetOrCreate calls for the SAME id from the SAME
//     goroutine WOULD deadlock (sync.Once.Do waits for the in-flight
//     Do); circular-dependency detection must catch this case before
//     reaching GetOrCreate (ServiceManager.GetByName does that via
//     a context-carried resolution stack).
func (sc *ScopeContainer) GetOrCreate(id string, factory func() (any, error)) (any, error) {
	sc.mu.Lock()
	if sc.closed {
		sc.mu.Unlock()
		return nil, sc.closedErr()
	}
	e := sc.entries[id]
	if e == nil {
		e = &entry{}
		sc.entries[id] = e
	}
	sc.mu.Unlock()

	e.once.Do(func() {
		value, err := factory()
		sc.mu.Lock()
		if sc.closed {
			// The container closed while this factory ran: Close did not
			// see the service, so it is disposed here, and nobody gets it.
			sc.mu.Unlock()
			if c, ok := value.(io.Closer); ok && err == nil && value != nil {
				_ = c.Close()
			}
			e.value, e.err = nil, sc.closedErr()
			return
		}
		e.value, e.err = value, err
		if err == nil {
			sc.next++
			e.seq = sc.next
		}
		sc.mu.Unlock()
	})
	return e.value, e.err
}

func (sc *ScopeContainer) closedErr() error {
	return fmt.Errorf("scope %q is closed", sc.scope.Name())
}

// Drainer is a service owning background work that must FINISH before the
// container closes — work that may itself resolve services from it (an event
// observer recording what a one-shot command just wrote). Close calls Drain
// on every Drainer first, with the container still open; Drain returns once
// the work is done (bounded by the service itself) and must take no new work.
type Drainer interface {
	Drain(ctx context.Context)
}

// Close disposes the container in two phases.
//
//  1. DRAIN — every cached Drainer is drained, in the reverse of the order the
//     services became available, while the container is still OPEN: the work
//     they finish can resolve services as usual.
//  2. CLOSE — the container is marked closed and its entries taken under the
//     mutex; then every io.Closer is closed WITHOUT the mutex, in the reverse
//     order of availability (a service before the services it was built
//     from). A resolution during this phase gets a prompt "closed" error —
//     never blocks on the mutex: a service's Close may wait on a goroutine
//     that is resolving a service (the CWM service stopping an event observer
//     whose handler calls application.UseService), which deadlocked until the
//     observer stop wait expired while Close held the mutex.
//
// After Close, GetOrCreate returns an error. A second Close is a no-op.
func (sc *ScopeContainer) Close() error {
	sc.mu.Lock()
	if sc.closed || sc.closing {
		sc.mu.Unlock()
		return nil
	}
	sc.closing = true
	drainers := sc.availableLocked()
	sc.mu.Unlock()
	for _, e := range drainers {
		if d, ok := e.value.(Drainer); ok {
			d.Drain(context.Background())
		}
	}

	sc.mu.Lock()
	sc.closed = true
	closers := sc.availableLocked()
	sc.entries = nil
	sc.mu.Unlock()
	var firstErr error
	for _, e := range closers {
		if c, ok := e.value.(io.Closer); ok {
			if err := c.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// availableLocked returns the entries holding a service, latest available
// first. The caller holds sc.mu.
func (sc *ScopeContainer) availableLocked() []*entry {
	out := make([]*entry, 0, len(sc.entries))
	for _, e := range sc.entries {
		if e == nil || e.seq == 0 || e.err != nil || e.value == nil {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].seq > out[j].seq })
	return out
}

// Reset clears the cache without closing services. Used by pooled
// scope implementations to recycle containers.
func (sc *ScopeContainer) Reset() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.entries = make(map[string]*entry)
	sc.closed = false
	sc.closing = false
}
