// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// MachineStrategyDef registers "workflow" in the MachineDomain.
var MachineStrategyDef = strategies.FuncStrategy[machine.MachineFactory](
	"workflow", "Multi-step workflow executor",
	func() (machine.MachineFactory, error) {
		return &workflowFactory{}, nil
	},
)

type workflowFactory struct{}

func (f *workflowFactory) Name() string { return "workflow" }

func (f *workflowFactory) CreateMachine(ctx context.Context, request any) (machine.Machine, error) {
	config, input, dryRun, currentNode := f.resolveRequest(request)

	// Load Node tree from config.
	root, err := LoadNodes(config)
	if err != nil {
		return nil, fmt.Errorf("workflow: %w", err)
	}

	// Create Spawner from shared Channel. LocalSpawner lives in
	// internal/core after D3's structural move (was in api/engine);
	// wkflow now imports it via the taskcore alias.
	// Its children are runs of the task service: its spawner in ctx (an
	// engine test passes a local one). None: a step that starts a child fails.
	spawner := machine.SpawnerFrom(ctx)

	// Index all nodes by ID for O(1) lookup.
	tree := make(map[string]*Node)
	indexTree(tree, root)

	// Read the workflow's own delegation mode from config (UC-053 / UC-054).
	// This is what handlerSpawn passes to ChildSpec.ParentMode so spawn-
	// policies fire on workflow-spawned children. Populated by the agent
	// factory via ConfigOverride (or directly via task definition for
	// nested workflows).
	mode, _ := config["_mode"].(string)

	// Read inherited sticky policies from `spawnPolicies` in config (set by
	// BuildChildOverrideWithPolicies's `serializeOverride` when this workflow
	// was spawned by a policy-carrying parent). UC-054 transitive case:
	// when this workflow spawns its own children, it forwards these names
	// via ChildSpec.ParentSpawnPolicyNames so the policies re-apply.
	spawnPolicies := readSpawnPolicies(config)

	return &workflowMachine{
		root:          root,
		tree:          tree,
		input:         input,
		spawner:       spawner,
		dryRunMode:    dryRun,
		mode:          mode,
		spawnPolicies: spawnPolicies,
		currentNode:   currentNode,
		exprCache:     el.NewCache(),
	}, nil
}

// readSpawnPolicies extracts the inherited sticky-policy names from a
// workflow's config map. Tolerant of both `[]any` (the standard JSON-decoded
// shape) and `[]string` (a directly-built map) — workflows may be
// instantiated through either path.
func readSpawnPolicies(config map[string]any) []string {
	raw, ok := config[presets.OverrideKeySpawnPolicies]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string{}, v...)
	}
	return nil
}

func (f *workflowFactory) resolveRequest(request any) (config map[string]any, input map[string]any, dryRun bool, currentNode map[string]any) {
	switch r := request.(type) {
	case *taskapi.TaskExecRequest:
		cfg, _ := r.Config.(map[string]any)
		if cfg == nil {
			cfg = make(map[string]any)
		}
		inp := r.Input
		if inp == nil {
			inp = make(map[string]any)
		}
		return cfg, inp, r.Runtime.DryRunMode, r.Runtime.CurrentNode
	default:
		return make(map[string]any), make(map[string]any), false, nil
	}
}

// ── SchemaProvider ─────────────────────────────────────────────────────

func (f *workflowFactory) DefaultInputSchema() json.RawMessage  { return nil }
func (f *workflowFactory) DefaultOutputSchema() json.RawMessage { return nil }
func (f *workflowFactory) BodyFieldName() string                { return "" }

var _ machine.SchemaProvider = (*workflowFactory)(nil)
