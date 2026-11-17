// SPDX-License-Identifier: Apache-2.0

package local

import (
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

// TestConfigOverride_RoundTrip pins the ConfigOverride wire format:
// the task side's builder (presets.SerializeOverride) against the agent
// side's parser (resolveFromExecRequest's JSON decode onto
// AgentRunRequest). A key drift on EITHER side — a renamed constant, a
// changed json tag — fails here instead of silently no-opping a
// dimension (the rule: pin every wire-format parser to its builder).
func TestConfigOverride_RoundTrip(t *testing.T) {
	// A fully-populated, fully-restrictive SpawnConfig: every field the
	// serializer can emit is exercised.
	cfg := presets.SpawnConfig{
		Scheduling: presets.SchedulingConfig{
			Mode:   presets.SchedulingBackground,
			Limits: presets.ResourceLimits{MaxBudgetUSD: 2.5},
		},
		Context: presets.ContextConfig{
			History: presets.HistoryFullFork,
			Workspace: presets.WorkspaceConfig{
				Mode:       presets.WorkspaceDedicated,
				Strategy:   "git_worktree",
				ReadPaths:  []string{"src/", "docs/"},
				WritePaths: []string{"src/"},
			},
		},
		Permissions: presets.PermissionsConfig{
			Tools:           []string{"file-read", "glob"},
			DisallowedTools: []string{"bash"},
			PermissionMode:  presets.PermissionRestricted,
		},
		UserComms: presets.UserCommsConfig{Input: presets.InputViaLeader},
		Isolation: presets.IsolationConfig{Mode: presets.IsolationContextIsolated},
		SpawnPolicies: []presets.SpawnPolicy{
			presets.CoordinatorRestrictsChildrenPolicy{},
		},
	}

	override := presets.SerializeOverride(cfg)
	if override == nil {
		t.Fatal("SerializeOverride returned nil for a restrictive config")
	}

	// The task service merges the override over the task config; the
	// body lands under the prompt key. Feed the merged map through the
	// REAL parser.
	config := map[string]any{presets.ConfigKeyPrompt: "do the thing", presets.ConfigKeyCapability: "fast-best"}
	for k, v := range override {
		config[k] = v
	}

	f := NewAgentMachineFactory(nil, nil)
	req, err := f.resolveFromExecRequest(&taskapi.TaskExecRequest{Config: config})
	if err != nil {
		t.Fatalf("resolveFromExecRequest: %v", err)
	}

	// Explicit config keys.
	if req.Prompt != "do the thing" {
		t.Errorf("Prompt = %q", req.Prompt)
	}
	if req.ProfileName != "fast-best" {
		t.Errorf("ProfileName = %q (capability key drift?)", req.ProfileName)
	}

	// spawnPermissions → SpawnPermissions.
	if req.SpawnPermissions == nil {
		t.Fatal("SpawnPermissions did not decode — top-level key drift")
	}
	if got := req.SpawnPermissions.AllowedTools; len(got) != 2 || got[0] != "file-read" {
		t.Errorf("AllowedTools = %v", got)
	}
	if got := req.SpawnPermissions.DisallowedTools; len(got) != 1 || got[0] != "bash" {
		t.Errorf("DisallowedTools = %v", got)
	}
	if req.SpawnPermissions.PermissionMode != string(presets.PermissionRestricted) {
		t.Errorf("PermissionMode = %q", req.SpawnPermissions.PermissionMode)
	}

	// workspacePaths → WorkspacePaths.
	if req.WorkspacePaths == nil {
		t.Fatal("WorkspacePaths did not decode — top-level key drift")
	}
	if got := req.WorkspacePaths.ReadPaths; len(got) != 2 || got[0] != "src/" {
		t.Errorf("ReadPaths = %v", got)
	}
	if got := req.WorkspacePaths.WritePaths; len(got) != 1 || got[0] != "src/" {
		t.Errorf("WritePaths = %v", got)
	}

	// Scalar dimensions.
	if req.HistoryMode != string(presets.HistoryFullFork) {
		t.Errorf("HistoryMode = %q", req.HistoryMode)
	}
	if req.IsolationMode != string(presets.IsolationContextIsolated) {
		t.Errorf("IsolationMode = %q", req.IsolationMode)
	}
	if req.MaxBudgetUSD != 2.5 {
		t.Errorf("MaxBudgetUSD = %v", req.MaxBudgetUSD)
	}
	if req.UserCommsInput != string(presets.InputViaLeader) {
		t.Errorf("UserCommsInput = %q", req.UserCommsInput)
	}

	// spawnPolicies is a runtime-spawner key (not an AgentRunRequest
	// field) — pin its presence + shape in the serialized map itself.
	names, ok := override[presets.OverrideKeySpawnPolicies].([]any)
	if !ok || len(names) == 0 {
		t.Errorf("spawnPolicies missing from override: %v", override[presets.OverrideKeySpawnPolicies])
	}
}
