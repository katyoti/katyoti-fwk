// SPDX-License-Identifier: Apache-2.0

package presets

import "fmt"

// DefaultMode is used when no mode is specified.
const DefaultMode = ModeWorker

// ResolvePreset returns the SpawnConfig for the given mode, with optional
// field-level overrides applied. Unknown modes return an error.
//
// Mode resolution priority (enforced by callers, documented here):
//  1. Explicit mode parameter to delegate() or spawn node
//  2. TASK.md frontmatter mode: field
//  3. DefaultMode (worker)
//
// Overrides are merged shallowly into the preset. Nil or zero-valued fields
// in the overrides are ignored (preset value is kept).
func ResolvePreset(mode Mode, overrides *SpawnConfig) (SpawnConfig, error) {
	if mode == "" {
		mode = DefaultMode
	}
	base, ok := presetsByMode[mode]
	if !ok {
		return SpawnConfig{}, fmt.Errorf("unknown delegation mode: %q", mode)
	}
	if overrides == nil {
		return base, nil
	}
	return mergeConfig(base, *overrides), nil
}

// MustResolvePreset is like ResolvePreset but panics on unknown mode.
// Use only for static initialization.
func MustResolvePreset(mode Mode) SpawnConfig {
	cfg, err := ResolvePreset(mode, nil)
	if err != nil {
		panic(err)
	}
	return cfg
}

// mergeConfig shallow-merges overrides into base.
// For each field: if the override has a non-zero value, use it; else keep base.
func mergeConfig(base, ov SpawnConfig) SpawnConfig {
	out := base

	// Scheduling
	if ov.Scheduling.Mode != "" {
		out.Scheduling.Mode = ov.Scheduling.Mode
	}
	if ov.Scheduling.AutoBackgroundMs != 0 {
		out.Scheduling.AutoBackgroundMs = ov.Scheduling.AutoBackgroundMs
	}
	if ov.Scheduling.FanOutError != "" {
		out.Scheduling.FanOutError = ov.Scheduling.FanOutError
	}
	if ov.Scheduling.Discovery != "" {
		out.Scheduling.Discovery = ov.Scheduling.Discovery
	}
	if ov.Scheduling.Pool != "" {
		out.Scheduling.Pool = ov.Scheduling.Pool
	}
	if ov.Scheduling.Limits != (ResourceLimits{}) {
		out.Scheduling.Limits = ov.Scheduling.Limits
	}

	// Context
	if ov.Context.History != "" {
		out.Context.History = ov.Context.History
	}
	if ov.Context.Memory != "" {
		out.Context.Memory = ov.Context.Memory
	}
	if ov.Context.Model != "" {
		out.Context.Model = ov.Context.Model
	}
	if ov.Context.SystemPrompt != "" {
		out.Context.SystemPrompt = ov.Context.SystemPrompt
	}
	out.Context.Workspace = mergeWorkspace(base.Context.Workspace, ov.Context.Workspace)

	// Permissions
	if len(ov.Permissions.Tools) > 0 {
		out.Permissions.Tools = ov.Permissions.Tools
	}
	if len(ov.Permissions.DisallowedTools) > 0 {
		out.Permissions.DisallowedTools = ov.Permissions.DisallowedTools
	}
	if ov.Permissions.PermissionMode != "" {
		out.Permissions.PermissionMode = ov.Permissions.PermissionMode
	}
	if ov.Permissions.PlanMode {
		out.Permissions.PlanMode = true
	}

	// AgentComms — bools: override only if different from zero-value defaults.
	// Since the override is a user-provided value, we trust it when explicitly set.
	// For bool fields, we use a simple rule: if override is true, it wins.
	// (Turning off a preset's true requires passing the explicit SpawnConfig, not presets.)
	if ov.AgentComms.Notifications {
		out.AgentComms.Notifications = true
	}
	if ov.AgentComms.Messaging {
		out.AgentComms.Messaging = true
	}
	if ov.AgentComms.Progress {
		out.AgentComms.Progress = true
	}

	// UserComms
	if ov.UserComms.Display != "" {
		out.UserComms.Display = ov.UserComms.Display
	}
	if ov.UserComms.Input != "" {
		out.UserComms.Input = ov.UserComms.Input
	}

	// Isolation
	if ov.Isolation.Mode != "" {
		out.Isolation.Mode = ov.Isolation.Mode
	}

	return out
}

func mergeWorkspace(base, ov WorkspaceConfig) WorkspaceConfig {
	out := base
	if ov.Mode != "" {
		out.Mode = ov.Mode
	}
	if ov.Strategy != "" {
		out.Strategy = ov.Strategy
	}
	if ov.BaseDir != "" {
		out.BaseDir = ov.BaseDir
	}
	if ov.OutputDir != "" {
		out.OutputDir = ov.OutputDir
	}
	if ov.Branch != "" {
		out.Branch = ov.Branch
	}
	if len(ov.ReadPaths) > 0 {
		out.ReadPaths = ov.ReadPaths
	}
	if len(ov.WritePaths) > 0 {
		out.WritePaths = ov.WritePaths
	}
	return out
}
