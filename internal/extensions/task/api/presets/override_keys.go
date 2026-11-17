// SPDX-License-Identifier: Apache-2.0

package presets

import taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"

// ConfigOverride wire contract — the key names of the override map that
// travels from the task side (producer: SerializeOverride) to the agent
// side (consumer: AgentMachineFactory's JSON decode onto AgentRunRequest)
// and to every other reader (workflow spawn nodes, the dry-run
// simulator's dimension recorder, the spawn audit event, the resume
// flow).
//
// RULES:
//   - No raw key literals outside this package — producers and readers
//     reference these constants (a typo'd literal is a silent no-op;
//     the round-trip test in agent/local pins the whole set).
//   - Top-level keys MUST equal the corresponding AgentRunRequest json
//     tags — the agent factory decodes the merged config by marshaling
//     it to JSON; nested keys must equal the nested structs' json tags
//     (SpawnPermissions, WorkspacePaths).
//   - Adding a dimension field = one entry here + one write in
//     SerializeOverride + the AgentRunRequest field — the round-trip
//     test fails until all three agree.
const (
	// Top-level override keys (== AgentRunRequest json tags).
	OverrideKeySpawnPermissions = "spawnPermissions"
	OverrideKeyWorkspacePaths   = "workspacePaths"
	OverrideKeyHistoryMode      = "historyMode"
	OverrideKeyIsolationMode    = "isolationMode"
	OverrideKeyMaxBudgetUSD     = "maxBudgetUSD"
	OverrideKeyUserCommsInput   = "userCommsInput"
	OverrideKeyFinish           = "finish"
	// OverrideKeySpawnPolicies is consumed by the RUNTIME spawn path
	// (taskplugin / workflow factory resurrect sticky policies), not by
	// AgentRunRequest.
	OverrideKeySpawnPolicies = "spawnPolicies"
	// OverrideKeyInitialMessages carries the parent conversation for
	// fork-mode / resume flows (== AgentRunRequest json tag).
	OverrideKeyInitialMessages = "initialMessages"

	// Nested under spawnPermissions (== api.SpawnPermissions json tags).
	OverrideKeyAllowedTools    = "allowedTools"
	OverrideKeyDisallowedTools = "disallowedTools"
	OverrideKeyPermissionMode  = "permissionMode"

	// Nested under workspacePaths (== api.WorkspacePaths json tags;
	// strategy is audit-only — the runtime decoder ignores it).
	OverrideKeyReadPaths  = "readPaths"
	OverrideKeyWritePaths = "writePaths"
	OverrideKeyStrategy   = "strategy"
)

// TASK.md config keys the agent factory reads EXPLICITLY from the
// config map (yaml-only names the JSON round-trip cannot map onto
// AgentRunRequest fields).
const (
	// ConfigKeyPrompt is where the framework injects the task body
	// (BodyFieldName).
	ConfigKeyPrompt = "prompt"
	// ConfigKeyCapability selects the LLM profile (maps to
	// AgentRunRequest.ProfileName, whose json tag is "profileName").
	ConfigKeyCapability = taskapi.ConfigKeyCapability
)
