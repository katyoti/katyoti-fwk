// SPDX-License-Identifier: Apache-2.0

package api

// PluginStrategy defines how a strategy type resolves plugin references
// into PluginFactory instances.
//
// Resolution is a two-phase process:
//  1. Requirements: the resolver calls GetRequirements for each ref of this type,
//     collecting additional refs (e.g., a skill's "requires" field). This repeats
//     until no new refs appear.
//  2. Batch resolve: the resolver groups all refs by strategy type and calls
//     ResolveAll once per strategy with the complete list. The strategy returns
//     one or more PluginFactory instances (e.g., one factory for all skills).
//
// Each strategy owns its naming convention:
//   - "skill" strategy: name is a Skill name (e.g., "doc-create")
//   - "task" strategy: name is [endpoint:]task_name
//   - "mcp" strategy: name is a server name, props hold inline config
//   - "group" strategy: name is a group name from Plugins.yaml
//   - Simple strategies (e.g., "workspace"): name is empty
type PluginStrategy interface {
	// Name returns the strategy identifier (e.g., "skill", "task", "mcp").
	Name() string

	// GetRequirements returns additional plugin refs required by the given ref.
	// Called per-ref during the requirements collection phase.
	// Return nil if this ref has no additional dependencies.
	GetRequirements(ref PluginRef) ([]PluginRef, error)

	// ResolveAll receives all refs of this strategy type (after requirements
	// collection is complete) and returns one or more configured PluginFactory.
	// The strategy decides whether to create one factory per ref, one shared
	// factory for all refs, or any combination.
	ResolveAll(refs []PluginRef) ([]PluginFactory, error)
}
