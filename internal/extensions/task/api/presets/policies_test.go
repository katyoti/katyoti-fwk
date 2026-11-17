// SPDX-License-Identifier: Apache-2.0

package presets

import (
	"reflect"
	"testing"
)

// ────────────────────────────────────────────────────────────────────────
// CoordinatorRestrictsChildrenPolicy — UC-053 + UC-054 (sticky)
// ────────────────────────────────────────────────────────────────────────

// TestCoordinatorRestrictsChildrenPolicy_Apply (UC-053) — Apply restricts
// the child's tools to SafeToolSubset and sets PermissionMode to restricted.
func TestCoordinatorRestrictsChildrenPolicy_Apply(t *testing.T) {
	parent := SpawnConfig{}
	child := SpawnConfig{
		Permissions: PermissionsConfig{
			Tools:          []string{"*"},
			PermissionMode: PermissionDefault,
		},
	}

	CoordinatorRestrictsChildrenPolicy{}.Apply(&parent, &child)

	if !reflect.DeepEqual(child.Permissions.Tools, SafeToolSubset) {
		t.Errorf("UC-053: Tools = %v, want SafeToolSubset %v", child.Permissions.Tools, SafeToolSubset)
	}
	if child.Permissions.PermissionMode != PermissionRestricted {
		t.Errorf("UC-053: PermissionMode = %q, want %q", child.Permissions.PermissionMode, PermissionRestricted)
	}
}

// TestCoordinatorRestrictsChildrenPolicy_Sticky (UC-054 building block) —
// after Apply, the policy is in child.SpawnPolicies so it propagates to
// grandchildren.
func TestCoordinatorRestrictsChildrenPolicy_Sticky(t *testing.T) {
	child := SpawnConfig{}
	CoordinatorRestrictsChildrenPolicy{}.Apply(&SpawnConfig{}, &child)

	if !hasPolicy(child.SpawnPolicies, "coordinator-restricts-children") {
		t.Fatalf("UC-054: child.SpawnPolicies missing coordinator-restricts-children policy after Apply (sticky propagation broken)")
	}
}

// TestCoordinatorRestrictsChildrenPolicy_Idempotent (UC-054 negative DoD) —
// applying the policy twice does NOT duplicate the entry in
// child.SpawnPolicies and does NOT change the dimensions further.
func TestCoordinatorRestrictsChildrenPolicy_Idempotent(t *testing.T) {
	child := SpawnConfig{
		Permissions: PermissionsConfig{Tools: []string{"*"}},
	}
	policy := CoordinatorRestrictsChildrenPolicy{}

	policy.Apply(&SpawnConfig{}, &child)
	policy.Apply(&SpawnConfig{}, &child)

	count := 0
	for _, p := range child.SpawnPolicies {
		if p.Name() == "coordinator-restricts-children" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("UC-054 idempotency: policy applied twice → %d entries in SpawnPolicies, want 1", count)
	}
	if !reflect.DeepEqual(child.Permissions.Tools, SafeToolSubset) {
		t.Errorf("UC-054 idempotency: dimensions diverged on second Apply")
	}
}

// ────────────────────────────────────────────────────────────────────────
// TeamLeaderTagsChildrenPolicy
// ────────────────────────────────────────────────────────────────────────

// TestTeamLeaderTagsChildrenPolicy_Apply — Messaging is enabled on child.
func TestTeamLeaderTagsChildrenPolicy_Apply(t *testing.T) {
	child := SpawnConfig{}
	TeamLeaderTagsChildrenPolicy{}.Apply(&SpawnConfig{}, &child)

	if !child.AgentComms.Messaging {
		t.Error("TeamLeaderTagsChildrenPolicy: child.AgentComms.Messaging not enabled after Apply")
	}
}

// TestTeamLeaderTagsChildrenPolicy_NotSticky — policy is NOT appended to
// child.SpawnPolicies (each team-leader spawns its own teammates without
// transitive escalation).
func TestTeamLeaderTagsChildrenPolicy_NotSticky(t *testing.T) {
	child := SpawnConfig{}
	TeamLeaderTagsChildrenPolicy{}.Apply(&SpawnConfig{}, &child)

	if hasPolicy(child.SpawnPolicies, "team-leader-tags-children") {
		t.Error("TeamLeaderTagsChildrenPolicy: should not be sticky (children should not re-tag their own children)")
	}
}

// ────────────────────────────────────────────────────────────────────────
// BuildChildOverride — unified entry point (UC-053 + UC-054)
// ────────────────────────────────────────────────────────────────────────

// TestBuildChildOverride_CoordinatorWorker (UC-053) — the standard delegate
// case: coord parent + worker child → override map carries SafeToolSubset.
func TestBuildChildOverride_CoordinatorWorker(t *testing.T) {
	override := BuildChildOverride(ModeWorker, ModeCoordinator)
	if override == nil {
		t.Fatal("UC-053: BuildChildOverride returned nil for coord+worker")
	}

	sp, ok := override["spawnPermissions"].(map[string]any)
	if !ok {
		t.Fatalf("UC-053: spawnPermissions missing or wrong type in override: %v", override)
	}
	allowed, ok := sp["allowedTools"].([]any)
	if !ok {
		t.Fatalf("UC-053: allowedTools missing in spawnPermissions: %v", sp)
	}
	if len(allowed) != len(SafeToolSubset) {
		t.Errorf("UC-053: allowedTools length = %d, want %d (SafeToolSubset)", len(allowed), len(SafeToolSubset))
	}
	if mode, _ := sp["permissionMode"].(string); mode != string(PermissionRestricted) {
		t.Errorf("UC-053: permissionMode = %q, want %q", mode, PermissionRestricted)
	}
}

// TestBuildChildOverride_NoCoordinator (UC-053 negative — non-coord parent) —
// without a coordinator parent, no escalation is applied. The worker preset
// has wildcard tools, so no restrictive Permissions override is generated.
func TestBuildChildOverride_NoCoordinator(t *testing.T) {
	override := BuildChildOverride(ModeWorker, ModeWorker)

	// Worker preset has Tools: ["*"] — an EXPLICIT allow-all. It MUST serialize
	// to spawnPermissions.allowedTools == ["*"]: the child's SpawnPermissionHook
	// fail-closes on dangerous tools (file-write, bash) when SpawnPermissions is
	// absent, so an allow-all preset that omitted its permissions would silently
	// deny the worker file-write (the bug that blocked the compose worker /
	// agent_as_human child). The wildcard is the permissive intent — it is NOT a
	// restriction, and it must NOT be the coordinator's SafeToolSubset.
	sp, ok := override["spawnPermissions"].(map[string]any)
	if !ok {
		t.Fatalf("worker child must carry explicit allow-all spawnPermissions, got override: %v", override)
	}
	allowed, _ := sp["allowedTools"].([]any)
	if len(allowed) != 1 || allowed[0] != "*" {
		t.Errorf("worker child allowedTools must be the wildcard [*] (allow-all), got %v", allowed)
	}
	if _, hasMode := sp["permissionMode"]; hasMode {
		t.Errorf("worker (PermissionDefault) must not emit a permissionMode, got %v", sp["permissionMode"])
	}
}

// TestBuildChildOverride_GrandchildEscalation (UC-054) — sticky propagation:
// a worker spawned by a coordinator carries the coordinator-restricts policy
// (via Apply's sticky behavior). When that worker spawns its own grandchild,
// the grandchild must ALSO be restricted.
//
// Strategy: simulate the two-level chain by applying the policy twice (once
// per spawn level) and verify the grandchild ends up restricted.
func TestBuildChildOverride_GrandchildEscalation(t *testing.T) {
	// Level 1: coord spawns worker (W1).
	// W1 starts as a worker preset; coord's policy fires.
	w1 := MustResolvePreset(ModeWorker)
	coord := MustResolvePreset(ModeCoordinator)
	for _, p := range coord.SpawnPolicies {
		p.Apply(&coord, &w1)
	}

	// W1 should now be restricted AND carry the policy stickily.
	if !reflect.DeepEqual(w1.Permissions.Tools, SafeToolSubset) {
		t.Fatalf("UC-054 setup: W1 not restricted after coord→worker spawn")
	}
	if !hasPolicy(w1.SpawnPolicies, "coordinator-restricts-children") {
		t.Fatalf("UC-054 setup: W1.SpawnPolicies missing sticky policy")
	}

	// Level 2: W1 spawns worker (W2). W2 starts fresh from preset; W1's
	// (now-sticky) policy fires.
	w2 := MustResolvePreset(ModeWorker)
	for _, p := range w1.SpawnPolicies {
		p.Apply(&w1, &w2)
	}

	// W2 must be restricted just like W1 — UC-054.
	if !reflect.DeepEqual(w2.Permissions.Tools, SafeToolSubset) {
		t.Errorf("UC-054: grandchild W2 not restricted (sticky propagation failed). Tools = %v", w2.Permissions.Tools)
	}
	if w2.Permissions.PermissionMode != PermissionRestricted {
		t.Errorf("UC-054: grandchild PermissionMode = %q, want %q", w2.Permissions.PermissionMode, PermissionRestricted)
	}
	if !hasPolicy(w2.SpawnPolicies, "coordinator-restricts-children") {
		t.Errorf("UC-054: sticky policy did not carry to grandchild — W3 would escape restriction")
	}
}

// TestBuildChildOverride_UnknownMode — invalid mode names return nil
// (no panic, no spurious override).
func TestBuildChildOverride_UnknownMode(t *testing.T) {
	override := BuildChildOverride("definitely-not-a-real-mode", "")
	if override != nil {
		t.Errorf("invalid child mode produced override: %v", override)
	}
}

// TestBuildChildOverride_IsolatedMode_EmitsWorkspaceStrategy locks in that
// presets with `Workspace.Strategy` set surface it in the serialized override
// map at `workspacePaths.strategy`. Before this change serializeOverride only
// emitted readPaths/writePaths under workspacePaths, leaving `isolated` and
// `fork-isolated` modes indistinguishable from `worker` / `fork` at the
// __spawn_dimensions snapshot layer. Closes the mode-fingerprint observability
// gap.
func TestBuildChildOverride_IsolatedMode_EmitsWorkspaceStrategy(t *testing.T) {
	override := BuildChildOverride(ModeIsolated, ModeWorker)
	if override == nil {
		t.Fatal("isolated mode produced no override map")
	}
	wp, ok := override["workspacePaths"].(map[string]any)
	if !ok {
		t.Fatalf("isolated mode should emit workspacePaths; got override = %v", override)
	}
	if strat, _ := wp["strategy"].(string); strat != "git_worktree" {
		t.Errorf("workspacePaths.strategy = %q, want %q (from presetIsolated.Workspace.Strategy)", strat, "git_worktree")
	}
}

// TestBuildChildOverride_WorkerMode_NoWorkspaceStrategy is the negative of the
// previous test: worker preset has Workspace.Strategy="" so workspacePaths
// should be absent from the override entirely (no key emitted with empty
// strategy and wildcard read/write paths).
func TestBuildChildOverride_WorkerMode_NoWorkspaceStrategy(t *testing.T) {
	override := BuildChildOverride(ModeWorker, ModeWorker)
	if override == nil {
		return // no override is the expected outcome
	}
	if _, present := override["workspacePaths"]; present {
		t.Errorf("worker mode should not emit workspacePaths; got %v", override["workspacePaths"])
	}
}

// TestSchedulingFieldsFromMode — the migrated requestFieldsFromMode helper
// (formerly in taskplugin) returns the right scheduling fields per mode.
func TestSchedulingFieldsFromMode(t *testing.T) {
	// Worker is foreground — no fire-and-forget.
	_, fire := SchedulingFieldsFromMode(ModeWorker)
	if fire {
		t.Error("worker mode reports fire_and_forget=true")
	}

	// Unknown mode returns zero-values, no panic.
	maxD, fire2 := SchedulingFieldsFromMode("not-a-mode")
	if maxD != 0 || fire2 {
		t.Errorf("unknown mode returned non-zero defaults: maxD=%d, fire=%v", maxD, fire2)
	}
}
