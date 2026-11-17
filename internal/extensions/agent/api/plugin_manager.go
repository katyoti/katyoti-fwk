// SPDX-License-Identifier: Apache-2.0

package api

import "context"

// PluginManager resolves plugin references (skills, groups, leaf
// strategies, …) into instantiated `AgentPlugin` values. Exposed via
// the application service container so callers outside the agent
// package (e.g. the cwm plugin's dynamic-injection path) can drive a
// resolve on demand without re-implementing the 2-phase resolver.
//
// The implementation lives in `internal/extensions/agent/internal/plugin`;
// this interface is the public seam.
//
// `profilePlugins` are the static plugin refs from the LLM profile /
// task config. `taskPlugins` are the runtime additions merged on top.
// `defaults` is the bare-name fallback applied when both slices are
// empty (e.g. `["augmented", "context"]` from `Plugins.yaml`).
//
// `ctx` MUST carry the application reference (it's required by every
// strategy that uses `application.UseService`). `loop` is the shared
// agent-loop state — pass the same `LoopContext` the outer agent run
// owns, so any plugin recursively resolved here sees the canonical
// turn state.
//
// Re-entrant: the resolver itself is stateless across `Resolve` calls,
// so a plugin's `CreatePlugins` may itself invoke `PluginManager.CreatePlugins`
// (e.g. the cwm plugin spawning skill plugins) without deadlocking.
type PluginManager interface {
	CreatePlugins(
		profilePlugins []PluginRef,
		taskPlugins []PluginRef,
		defaults []string,
		ctx context.Context,
		loop LoopContext,
	) ([]AgentPlugin, error)
	// Has reports whether a plugin strategy is registered in this binary.
	Has(strategy string) bool
}
