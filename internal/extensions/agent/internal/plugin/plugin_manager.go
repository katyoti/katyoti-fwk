// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// PluginManager (local alias) — re-exported for backwards-compat
// within the agent internals. Public interface lives at
// `internal/extensions/agent/api.PluginManager` so out-of-package
// consumers (e.g. the cwm plugin's dynamic-injection path) can
// resolve plugin refs without importing this internal package.
type PluginManager = api.PluginManager

type pluginManager struct {
	types typeLookup
}

// PluginManagerExtension registers the manager as `api.PluginManager`
// (the public interface). Callers do `application.UseService[api.PluginManager](ctx)`.
var PluginManagerExtension = application.DeclareSimpleService[api.PluginManager](
	func(_ context.Context) (api.PluginManager, error) {
		return &pluginManager{types: PluginDomain.Types}, nil
	},
)

func (pm *pluginManager) CreatePlugins(profilePlugins []api.PluginRef, taskPlugins []api.PluginRef, defaults []string, ctx context.Context, loop api.LoopContext) ([]api.AgentPlugin, error) {
	refs := profilePlugins
	if len(taskPlugins) > 0 {
		refs = mergePluginRefs(refs, taskPlugins)
	}

	// Apply defaults if no refs provided
	if len(refs) == 0 {
		refs = make([]api.PluginRef, len(defaults))
		for i, name := range defaults {
			refs[i] = api.ParsePluginRef(name)
		}
	}

	// Layer 1: expand groups
	if GlobalGroups == nil {
		GlobalGroups = make(GroupRegistry)
	}
	expanded, err := ExpandGroups(refs, GlobalGroups)
	if err != nil {
		return nil, fmt.Errorf("expand groups: %w", err)
	}

	// Build PluginStrategy wrappers from domain TypeDefs
	strats := BuildStrategiesFromDomain(pm.types)

	// Apply custom GetRequirements for skill strategy
	for _, s := range strats {
		if tds, ok := s.(*TypeDefStrategy); ok && tds.Name() == "skill" {
			tds.WithGetRequirements(func(ref api.PluginRef) ([]api.PluginRef, error) {
				return skillGetRequirements(ref, ctx)
			})
		}
	}

	// 2-phase resolve: collect requirements → batch resolve → create plugins
	resolver := NewPluginResolver(strats, pm.buildFallback())
	return resolver.Resolve(expanded, ctx, loop)
}

// Has reports whether a plugin strategy is registered.
func (pm *pluginManager) Has(strategy string) bool { return pm.types.Has(strategy) }

func (pm *pluginManager) buildFallback() func(name string) (api.PluginFactory, bool) {
	return func(name string) (api.PluginFactory, bool) {
		if pm.types.Has(name) {
			td := pm.types.Get(name)
			if td.Impl != nil {
				return td.Impl, true
			}
		}
		return nil, false
	}
}

// skillGetRequirements reads the `requires` field from skill metadata.
func skillGetRequirements(ref api.PluginRef, ctx context.Context) ([]api.PluginRef, error) {
	mgr, err := application.UseService[skillRequiresReader](ctx)
	if err != nil {
		return nil, nil // no skill manager — no requirements
	}
	// A `skills:` list desugars to ONE ref with the selectors in Props and
	// an empty Name; until 2026-09-22 this looked up requires by that empty
	// name and found nothing, so `requires:` was honoured only for the legacy
	// `skill:<name>` form. Exact names resolve; a glob or `*` cannot name a
	// skill and is left to the catalog match.
	names := []string{ref.Name}
	if ref.Name == "" {
		names = names[:0]
		for _, sel := range api.StringList(ref.Props["skills"]) {
			if sel == "" || strings.ContainsAny(sel, "*?[{") {
				continue
			}
			if i := strings.LastIndex(sel, "/"); i >= 0 {
				sel = sel[i+1:] // <provider>:/skill/<name> → name
			}
			names = append(names, sel)
		}
	}
	seen := map[string]bool{}
	var refs []api.PluginRef
	for _, name := range names {
		for _, r := range mgr.GetSkillRequires(name) {
			if !seen[r] {
				seen[r] = true
				refs = append(refs, api.ParsePluginRef(r))
			}
		}
	}
	return refs, nil
}

// skillRequiresReader is a minimal interface to read skill requires without
// importing the full skills package. Implemented by SkillManager.
type skillRequiresReader interface {
	GetSkillRequires(name string) []string
}

func mergePluginRefs(base, overrides []api.PluginRef) []api.PluginRef {
	if len(base) == 0 {
		return overrides
	}
	seen := make(map[string]int)
	result := make([]api.PluginRef, len(base))
	copy(result, base)
	for i, ref := range result {
		seen[ref.Ref()] = i
	}
	for _, ref := range overrides {
		if idx, ok := seen[ref.Ref()]; ok {
			result[idx] = ref
		} else {
			result = append(result, ref)
			seen[ref.Ref()] = len(result) - 1
		}
	}
	return result
}

var _ PluginManager = (*pluginManager)(nil)

type PluginsConfig = api.PluginsConfig
type PluginRef = api.PluginRef

func FormatError(pluginName string, err error) error {
	return fmt.Errorf("plugin %q: %w", pluginName, err)
}
