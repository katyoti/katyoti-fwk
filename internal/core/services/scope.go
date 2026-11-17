// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"sync"
)

// Scope defines a lifecycle boundary for service instances.
// Each scope implementation controls how containers are found, created, and disposed.
// Scopes are independent dimensions — no hierarchy enforced.
type Scope interface {
	// Name returns a human-readable scope name (for error messages).
	Name() string

	// ContainerFromContext finds the active container for this scope in ctx, or nil.
	ContainerFromContext(ctx context.Context) *ScopeContainer

	// EnterScope creates a new scope instance and attaches its container to ctx.
	// Use for long-lived scopes (e.g., WebSocket sessions) where enter/exit don't
	// align with a single function call. The caller must call ScopeContainer.Close().
	EnterScope(ctx context.Context) (context.Context, *ScopeContainer)

	// RunInScope creates a new scope instance, runs fn, then disposes.
	// Bracket pattern — sugar over EnterScope + defer Close().
	RunInScope(ctx context.Context, fn func(ctx context.Context) error) error
}

// Predefined scopes. ApplicationScope is ONE value for the process — the
// identity a registration declares — and is never reassigned; the container
// it resolves to belongs to a ServiceManager (see applicationScope).
// SessionScope and TaskScope are transient — each EnterScope/RunInScope creates a new container.
var (
	ApplicationScope Scope = &applicationScope{}
	SessionScope     Scope = NewTransientScope("session")
	TaskScope        Scope = NewTransientScope("task")
)

// --- applicationScope ---

// applicationScope resolves to the container of the ServiceManager the
// context belongs to (WithApplicationContainer, stamped by the manager on
// every resolution and by application.WithApplication), and for a context
// that carries none, to the most recently created manager's container.
//
// It used to hold ONE container and be reassigned by every NewServiceManager,
// which made the scope process-global: a second Application in the process —
// a parallel test, the census, a GUI universe core — pointed it at its own
// container and every other Application lost its singletons ("service not
// found" on a service it had just Put).
type applicationScope struct {
	mu       sync.RWMutex
	fallback *ScopeContainer
}

type appContainerKey struct{}

// WithApplicationContainer attaches a ServiceManager's application container
// to ctx, so ApplicationScope resolves to it from that context on.
func WithApplicationContainer(ctx context.Context, c *ScopeContainer) context.Context {
	return context.WithValue(ctx, appContainerKey{}, c)
}

// adopt makes c the fallback for contexts that carry no application.
func (s *applicationScope) adopt(c *ScopeContainer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fallback = c
}

func (s *applicationScope) Name() string { return "application" }

func (s *applicationScope) ContainerFromContext(ctx context.Context) *ScopeContainer {
	if ctx != nil {
		if c, ok := ctx.Value(appContainerKey{}).(*ScopeContainer); ok && c != nil {
			return c
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.fallback
}

func (s *applicationScope) EnterScope(ctx context.Context) (context.Context, *ScopeContainer) {
	// One container per manager already exists; make sure ctx names it.
	c := s.ContainerFromContext(ctx)
	return WithApplicationContainer(ctx, c), c
}

func (s *applicationScope) RunInScope(ctx context.Context, fn func(ctx context.Context) error) error {
	// Singleton — no lifecycle to manage, just run.
	return fn(ctx)
}

// --- transientScope ---

// transientScope creates a new ScopeContainer on each EnterScope/RunInScope.
// Uses itself as the context key — each scope instance is unique.
type transientScope struct {
	name string
}

// NewTransientScope creates a scope that produces a new container on each entry.
// Use for custom scopes beyond the predefined Session/Task.
func NewTransientScope(name string) Scope {
	return &transientScope{name: name}
}

func (s *transientScope) Name() string { return s.name }

func (s *transientScope) ContainerFromContext(ctx context.Context) *ScopeContainer {
	sc, _ := ctx.Value(s).(*ScopeContainer)
	return sc
}

func (s *transientScope) EnterScope(ctx context.Context) (context.Context, *ScopeContainer) {
	sc := NewScopeContainer(s)
	return context.WithValue(ctx, s, sc), sc
}

func (s *transientScope) RunInScope(ctx context.Context, fn func(ctx context.Context) error) error {
	ctx, sc := s.EnterScope(ctx)
	defer sc.Close()
	return fn(ctx)
}
