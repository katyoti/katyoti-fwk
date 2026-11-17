// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// PluginResolver resolves a list of PluginRef references into instantiated AgentPlugins.
// Two-phase resolution:
//  1. Collect requirements: expand each ref's dependencies until stable
//  2. Batch resolve: group refs by strategy, call ResolveAll, create plugins
type PluginResolver struct {
	strategies map[string]api.PluginStrategy
	fallback   func(name string) (api.PluginFactory, bool)
}

// NewPluginResolver creates a resolver with registered strategies and a fallback for legacy names.
func NewPluginResolver(strategies []api.PluginStrategy, fallback func(string) (api.PluginFactory, bool)) *PluginResolver {
	m := make(map[string]api.PluginStrategy, len(strategies))
	for _, s := range strategies {
		m[s.Name()] = s
	}
	return &PluginResolver{strategies: m, fallback: fallback}
}

// Resolve resolves plugin refs into instantiated plugins.
// Phase 1: collect all requirements recursively.
// Phase 2: group by strategy, call ResolveAll, create plugins.
func (r *PluginResolver) Resolve(
	refs []api.PluginRef,
	ctx context.Context,
	loop api.LoopContext,
) ([]api.AgentPlugin, error) {

	// Phase 1: collect all requirements
	allRefs, err := r.collectRequirements(refs)
	if err != nil {
		return nil, err
	}

	// Phase 2: group by strategy type
	grouped := make(map[string][]api.PluginRef)
	var fallbackRefs []api.PluginRef
	var order []string // preserve first-seen order of strategy types

	for _, ref := range allRefs {
		if _, ok := r.strategies[ref.Type]; ok {
			if _, seen := grouped[ref.Type]; !seen {
				order = append(order, ref.Type)
			}
			grouped[ref.Type] = append(grouped[ref.Type], ref)
		} else {
			fallbackRefs = append(fallbackRefs, ref)
		}
	}

	// Phase 2b: resolve each strategy group
	var all []api.AgentPlugin

	for _, stratName := range order {
		strategy := r.strategies[stratName]
		factories, err := strategy.ResolveAll(grouped[stratName])
		if err != nil {
			return nil, fmt.Errorf("resolve strategy %q: %w", stratName, err)
		}
		for _, factory := range factories {
			plugins, err := factory.CreatePlugins(ctx, loop, grouped[stratName])
			if err != nil {
				return nil, fmt.Errorf("create plugins for %q: %w", factory.Name(), err)
			}
			all = append(all, plugins...)
		}
	}

	// Phase 2c: fallback refs (legacy name-based lookup)
	for _, ref := range fallbackRefs {
		factory := r.resolveFallback(ref)
		if factory == nil {
			return nil, fmt.Errorf("unknown plugin strategy %q", ref.Type)
		}
		plugins, err := factory.CreatePlugins(ctx, loop, []api.PluginRef{ref})
		if err != nil {
			return nil, fmt.Errorf("create plugins for %q: %w", ref.Ref(), err)
		}
		all = append(all, plugins...)
	}

	return all, nil
}

// collectRequirements expands all refs by calling GetRequirements recursively
// until no new refs appear. Returns deduplicated refs in discovery order.
func (r *PluginResolver) collectRequirements(initial []api.PluginRef) ([]api.PluginRef, error) {
	seen := make(map[string]bool)
	var result []api.PluginRef
	queue := make([]api.PluginRef, len(initial))
	copy(queue, initial)

	for len(queue) > 0 {
		ref := queue[0]
		queue = queue[1:]

		key := ref.Ref()
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, ref)

		strategy, ok := r.strategies[ref.Type]
		if !ok {
			continue // fallback refs have no requirements
		}

		deps, err := strategy.GetRequirements(ref)
		if err != nil {
			return nil, fmt.Errorf("get requirements for %q: %w", key, err)
		}
		for _, dep := range deps {
			if !seen[dep.Ref()] {
				queue = append(queue, dep)
			}
		}
	}

	return result, nil
}

func (r *PluginResolver) resolveFallback(ref api.PluginRef) api.PluginFactory {
	if r.fallback == nil {
		return nil
	}
	if f, ok := r.fallback(ref.Ref()); ok {
		return f
	}
	if ref.Name == "" {
		if f, ok := r.fallback(ref.Type); ok {
			return f
		}
	}
	return nil
}

// MergeResources merges plugin resources from multiple layers.
// Later layers override earlier layers (by strategy:pattern key).
// Order: builtins → defaults → profile → task
func MergeResources(layers ...[]api.PluginRef) []api.PluginRef {
	seen := make(map[string]int) // ref → index in result
	var result []api.PluginRef

	for _, layer := range layers {
		for _, res := range layer {
			ref := res.Ref()
			if idx, exists := seen[ref]; exists {
				result[idx] = res // override
			} else {
				seen[ref] = len(result)
				result = append(result, res)
			}
		}
	}

	return result
}

// LegacyPluginsToResources converts legacy []string plugin names to []PluginRef.
func LegacyPluginsToResources(names []string) []api.PluginRef {
	resources := make([]api.PluginRef, len(names))
	for i, name := range names {
		resources[i] = api.ParsePluginRef(name)
	}
	return resources
}

// DefaultsToResources converts default plugin name strings to resources.
func DefaultsToResources(defaults []string) []api.PluginRef {
	return LegacyPluginsToResources(defaults)
}

// DetectCycle checks if adding name to path creates a cycle.
func DetectCycle(name string, path []string) error {
	for _, p := range path {
		if p == name {
			return fmt.Errorf("cycle detected: %s → %s", strings.Join(path, " → "), name)
		}
	}
	return nil
}
