// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TypeDefStrategy wraps a TypeDef[PluginFactory] as a PluginStrategy.
// This bridges the domain system (StrategyDef → TypeDef) with the 2-phase resolver
// (PluginStrategy with GetRequirements + ResolveAll).
//
// Default behavior: GetRequirements returns nil (no deps). ResolveAll returns
// the factory from the TypeDef. Strategies that need custom GetRequirements
// (like skill) can use WithGetRequirements to override.
type TypeDefStrategy struct {
	name            string
	typeDef         strategies.TypeDef[api.PluginFactory]
	getRequirements func(ref api.PluginRef) ([]api.PluginRef, error)
}

// WrapTypeDef creates a PluginStrategy from a TypeDef.
func WrapTypeDef(td strategies.TypeDef[api.PluginFactory]) *TypeDefStrategy {
	return &TypeDefStrategy{
		name:    td.Name,
		typeDef: td,
	}
}

// WithGetRequirements sets a custom GetRequirements function.
func (s *TypeDefStrategy) WithGetRequirements(fn func(ref api.PluginRef) ([]api.PluginRef, error)) *TypeDefStrategy {
	s.getRequirements = fn
	return s
}

func (s *TypeDefStrategy) Name() string { return s.name }

// GetRequirements returns additional refs required by this ref.
// Default: nil (no deps). Override via WithGetRequirements for skills.
func (s *TypeDefStrategy) GetRequirements(ref api.PluginRef) ([]api.PluginRef, error) {
	if s.getRequirements != nil {
		return s.getRequirements(ref)
	}
	return nil, nil
}

// ResolveAll returns the factory from the TypeDef.
// The factory's CreatePlugins will be called with all refs of this type.
func (s *TypeDefStrategy) ResolveAll(refs []api.PluginRef) ([]api.PluginFactory, error) {
	if s.typeDef.Impl == nil {
		return nil, nil
	}
	return []api.PluginFactory{s.typeDef.Impl}, nil
}

// typeLookup is the slice of the plugin domain this package reads: the names it
// holds, and the TypeDef behind each one.
//
// A consumer-defined interface rather than *em.IndexedPoint, so the domain
// layer stops naming the extension manager for a parameter's TYPE. Nothing here
// contributes to a point or registers one — it only reads what is already
// there, and that is expressible without the framework.
//
// *em.IndexedPoint satisfies it structurally, so the call sites are unchanged.
type typeLookup interface {
	Keys() []string
	Has(name string) bool
	Get(name string) strategies.TypeDef[api.PluginFactory]
}

// BuildStrategiesFromDomain wraps all TypeDefs in the domain's type point as
// PluginStrategy instances. Returns them for use with NewPluginResolver.
func BuildStrategiesFromDomain(types typeLookup) []api.PluginStrategy {
	var result []api.PluginStrategy
	for _, name := range types.Keys() {
		td := types.Get(name)
		result = append(result, WrapTypeDef(td))
	}
	return result
}
