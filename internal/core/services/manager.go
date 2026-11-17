// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"fmt"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
)

// ServiceManager resolves, creates, and caches services across scopes.
//
// Phase E §5a refactor (2026-06-05): the prior shape held one
// process-wide mutex around `resolve` (factory execution included),
// which forced recursive lookups to escape via a context-carried
// `resolverFunc`. The new shape:
//
//   - `regMu` is an `RWMutex` and protects ONLY the factories map.
//     It is held briefly for register/lookup, NEVER during factory
//     execution.
//   - Per-service serialization moves into ScopeContainer's per-entry
//     `sync.Once` (see ScopeContainer.GetOrCreate). Concurrent
//     resolution of the SAME service is still single-flighted; nothing
//     else is contended.
//   - Circular-dependency detection moves from a shared `stack` map
//     onto the `context.Context` carrying the resolution chain — each
//     goroutine sees only its own stack, no cross-goroutine
//     interference.
//
// Net effect: `application.UseService[T](ctx)` works uniformly during
// factory execution and outside it. No more `resolverFunc` indirection.
type ServiceManager struct {
	cfgSvc    *config.ConfigService
	regMu     sync.RWMutex
	factories map[string]ServiceRegistration // className → factory
	app       *ScopeContainer                // this manager's ApplicationScope container
}

// NewServiceManager creates the root ServiceManager with its own
// ApplicationScope container, and makes that container the scope's fallback
// for contexts that carry no application.
func NewServiceManager(cfgSvc *config.ConfigService) *ServiceManager {
	container := NewScopeContainer(ApplicationScope)
	ApplicationScope.(*applicationScope).adopt(container)
	return &ServiceManager{
		cfgSvc:    cfgSvc,
		factories: make(map[string]ServiceRegistration),
		app:       container,
	}
}

// Container returns this manager's ApplicationScope container — the one its
// singletons live in. Stamp it on a context with WithApplicationContainer.
func (sm *ServiceManager) Container() *ScopeContainer { return sm.app }

// containerFor resolves the container a scope's services live in, for this
// manager: its own for ApplicationScope, the context's for every other.
func (sm *ServiceManager) containerFor(scope Scope, ctx context.Context) *ScopeContainer {
	if scope == ApplicationScope {
		return sm.app
	}
	return scope.ContainerFromContext(ctx)
}

// RegisterService registers a ServiceRegistration with the manager.
// Holds the registry write lock briefly; never blocks factory execution.
func (sm *ServiceManager) RegisterService(reg ServiceRegistration) {
	sm.regMu.Lock()
	defer sm.regMu.Unlock()
	sm.factories[reg.Name()] = reg
}

// factoryFor returns the registered factory for className, or nil. Read-only.
func (sm *ServiceManager) factoryFor(className string) ServiceRegistration {
	sm.regMu.RLock()
	defer sm.regMu.RUnlock()
	return sm.factories[className]
}

// Put manually places a service instance into the ApplicationScope cache.
// Used for bootstrap services (ConfigService, EM, Application)
// that are created before extensions run.
func Put[T any](sm *ServiceManager, instanceName string, svc T) {
	className := config.TypeName[T]()
	id := config.ServiceName(className, instanceName)
	sm.app.Put(id, svc)
}

// GetByName resolves a service by class name and instance name.
// Non-generic version for dynamic/reflection-based resolution.
//
// Safe to call recursively: the factory for service X may resolve
// service Y via UseService inside its body; the second call into
// GetByName/Get for Y sees no held lock and proceeds normally.
// True circular dependencies (X depends on Y depends on X) are
// detected via the resolution stack carried in `ctx`.
func (sm *ServiceManager) GetByName(ctx context.Context, className, instanceName string) (any, error) {
	id := config.ServiceName(className, instanceName)

	// Per-goroutine circular dep check via ctx stack.
	if stackContains(ctx, id) {
		return nil, ErrCircularDependency{ID: id, Stack: stackSlice(ctx)}
	}

	factory := sm.factoryFor(className)
	if factory == nil {
		// No factory — check ApplicationScope cache for bootstrap
		// services (Application, ConfigService, EM are Put-ed there
		// before any extension runs).
		if svc, found := sm.app.Get(id); found {
			return svc, nil
		}
		return nil, ErrServiceNotFound{ID: id}
	}

	// Find the container for this service's scope.
	scope := factory.Scope()
	container := sm.containerFor(scope, ctx)
	if container == nil {
		return nil, fmt.Errorf("services: service %q requires %s scope (not in context)", id, scope.Name())
	}

	// Run the factory under the entry's sync.Once. No SM lock held. The child
	// context names this manager's container, so a factory resolving its
	// dependencies never reaches another manager's singletons.
	childCtx := WithApplicationContainer(ctxWithStack(ctx, id), sm.app)
	return container.GetOrCreate(id, func() (any, error) {
		cfgPtr, createFn := factory.NewService()
		if cfgPtr != nil && !isEmptyStruct(cfgPtr) {
			if err := sm.fillConfig(className, instanceName, cfgPtr); err != nil {
				return nil, fmt.Errorf("services: config for %s: %w", id, err)
			}
		}
		svc, err := createFn(childCtx)
		if err != nil {
			return nil, fmt.Errorf("services: create %s: %w", id, err)
		}
		return svc, nil
	})
}

// Get resolves a service by T's type name and instanceName.
// Thread-safe. Lazy creation on first call. Cached in the service's declared scope container.
func Get[T any](sm *ServiceManager, ctx context.Context, instanceName string) (T, error) {
	className := config.TypeName[T]()
	raw, err := sm.GetByName(ctx, className, instanceName)
	if err != nil {
		var zero T
		return zero, err
	}
	svc, ok := raw.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("services: %s is %T, not %s",
			config.ServiceName(className, instanceName), raw, className)
	}
	return svc, nil
}

// fillConfig resolves the 4-layer config merge and unmarshals into cfgPtr.
// Uses config.DecodeMap for variable expansion + ProcessConfig (envkey, default, required).
// ProcessConfig always runs, even when no config file exists, so envkey/default/required
// tags are resolved on zero-value structs.
func (sm *ServiceManager) fillConfig(className, instanceName string, cfgPtr any) error {
	merged, err := sm.cfgSvc.ResolveServiceConfig(className, instanceName)
	if err != nil {
		return err
	}
	if len(merged) > 0 {
		return config.DecodeMap(merged, cfgPtr)
	}
	// No config found — still run ProcessConfig so envkey/default/required tags fire.
	return config.ProcessConfig(cfgPtr)
}

// --- Resolution stack (per-goroutine, carried in ctx) -------------------

type stackKey struct{}

// stackContains reports whether id is on the resolution chain leading to ctx.
func stackContains(ctx context.Context, id string) bool {
	st, _ := ctx.Value(stackKey{}).([]string)
	for _, x := range st {
		if x == id {
			return true
		}
	}
	return false
}

// stackSlice returns a copy of the current resolution stack for error messages.
func stackSlice(ctx context.Context) []string {
	st, _ := ctx.Value(stackKey{}).([]string)
	out := make([]string, len(st))
	copy(out, st)
	return out
}

// ContextWithoutResolutionStack returns ctx with the service-resolution stack
// cleared, preserving every other context value. Use it when a service factory
// captures its resolution context for LATER, INDEPENDENT resolutions — e.g. a
// background runner that stores the factory ctx and uses it as the base for
// child task runs. Without clearing, the captured factory-scoped stack would
// make those later resolves of the same service (or any service still on the
// factory stack) falsely report a circular dependency.
func ContextWithoutResolutionStack(ctx context.Context) context.Context {
	return context.WithValue(ctx, stackKey{}, []string(nil))
}

// ctxWithStack pushes id onto the resolution stack for descendant resolves.
// Uses a tight slice (cap == len) so sibling pushes can't collide via shared
// backing storage.
func ctxWithStack(ctx context.Context, id string) context.Context {
	prev, _ := ctx.Value(stackKey{}).([]string)
	next := make([]string, len(prev)+1)
	copy(next, prev)
	next[len(prev)] = id
	return context.WithValue(ctx, stackKey{}, next)
}

// --- Error types -------------------------------------------------------

// ErrServiceNotFound is returned when no factory is registered for the given service ID.
type ErrServiceNotFound struct {
	ID string
}

func (e ErrServiceNotFound) Error() string {
	return fmt.Sprintf("services: service %q not found", e.ID)
}

// ErrCircularDependency is returned when a circular dependency is detected.
type ErrCircularDependency struct {
	ID    string
	Stack []string
}

func (e ErrCircularDependency) Error() string {
	return fmt.Sprintf("services: circular dependency detected resolving %q (stack: %v)", e.ID, e.Stack)
}
