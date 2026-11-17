// SPDX-License-Identifier: Apache-2.0

package presets

// SpawnPolicy is a rule that fires AT spawn time and mutates the child's
// SpawnConfig based on the parent's state. Modes carry their canonical
// policies via SpawnConfig.SpawnPolicies; the spawner applies them when
// constructing a child via BuildChildOverride.
//
// Apply may also append the policy (or a derivative) to child.SpawnPolicies
// to make the policy "sticky" — propagating to grandchildren without a
// special transitive rule. CoordinatorRestrictsChildrenPolicy works this
// way; that's how UC-054 (transitive escalation) closes by construction.
//
// Policies must be idempotent: applying the same policy twice should
// produce the same result as applying once. This guards against double-
// application during sticky propagation chains.
type SpawnPolicy interface {
	// Name returns a stable identifier used for diagnostics, lint rules,
	// and idempotency checks (avoiding duplicate policy entries in
	// child.SpawnPolicies).
	Name() string

	// Apply mutates the child's SpawnConfig based on the parent's state.
	// Implementations should be idempotent.
	Apply(parent, child *SpawnConfig)
}

// SafeToolSubset is the restricted toolset for coordinator-spawned children
// (per kts-task-engine.md and the agent-orchestration-architecture ADR).
// Renamed from taskplugin.CoordinatorWorkerTools — same content; single home
// in the presets package as part of the spawn-policy formalization.
// Tool names are kebab (the registered names); a snake entry matches no
// real tool — see runtime.SafeToolSubset for the same constraint.
var SafeToolSubset = []string{"file-read", "file-write", "file-edit", "glob", "complete"}

// hasPolicy returns true if a policy with the given name already exists in
// the slice. Used by sticky policies to avoid duplicate entries.
func hasPolicy(policies []SpawnPolicy, name string) bool {
	for _, p := range policies {
		if p.Name() == name {
			return true
		}
	}
	return false
}

// policyRegistry maps a policy name to a zero-value instance so the runtime
// can resurrect SpawnPolicy values from their serialized names (the wire-
// format used by ConfigOverride.spawnPolicies — see serializeOverride +
// PolicyByName below). Every policy type must register itself here.
var policyRegistry = map[string]SpawnPolicy{
	"coordinator-restricts-children": CoordinatorRestrictsChildrenPolicy{},
	"team-leader-tags-children":      TeamLeaderTagsChildrenPolicy{},
}

// PolicyByName returns the registered SpawnPolicy for the given name, or
// nil if unknown. The runtime uses this when deserializing parent policies
// off ConfigOverride.spawnPolicies into runtime-applicable instances.
func PolicyByName(name string) SpawnPolicy {
	return policyRegistry[name]
}

// PoliciesByName resolves a list of names; unknown names are dropped with
// no error (idempotent across schema evolution — a future policy that
// hasn't been added yet doesn't crash old binaries that see its name on
// the wire).
func PoliciesByName(names []string) []SpawnPolicy {
	out := make([]SpawnPolicy, 0, len(names))
	for _, n := range names {
		if p := PolicyByName(n); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// ── Policy implementations ────────────────────────────────────────────

// CoordinatorRestrictsChildrenPolicy is the spawn policy carried by the
// `coordinator` mode preset (UC-053). It restricts spawned children to
// SafeToolSubset and the `restricted` PermissionMode. The policy is
// **sticky**: it appends itself to child.SpawnPolicies so the restriction
// propagates transitively to grandchildren (UC-054).
//
// Without sticky propagation, a coordinator would restrict its workers
// but the workers' grandchildren would inherit unrestricted permissions —
// the original threat-T1 escalation gap. Stickiness closes UC-054 by
// construction: every spawn from a restricted ancestor stays restricted.
type CoordinatorRestrictsChildrenPolicy struct{}

// Name returns the canonical policy identifier.
func (CoordinatorRestrictsChildrenPolicy) Name() string {
	return "coordinator-restricts-children"
}

// Apply restricts the child to SafeToolSubset and marks the child sticky
// so grandchildren also get restricted. Idempotent: applying twice yields
// the same child config and a single policy entry.
func (p CoordinatorRestrictsChildrenPolicy) Apply(parent, child *SpawnConfig) {
	// Restrict permissions. Idempotent — re-applying the same values is a no-op.
	child.Permissions.Tools = SafeToolSubset
	child.Permissions.PermissionMode = PermissionRestricted

	// Sticky: child carries the policy so grandchildren are also restricted.
	if !hasPolicy(child.SpawnPolicies, p.Name()) {
		child.SpawnPolicies = append(child.SpawnPolicies, p)
	}
}

// TeamLeaderTagsChildrenPolicy is the spawn policy carried by the
// `team-leader` mode preset. It enables the child's mailbox messaging
// so the child can participate in the team's send-message / check-messages
// flow. NOT sticky — each team-leader manages its own teammates; children
// don't transitively re-tag their own children.
//
// Note: full team-name injection (TeamName + TeammateName fields) is
// handled by orchestration-side wiring at spawn-teammate time. This
// policy locks in the AgentComms.Messaging contract at the SpawnConfig
// level so the child's runtime is configured correctly.
type TeamLeaderTagsChildrenPolicy struct{}

// Name returns the canonical policy identifier.
func (TeamLeaderTagsChildrenPolicy) Name() string {
	return "team-leader-tags-children"
}

// Apply enables messaging on the child. NOT sticky.
func (TeamLeaderTagsChildrenPolicy) Apply(parent, child *SpawnConfig) {
	child.AgentComms.Messaging = true
}

// ── Unified entry point ───────────────────────────────────────────────

// BuildChildOverride is the unified entry point for both delegate-spawn
// (taskplugin) and workflow-spawn (LocalSpawner) paths. It:
//
//  1. Resolves the child's mode preset.
//  2. Resolves the parent's mode preset (if parentMode != "").
//  3. Applies each parent.SpawnPolicies to the child config — which mutates
//     the child's dimensions AND may append sticky policies to
//     child.SpawnPolicies for transitive propagation (UC-054).
//  4. Serializes the child's restrictive dimensions into a ConfigOverride
//     map suitable for TaskRunRequest.ConfigOverride or
//     TaskExecRequest.ConfigOverride.
//
// Returns nil if neither child preset nor parent policies produced any
// restrictive overrides (caller can pass through unchanged).
//
// This is the function that makes UC-053 close by construction: every
// spawn path calls BuildChildOverride with a real parentMode, so the
// sec-1 / threat-T1 bug (workflow-spawn bypassing mode coercion) becomes
// structurally impossible.
func BuildChildOverride(childMode, parentMode Mode) map[string]any {
	return BuildChildOverrideWithPolicies(childMode, parentMode, nil)
}

// BuildChildOverrideWithPolicies extends BuildChildOverride with explicit
// `extraParentPolicies` — policies the parent carries AT RUNTIME (not just
// the ones implied by its mode preset). This closes UC-054's transitive
// case:
//
//	coord (mode=coordinator) → spawns mid (mode=worker, sticky-policy=[coord-restricts])
//	mid (mode=worker)         → spawns grandchild (mode=worker)
//
// Without runtime policies, the mid→grandchild call resolves the worker
// preset (no policies) and the grandchild escapes restriction. With this
// function, mid passes its own runtime SpawnPolicies forward; the sticky
// policy fires on grandchild and the restriction propagates.
//
// extraParentPolicies are applied AFTER the parent preset's policies. A
// policy that appears in both lists is applied once (idempotency is the
// SpawnPolicy contract).
func BuildChildOverrideWithPolicies(childMode, parentMode Mode, extraParentPolicies []SpawnPolicy) map[string]any {
	childCfg, err := ResolvePreset(childMode, nil)
	if err != nil {
		return nil
	}
	applied := map[string]bool{}
	apply := func(parent SpawnConfig, policies []SpawnPolicy) {
		for _, policy := range policies {
			if policy == nil || applied[policy.Name()] {
				continue
			}
			applied[policy.Name()] = true
			policy.Apply(&parent, &childCfg)
		}
	}
	if parentMode != "" {
		parentCfg, perr := ResolvePreset(parentMode, nil)
		if perr == nil {
			apply(parentCfg, parentCfg.SpawnPolicies)
		}
	}
	if len(extraParentPolicies) > 0 {
		// Synthetic parent — only policies matter; the parent SpawnConfig
		// fields are not read by sticky policies (Apply uses parent only for
		// reference, never for inheriting fields).
		apply(SpawnConfig{}, extraParentPolicies)
	}
	return SerializeOverride(childCfg)
}

// SchedulingFieldsFromMode returns request-level scheduling fields
// (MaxDurationS, FireAndForget) for the resolved mode preset. These don't
// belong in ConfigOverride — they're TaskRunRequest fields. Migrated from
// taskplugin.requestFieldsFromMode to consolidate preset-derived helpers.
func SchedulingFieldsFromMode(mode Mode) (maxDurationS int, fireAndForget bool) {
	cfg, err := ResolvePreset(mode, nil)
	if err != nil {
		return 0, false
	}
	maxDurationS = cfg.Scheduling.Limits.MaxDurationS
	fireAndForget = cfg.Scheduling.Mode == SchedulingFireAndForget
	return
}

// SerializeOverride converts a SpawnConfig into the ConfigOverride map
// shape expected by TaskRunRequest.ConfigOverride / TaskExecRequest.ConfigOverride.
// Mirrors what taskplugin.configOverrideFromModeWithParent did before the
// refactor; only injects fields when they would actually constrain the
// child (avoids bloating the override map with default values).
func SerializeOverride(cfg SpawnConfig) map[string]any {
	override := map[string]any{}

	// Permissions. A preset that declares Tools — INCLUDING the explicit
	// allow-all "*" — expresses an intent that MUST survive serialization. The
	// child's SpawnPermissionHook fail-closes on dangerous tools (file-write,
	// bash, …) whenever the child arrives with zero SpawnPermissions, so
	// omitting an allow-all preset's permissions silently turns the most
	// "permissive" preset into the most restrictive outcome — which is exactly
	// why a delegated worker (e.g. the compose wizard's file-writing worker, or
	// an agent_as_human child) was denied file-write. Emit AllowedTools whenever
	// Tools is set: ["*"] is a valid explicit allow the hook reads as
	// wildcard-allow AND that opts the child out of the fail-closed default.
	// Only a SpawnConfig that declares NO permission intent at all stays absent
	// (and correctly fail-closes).
	perms := cfg.Permissions
	if len(perms.Tools) > 0 || len(perms.DisallowedTools) > 0 ||
		(perms.PermissionMode != "" && perms.PermissionMode != PermissionDefault) {
		sp := map[string]any{}
		if len(perms.Tools) > 0 {
			sp[OverrideKeyAllowedTools] = toAnyList(perms.Tools)
		}
		if len(perms.DisallowedTools) > 0 {
			sp[OverrideKeyDisallowedTools] = toAnyList(perms.DisallowedTools)
		}
		if perms.PermissionMode != "" && perms.PermissionMode != PermissionDefault {
			sp[OverrideKeyPermissionMode] = string(perms.PermissionMode)
		}
		override[OverrideKeySpawnPermissions] = sp
	}

	// WorkspacePaths — inject if the workspace config has read/write path
	// restrictions OR a non-default strategy (git_worktree, etc.) that's
	// observable in the journal. The agent runtime decodes this into the
	// WorkspacePaths struct which only reads readPaths/writePaths/URLAllowlist;
	// strategy is informational (audit-trail / spawn-fingerprint) and is
	// silently ignored by the runtime decoder.
	ws := cfg.Context.Workspace
	hasReadRestriction := len(ws.ReadPaths) > 0 && !isAllWildcardWorkspace(ws.ReadPaths)
	hasWriteRestriction := len(ws.WritePaths) > 0 && !isAllWildcardWorkspace(ws.WritePaths)
	hasStrategy := ws.Strategy != ""
	if hasReadRestriction || hasWriteRestriction || hasStrategy {
		wp := map[string]any{}
		if len(ws.ReadPaths) > 0 {
			wp[OverrideKeyReadPaths] = toAnyList(ws.ReadPaths)
		}
		if len(ws.WritePaths) > 0 {
			wp[OverrideKeyWritePaths] = toAnyList(ws.WritePaths)
		}
		if hasStrategy {
			wp[OverrideKeyStrategy] = ws.Strategy
		}
		override[OverrideKeyWorkspacePaths] = wp
	}

	// HistoryMode — record the dimension value for explicit declaration.
	if cfg.Context.History != "" {
		override[OverrideKeyHistoryMode] = string(cfg.Context.History)
	}

	// IsolationMode — carry the isolation dimension to the child.
	// Only inject if non-default (in_process is the default behavior).
	if cfg.Isolation.Mode != "" && cfg.Isolation.Mode != IsolationInProcess {
		override[OverrideKeyIsolationMode] = string(cfg.Isolation.Mode)
	}

	// Resource limits — MaxBudgetUSD threads through to AgentRunRequest.
	if cfg.Scheduling.Limits.MaxBudgetUSD > 0 {
		override[OverrideKeyMaxBudgetUSD] = cfg.Scheduling.Limits.MaxBudgetUSD
	}

	// UserComms.Input — carry the preset default through to the agent
	// loop. The factory finalizes via EffectiveUserCommsInput(default,
	// override): an explicit value here is the upstream signal that
	// wins over the child's own preset default.
	// We always emit it (even for the InputInbox preset default) because
	// the agent loop needs an explicit value to route ask_user calls —
	// an empty field would force every loop call site to re-derive the
	// default, fragmenting the resolution rule.
	if cfg.UserComms.Input != "" {
		override[OverrideKeyUserCommsInput] = string(cfg.UserComms.Input)
	}

	// Sticky policies — serialize the child's accumulated SpawnPolicies so
	// the runtime can resurrect them at the next spawn level. Closes UC-054's
	// transitive case: mid-workers carry the coordinator's policy forward
	// to their grandchildren via BuildChildOverrideWithPolicies.
	if len(cfg.SpawnPolicies) > 0 {
		names := make([]any, 0, len(cfg.SpawnPolicies))
		for _, p := range cfg.SpawnPolicies {
			names = append(names, p.Name())
		}
		override[OverrideKeySpawnPolicies] = names
	}

	if len(override) == 0 {
		return nil
	}
	return override
}

func isAllWildcardWorkspace(list []string) bool {
	if len(list) == 0 {
		return true
	}
	for _, p := range list {
		if p == "*" || p == "**" {
			return true
		}
	}
	return false
}

func toAnyList(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}
