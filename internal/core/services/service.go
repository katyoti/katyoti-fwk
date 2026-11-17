// SPDX-License-Identifier: Apache-2.0

// Package services provides service dependency injection with scoped caching.
//
// # Creating registrations
//
//	reg := services.ConfiguredService(myext.NewGreeter)          // config + ctx-based deps (ApplicationScope)
//	reg := services.SimpleService(myext.NewMetrics)              // no config (ApplicationScope)
//	reg := services.ScopedService(services.TaskScope, myext.New) // task-scoped with config
//	sm.RegisterService(reg)
//
// # Consuming services
//
//	svc, err := services.Get[Greeter](sm, ctx, "default")
//
// # Bootstrap services
//
//	services.Put[*config.ConfigService](sm, "default", cfgSvc)
package services

import (
	"context"
	"reflect"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
)

// ServiceRegistration is the factory interface for a service class.
// Each call to NewService() returns a fresh config pointer (if needed) and
// a create function that receives context for dependency resolution.
//
// Create via ConfiguredService, SimpleService, ScopedService, or ScopedSimpleService.
type ServiceRegistration interface {
	// Name returns the service class name (derived from the Go type name).
	Name() string
	// Scope returns the lifecycle scope for instances of this service.
	Scope() Scope
	// NewService returns a config pointer (nil if no config) and a create function.
	// The create function receives context.Context for resolving deps via UseService[T](ctx).
	NewService() (configPtr any, createFn func(ctx context.Context) (any, error))
}

// isEmptyStruct returns true if ptr points to a struct{} (no fields).
func isEmptyStruct(ptr any) bool {
	if ptr == nil {
		return true
	}
	t := reflect.TypeOf(ptr)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && t.NumField() == 0
}

// --- ServiceRegistration constructors ──────────────────────────────────

// ConfiguredService creates a ServiceRegistration with config (ApplicationScope).
// The create function receives context for resolving dependencies via UseService[T](ctx).
func ConfiguredService[TService, TConfig any](create func(ctx context.Context, cfg *TConfig) (TService, error)) ServiceRegistration {
	return &configuredService[TService, TConfig]{
		name:   config.TypeName[TService](),
		scope:  ApplicationScope,
		create: create,
	}
}

// SimpleService creates a ServiceRegistration with no config (ApplicationScope).
// The create function receives context for resolving dependencies via UseService[T](ctx).
func SimpleService[TService any](create func(ctx context.Context) (TService, error)) ServiceRegistration {
	return &simpleService[TService]{
		name:   config.TypeName[TService](),
		scope:  ApplicationScope,
		create: create,
	}
}

// ScopedService creates a ServiceRegistration with config bound to a specific scope.
func ScopedService[TService, TConfig any](scope Scope, create func(ctx context.Context, cfg *TConfig) (TService, error)) ServiceRegistration {
	return &configuredService[TService, TConfig]{
		name:   config.TypeName[TService](),
		scope:  scope,
		create: create,
	}
}

// ScopedSimpleService creates a ServiceRegistration with no config bound to a specific scope.
func ScopedSimpleService[TService any](scope Scope, create func(ctx context.Context) (TService, error)) ServiceRegistration {
	return &simpleService[TService]{
		name:   config.TypeName[TService](),
		scope:  scope,
		create: create,
	}
}

// --- Type-erased adapters ──────────────────────────────────────────────

type configuredService[TService, TConfig any] struct {
	name   string
	scope  Scope
	create func(ctx context.Context, cfg *TConfig) (TService, error)
}

func (s *configuredService[TService, TConfig]) Name() string { return s.name }
func (s *configuredService[TService, TConfig]) Scope() Scope {
	if s.scope != nil {
		return s.scope
	}
	return ApplicationScope
}

func (s *configuredService[TService, TConfig]) NewService() (any, func(context.Context) (any, error)) {
	cfg := new(TConfig)
	var cfgPtr any
	if !isEmptyStruct(cfg) {
		cfgPtr = cfg
	}
	return cfgPtr, func(ctx context.Context) (any, error) {
		return s.create(ctx, cfg)
	}
}

type simpleService[TService any] struct {
	name   string
	scope  Scope
	create func(ctx context.Context) (TService, error)
}

func (s *simpleService[TService]) Name() string { return s.name }
func (s *simpleService[TService]) Scope() Scope {
	if s.scope != nil {
		return s.scope
	}
	return ApplicationScope
}

func (s *simpleService[TService]) NewService() (any, func(context.Context) (any, error)) {
	return nil, func(ctx context.Context) (any, error) {
		return s.create(ctx)
	}
}
