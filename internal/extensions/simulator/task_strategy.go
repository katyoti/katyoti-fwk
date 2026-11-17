// SPDX-License-Identifier: Apache-2.0

// Package simulator provides a token-free machine strategy for testing
// workflow patterns. The simulator sleeps for a configurable duration,
// optionally fails with a given probability, and returns static or
// input-derived output. No LLM calls.
//
// When SimulatorConfig.RecordContext is true, the simulator's Output is
// augmented with introspection snapshots (__runtime / __config / __input /
// __spawn_dimensions) so L2 tests can assert that the spawn dimensions
// actually applied at runtime — not just that the parent intended them.
// See `tier-1-simulator-introspection` task for design context.
package simulator

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// MachineStrategyDef registers "simulator" in the MachineDomain.
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	"simulator", "Token-free task simulator for testing workflows",
	func() (engine.MachineFactory, error) {
		return &simulatorFactory{}, nil
	},
)

// SimulatorConfig is the strategy-specific config decoded from the TASK.md config: block.
type SimulatorConfig struct {
	// DurationMs is the simulated work time in milliseconds. 0 = instant.
	DurationMs int `json:"duration_ms" yaml:"duration_ms"`

	// Output is the value returned on success. Can be a string, map, or any JSON value.
	// If nil, returns a default summary map with input echo.
	Output any `json:"output,omitempty" yaml:"output"`

	// FailProbability is the chance of failure (0.0 = never, 1.0 = always).
	FailProbability float64 `json:"fail_probability,omitempty" yaml:"fail_probability"`

	// FailMessage is the error message when the simulator fails.
	FailMessage string `json:"fail_message,omitempty" yaml:"fail_message"`

	// Script, when non-empty, makes the simulator CONVERSATIONAL: it runs the
	// steps in order, driving the comms service (ask / await / answer / stop /
	// notify) with zero LLM. An empty Script keeps the one-shot behavior below
	// (sleep → output). See script.go.
	Script []Step `json:"script,omitempty" yaml:"script"`

	// RecordContext enables runtime introspection. When true, the run's Output
	// is augmented with four "__"-prefixed keys: __runtime (RuntimeContext),
	// __config (the merged TaskExecRequest.Config map), __input (the input
	// map), and __spawn_dimensions (a flattened 6-dimension view derived from
	// __config). L2 .tasktest scenarios use these to assert that
	// dimension settings (SafeToolSubset, WorkspacePaths, HistoryMode, etc.)
	// actually applied — not just that the parent's BuildChildOverride
	// intended them. Default false (back-compat).
	RecordContext bool `json:"record_context,omitempty" yaml:"record_context"`
}

type simulatorFactory struct{}

func (f *simulatorFactory) Name() string { return "simulator" }

func (f *simulatorFactory) CreateMachine(_ context.Context, request any) (engine.Machine, error) {
	cfg, input, runtime, configMap := resolveRequest(request)

	// Its one step is a token-free stand-in: safe to run again after a crash.
	return engine.ReplaySafe(engine.OneShotMachine("simulator", func(ctx context.Context) (any, error) {
		// Conversational mode: run the script through the comms channel.
		if len(cfg.Script) > 0 {
			return runScript(ctx, cfg, runtime)
		}

		// Simulate work duration.
		if cfg.DurationMs > 0 {
			select {
			case <-time.After(time.Duration(cfg.DurationMs) * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Random failure.
		if cfg.FailProbability > 0 && rand.Float64() < cfg.FailProbability {
			msg := cfg.FailMessage
			if msg == "" {
				msg = "simulated failure"
			}
			return nil, fmt.Errorf("%s", msg)
		}

		// Build the base output.
		var out any
		if cfg.Output != nil {
			out = cfg.Output
		} else {
			out = map[string]any{
				"status":    "completed",
				"simulator": true,
				"input":     input,
				"duration":  fmt.Sprintf("%dms", cfg.DurationMs),
			}
		}

		// Introspection: augment with __-prefixed snapshots when RecordContext
		// is enabled. The merge rules are documented on SimulatorConfig.
		if cfg.RecordContext {
			out = withIntrospection(out, runtime, configMap, input)
		}

		return out, nil
	})), nil
}

// resolveRequest extracts the simulator-relevant pieces of the framework's
// request envelope. Returns the decoded config, the input map, the runtime
// context, and the raw config map (for introspection re-derivation).
func resolveRequest(request any) (SimulatorConfig, map[string]any, taskapi.RuntimeContext, map[string]any) {
	var cfg SimulatorConfig
	var input map[string]any
	var runtime taskapi.RuntimeContext
	var configMap map[string]any

	switch r := request.(type) {
	case *taskapi.TaskExecRequest:
		if cm, ok := r.Config.(map[string]any); ok {
			configMap = cm
			data, _ := json.Marshal(cm)
			_ = json.Unmarshal(data, &cfg)
		}
		input = r.Input
		runtime = r.Runtime
	}

	return cfg, input, runtime, configMap
}

// withIntrospection wraps the simulator's logical Output with four
// "__"-prefixed snapshots. Merge semantics (per SimulatorConfig.RecordContext):
//
//   - Output is a map  → introspection keys merged into a copy of Output.
//     Output's keys win for non-"__" fields; "__" keys are framework-controlled
//     and overwrite any user-supplied values.
//   - Output is non-map → wrapped into {value: Output, __runtime: ..., ...}.
//
// configMap may be nil when the framework called CreateMachine with a non-map
// Config (legacy path); in that case __config is omitted.
func withIntrospection(
	out any,
	runtime taskapi.RuntimeContext,
	configMap map[string]any,
	input map[string]any,
) map[string]any {
	merged := map[string]any{}

	switch typed := out.(type) {
	case map[string]any:
		for k, v := range typed {
			merged[k] = v
		}
	default:
		// Non-map Output (string, number, slice, etc.): wrap.
		merged["value"] = out
	}

	merged["__runtime"] = map[string]any{
		"workspace_root": runtime.WorkspaceRoot,
		"parent_run_id":  runtime.ParentRunID,
		"session_id":     runtime.SessionID,
		"dry_run_mode":   runtime.DryRunMode,
	}
	if configMap != nil {
		merged["__config"] = configMap
	}
	merged["__input"] = input
	merged["__spawn_dimensions"] = flattenSpawnDimensions(configMap)

	return merged
}

// flattenSpawnDimensions re-derives a 6-dimension view from the post-merge
// config map. Mirrors presets.SpawnConfig structure so L2 assertions can use
// stable, dimension-grouped paths regardless of how the framework's
// BuildChildOverride writes individual keys.
//
// Re-derivation (rather than carrying a parallel field) keeps the simulator's
// view in lockstep with the actual config map — no synchronisation hazard.
//
// Sub-keys returned (always present, populated only when the config has data):
//
//	scheduling   — {max_budget_usd}
//	context      — {history_mode, workspace_paths: {read_paths, write_paths}}
//	permissions  — {allowed_tools, disallowed_tools, permission_mode}
//	agent_comms  — (empty today; reserved for future config-routed comms fields)
//	user_comms   — (empty today; reserved)
//	isolation    — {mode}
//
// Some dimension fields (max_duration_s, fire_and_forget, _teammate_name,
// _team_name) flow through TaskRunRequest or loop meta, not through the
// config map — so the simulator can't see them. L2 tests for those UCs assert
// at the parent or harness level instead.
func flattenSpawnDimensions(configMap map[string]any) map[string]any {
	dims := map[string]any{
		"scheduling":  map[string]any{},
		"context":     map[string]any{},
		"permissions": map[string]any{},
		"agent_comms": map[string]any{},
		"user_comms":  map[string]any{},
		"isolation":   map[string]any{},
	}
	if configMap == nil {
		return dims
	}

	// Permissions — sourced from the spawnPermissions sub-map written by
	// presets.SerializeOverride.
	if sp, ok := configMap[presets.OverrideKeySpawnPermissions].(map[string]any); ok {
		perms := dims["permissions"].(map[string]any)
		if v, ok := sp[presets.OverrideKeyAllowedTools]; ok {
			perms["allowed_tools"] = v
		}
		if v, ok := sp[presets.OverrideKeyDisallowedTools]; ok {
			perms["disallowed_tools"] = v
		}
		if v, ok := sp[presets.OverrideKeyPermissionMode]; ok {
			perms["permission_mode"] = v
		}
	}

	// Permissions.spawn_policies — sourced from the top-level spawnPolicies
	// list written by presets.SerializeOverride when the parent's preset
	// carries SpawnPolicies (coordinator, team-leader). The list is opaque
	// to the simulator — just a list of policy NAMES — but it lets L2
	// scenarios verify the policy actually propagates as a config field
	// (the CAUSE), separately from asserting on its EFFECT (allowed_tools,
	// permission_mode). Closes the "name vs effect" gap for UC-053/054.
	if v, ok := configMap[presets.OverrideKeySpawnPolicies]; ok {
		perms := dims["permissions"].(map[string]any)
		perms["spawn_policies"] = v
	}

	// Context.workspace_paths — sourced from workspacePaths sub-map.
	if wp, ok := configMap[presets.OverrideKeyWorkspacePaths].(map[string]any); ok {
		ctxDim := dims["context"].(map[string]any)
		paths := map[string]any{}
		if v, ok := wp[presets.OverrideKeyReadPaths]; ok {
			paths["read_paths"] = v
		}
		if v, ok := wp[presets.OverrideKeyWritePaths]; ok {
			paths["write_paths"] = v
		}
		if v, ok := wp["mode"]; ok {
			paths["mode"] = v
		}
		if v, ok := wp[presets.OverrideKeyStrategy]; ok {
			paths["strategy"] = v
		}
		if len(paths) > 0 {
			ctxDim["workspace_paths"] = paths
		}
	}

	// Context.history_mode — top-level historyMode key.
	if v, ok := configMap[presets.OverrideKeyHistoryMode]; ok {
		ctxDim := dims["context"].(map[string]any)
		ctxDim["history_mode"] = v
	}

	// Isolation.mode — top-level isolationMode key.
	if v, ok := configMap[presets.OverrideKeyIsolationMode]; ok {
		iso := dims["isolation"].(map[string]any)
		iso["mode"] = v
	}

	// Scheduling.max_budget_usd — top-level maxBudgetUSD key.
	if v, ok := configMap[presets.OverrideKeyMaxBudgetUSD]; ok {
		sched := dims["scheduling"].(map[string]any)
		sched["max_budget_usd"] = v
	}

	return dims
}

// ── SchemaProvider ─────────────────────────────────────────────────────

func (f *simulatorFactory) DefaultInputSchema() json.RawMessage {
	return json.RawMessage(`{"name":"simulator-input","root":"SimulatorInput","types":{"SimulatorInput":{"type":"object","description":"Arbitrary input passed through to output","catchall":"any"}}}`)
}

func (f *simulatorFactory) DefaultOutputSchema() json.RawMessage { return nil }

func (f *simulatorFactory) BodyFieldName() string { return "" }

var _ engine.SchemaProvider = (*simulatorFactory)(nil)
