// SPDX-License-Identifier: Apache-2.0

package simulator

import (
	"context"
	"reflect"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// runSimulator is a small driver: builds a Machine via the factory, drives it
// to completion via Executor.Run, and returns the resulting StepResult.Output.
// Keeps the test bodies focused on the assertion (not the engine wiring).
func runSimulator(t *testing.T, request *taskapi.TaskExecRequest) any {
	t.Helper()

	f := &simulatorFactory{}
	machine, err := f.CreateMachine(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}

	bus := engine.NewMemoryEventBus()
	channel := engine.NewChannel(bus)
	executor := engine.NewExecutor(machine, channel, nil)

	result, err := executor.Run(context.Background(), engine.Event{Type: "start", Payload: request})
	if err != nil {
		t.Fatalf("executor.Run: %v", err)
	}
	if result == nil {
		t.Fatal("executor returned nil result")
	}
	return result.Output
}

// TestRecordContext_BasicSnapshot — Phase A primary test.
//
// Runs the simulator with RecordContext=true and Output=nil; asserts the
// four "__"-prefixed snapshots are present and shaped correctly.
func TestRecordContext_BasicSnapshot(t *testing.T) {
	configMap := map[string]any{
		"record_context": true,
	}
	req := &taskapi.TaskExecRequest{
		Config: configMap,
		Input:  map[string]any{"task": "test-input"},
		Runtime: taskapi.RuntimeContext{
			WorkspaceRoot: "/tmp/ws",
			ParentRunID:   "run-parent-abc",
			SessionID:     "session-xyz",
			DryRunMode:    false,
		},
	}

	out := runSimulator(t, req)

	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T: %v", out, out)
	}

	// __runtime is shaped per RuntimeContext fields.
	rt, ok := m["__runtime"].(map[string]any)
	if !ok {
		t.Fatalf("__runtime missing or wrong shape: %T", m["__runtime"])
	}
	if rt["workspace_root"] != "/tmp/ws" {
		t.Errorf("__runtime.workspace_root = %v, want /tmp/ws", rt["workspace_root"])
	}
	if rt["parent_run_id"] != "run-parent-abc" {
		t.Errorf("__runtime.parent_run_id = %v, want run-parent-abc", rt["parent_run_id"])
	}
	if rt["session_id"] != "session-xyz" {
		t.Errorf("__runtime.session_id = %v, want session-xyz", rt["session_id"])
	}

	// __config is the raw config map (carries record_context itself).
	cfg, ok := m["__config"].(map[string]any)
	if !ok {
		t.Fatalf("__config missing or wrong shape: %T", m["__config"])
	}
	if cfg["record_context"] != true {
		t.Errorf("__config.record_context = %v, want true", cfg["record_context"])
	}

	// __input echoes the input map.
	in, ok := m["__input"].(map[string]any)
	if !ok {
		t.Fatalf("__input missing or wrong shape: %T", m["__input"])
	}
	if in["task"] != "test-input" {
		t.Errorf("__input.task = %v, want test-input", in["task"])
	}

	// __spawn_dimensions has all 6 dimension keys (empty when no policy applied).
	dims, ok := m["__spawn_dimensions"].(map[string]any)
	if !ok {
		t.Fatalf("__spawn_dimensions missing or wrong shape: %T", m["__spawn_dimensions"])
	}
	for _, key := range []string{"scheduling", "context", "permissions", "agent_comms", "user_comms", "isolation"} {
		if _, ok := dims[key].(map[string]any); !ok {
			t.Errorf("__spawn_dimensions.%s missing or wrong shape: %T", key, dims[key])
		}
	}
}

// TestRecordContext_DimensionsFromParentPolicy — closes UC-053 by structural
// assertion at the simulator-output layer.
//
// Simulates what BuildChildOverride writes when a coordinator parent applies
// CoordinatorRestrictsChildrenPolicy: spawnPermissions block with
// allowedTools=SafeToolSubset and permissionMode=restricted. Asserts the
// simulator's __spawn_dimensions surfaces these correctly.
func TestRecordContext_DimensionsFromParentPolicy(t *testing.T) {
	// Mimic the post-BuildChildOverride config map a coordinator-spawned child
	// would receive. Keys must match what presets.serializeOverride writes.
	configMap := map[string]any{
		"record_context": true,
		"spawnPermissions": map[string]any{
			"allowedTools":   []any{"file-read", "file-write", "file-edit", "glob", "complete"},
			"permissionMode": "restricted",
		},
		"workspacePaths": map[string]any{
			"readPaths":  []any{"src/", "docs/"},
			"writePaths": []any{"src/"},
		},
		"historyMode":   "prompt_only",
		"isolationMode": "context_isolated",
		"maxBudgetUSD":  5.0,
	}
	req := &taskapi.TaskExecRequest{
		Config:  configMap,
		Input:   map[string]any{},
		Runtime: taskapi.RuntimeContext{},
	}

	out := runSimulator(t, req)
	m := out.(map[string]any)
	dims := m["__spawn_dimensions"].(map[string]any)

	// Permissions — SafeToolSubset asserted directly.
	perms := dims["permissions"].(map[string]any)
	wantTools := []any{"file-read", "file-write", "file-edit", "glob", "complete"}
	if got := perms["allowed_tools"]; !reflect.DeepEqual(got, wantTools) {
		t.Errorf("permissions.allowed_tools = %v, want %v", got, wantTools)
	}
	if perms["permission_mode"] != "restricted" {
		t.Errorf("permissions.permission_mode = %v, want restricted", perms["permission_mode"])
	}

	// Context — workspace paths + history mode.
	ctxDim := dims["context"].(map[string]any)
	if ctxDim["history_mode"] != "prompt_only" {
		t.Errorf("context.history_mode = %v, want prompt_only", ctxDim["history_mode"])
	}
	wsPaths, ok := ctxDim["workspace_paths"].(map[string]any)
	if !ok {
		t.Fatalf("context.workspace_paths missing")
	}
	if got := wsPaths["read_paths"]; !reflect.DeepEqual(got, []any{"src/", "docs/"}) {
		t.Errorf("context.workspace_paths.read_paths = %v", got)
	}

	// Isolation — mode passed through.
	iso := dims["isolation"].(map[string]any)
	if iso["mode"] != "context_isolated" {
		t.Errorf("isolation.mode = %v, want context_isolated", iso["mode"])
	}

	// Scheduling — budget surfaced.
	sched := dims["scheduling"].(map[string]any)
	if sched["max_budget_usd"] != 5.0 {
		t.Errorf("scheduling.max_budget_usd = %v, want 5.0", sched["max_budget_usd"])
	}
}

// TestRecordContext_BackwardCompat — RecordContext=false (default) leaves the
// existing default-output map untouched. No "__"-prefixed keys, no behavior
// drift for existing tests.
func TestRecordContext_BackwardCompat(t *testing.T) {
	req := &taskapi.TaskExecRequest{
		Config: map[string]any{
			"duration_ms": 0,
			// record_context omitted → defaults to false
		},
		Input: map[string]any{"x": 1},
	}
	out := runSimulator(t, req)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", out)
	}
	for _, key := range []string{"__runtime", "__config", "__input", "__spawn_dimensions"} {
		if _, present := m[key]; present {
			t.Errorf("expected no %q key when RecordContext=false; map = %v", key, m)
		}
	}
	// Existing default-output keys still present.
	for _, key := range []string{"status", "simulator", "input", "duration"} {
		if _, present := m[key]; !present {
			t.Errorf("default output key %q missing; map = %v", key, m)
		}
	}
}

// TestRecordContext_MergeWithExplicitOutput — when both RecordContext=true and
// Output={...} are set, introspection keys are merged into a copy of Output.
// User keys win for non-"__" fields; "__" keys are framework-controlled.
func TestRecordContext_MergeWithExplicitOutput(t *testing.T) {
	configMap := map[string]any{
		"record_context": true,
		"output": map[string]any{
			"role":      "passthrough",
			"__runtime": "user-supplied-should-be-overwritten",
		},
	}
	req := &taskapi.TaskExecRequest{
		Config:  configMap,
		Runtime: taskapi.RuntimeContext{WorkspaceRoot: "/ws"},
	}

	out := runSimulator(t, req)
	m := out.(map[string]any)

	// User key preserved.
	if m["role"] != "passthrough" {
		t.Errorf("role = %v, want passthrough (user key should be preserved)", m["role"])
	}
	// Framework key overwrote the user-supplied __runtime.
	rt, ok := m["__runtime"].(map[string]any)
	if !ok {
		t.Fatalf("__runtime should be the framework-derived map, got %T: %v", m["__runtime"], m["__runtime"])
	}
	if rt["workspace_root"] != "/ws" {
		t.Errorf("__runtime.workspace_root = %v, want /ws", rt["workspace_root"])
	}
}

// TestRecordContext_NonMapOutputWrapped — Output that's a string/number is
// wrapped under {"value": Output} so the introspection keys can sit alongside.
func TestRecordContext_NonMapOutputWrapped(t *testing.T) {
	configMap := map[string]any{
		"record_context": true,
		"output":         "hello world",
	}
	req := &taskapi.TaskExecRequest{Config: configMap}

	out := runSimulator(t, req)
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("non-map Output should be wrapped into a map; got %T", out)
	}
	if m["value"] != "hello world" {
		t.Errorf("value = %v, want hello world", m["value"])
	}
	if _, present := m["__spawn_dimensions"]; !present {
		t.Error("__spawn_dimensions should be present even when Output was non-map")
	}
}
