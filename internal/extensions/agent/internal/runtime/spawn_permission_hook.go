// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// SafeToolSubset is the restricted set of tools available to workers spawned
// by a coordinator, or to any agent with PermissionMode=restricted.
// See (modules/tasks/decisions/OD-037-how-does-a-caller-say-how-a-child-runs-beside.cwm.md) (Dimension 3).
// Tool names are kebab (the registered names). A snake entry here matches
// no real tool, so a restricted worker calling file-read/file-write/file-edit
// would be denied — only glob/complete (single-word, snake==kebab) would pass.
var SafeToolSubset = []string{
	"file-read",
	"file-write",
	"file-edit",
	"glob",
	"complete",
}

// Dangerous-tool detection lives in toolcap.go: a tool is dangerous when its
// registered capabilities include any of {CapWritesPaths, CapExecutesShell,
// CapIssuesNetworkRequests, CapSpawnsChild}. R3 replaced R4's hardcoded
// `dangerousToolKinds` map with capability metadata so a single declaration
// per tool propagates to every gate. See toolcap.go::IsDangerousTool.

// SpawnPermissionHook enforces per-spawn tool access restrictions from the
// parent (via AgentRunRequest.SpawnPermissions). It runs in the OnPreToolExec
// chain, before the user-curated permission profile gate.
//
// Enforcement order (first match wins):
//  1. DisallowedTools — if tool is in this list, Deny
//  2. AllowedTools (non-empty, non-wildcard) — if tool NOT in list, Deny
//  3. PermissionMode="restricted" — if tool not in SafeToolSubset, Deny
//  4. Fail-closed default — if AllowAll is false AND no explicit AllowedTools
//     was set (the caller did not declare an intent), deny tools in
//     dangerousToolKinds. Closes the historic fail-open gap where a zero-
//     value SpawnPermissions{} let every tool through.
//  5. PermissionMode="bubble" — deferred (R9 owns this)
//  6. Otherwise Allow (profile gate will further evaluate)
//
// The hook does NOT replace the permission profile system. User-curated rules
// still apply after the hook returns Allow.
type SpawnPermissionHook struct {
	perms *agentapi.SpawnPermissions
}

// NewSpawnPermissionHook creates a hook from the per-spawn permission config.
// A nil perms is treated as a zero-value SpawnPermissions{} so the fail-closed
// gate fires (R4): any spawn that forgets to populate permissions inherits
// the deny-dangerous-tools default rather than silently bypassing enforcement.
func NewSpawnPermissionHook(perms *agentapi.SpawnPermissions) *SpawnPermissionHook {
	if perms == nil {
		perms = &agentapi.SpawnPermissions{}
	}
	return &SpawnPermissionHook{perms: perms}
}

// OnPreToolExec matches the agentapi.PluginHooks.OnPreToolExec signature.
func (h *SpawnPermissionHook) OnPreToolExec(
	_ agentapi.LoopContext,
	toolName string,
	_ map[string]any,
) (*agentapi.ToolDecision, error) {
	if h == nil || h.perms == nil {
		return nil, nil
	}

	// 1. Deny list takes precedence.
	if contains(h.perms.DisallowedTools, toolName) {
		reason := fmt.Sprintf("tool %q is in the spawn deny list", toolName)
		emitAuditDeny(AuditEvent{Hook: "spawn_permission", Tool: toolName, Reason: reason, DeniedValue: toolName})
		return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
	}

	// 2. Allow list — if set, tool must be present (["*"] = wildcard, allow all).
	if len(h.perms.AllowedTools) > 0 && !isWildcard(h.perms.AllowedTools) {
		if !contains(h.perms.AllowedTools, toolName) {
			reason := fmt.Sprintf("tool %q is not in the spawn allow list", toolName)
			emitAuditDeny(AuditEvent{Hook: "spawn_permission", Tool: toolName, Reason: reason, DeniedValue: toolName})
			return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
		}
	}

	// 3. Restricted mode — only SafeToolSubset allowed.
	if h.perms.PermissionMode == "restricted" {
		if !contains(SafeToolSubset, toolName) {
			reason := fmt.Sprintf("tool %q not in safe subset (restricted mode)", toolName)
			emitAuditDeny(AuditEvent{Hook: "spawn_permission", Tool: toolName, Reason: reason, DeniedValue: toolName})
			return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
		}
	}

	// 4. Fail-closed default. When the caller did NOT express an intent
	// (no AllowedTools, no DisallowedTools-only-match, no restricted mode)
	// AND AllowAll is false, deny tools in the dangerous-kind list. This
	// closes the historic fail-open gap: a zero-value SpawnPermissions{}
	// (or nil that was promoted to zero-value in the constructor) used to
	// allow every tool; now it denies the dangerous ones.
	//
	// Skip when AllowAll is explicitly set OR when AllowedTools opts the
	// caller out (a non-wildcard AllowedTools that already passed step 2
	// means the caller listed the tool — respect their intent).
	if !h.perms.AllowAll &&
		len(h.perms.AllowedTools) == 0 &&
		h.perms.PermissionMode != "restricted" {
		if IsDangerousTool(toolName) {
			reason := fmt.Sprintf("tool %q denied by fail-closed default (no SpawnPermissions.AllowAll); declare AllowedTools, PermissionMode, or AllowAll explicitly", toolName)
			emitAuditDeny(AuditEvent{Hook: "spawn_permission", Tool: toolName, Reason: reason, DeniedValue: toolName})
			return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
		}
	}

	// 5. Bubble mode — rejected at attach time by factory.CreateMachine (R9).
	// If we ever reach this point with PermissionMode == "bubble" it means
	// the factory was bypassed; defensive deny rather than the historic
	// silent-allow no-op. The ProtocolPermission round-trip implementation
	// is a tier-2 follow-up.
	if h.perms.PermissionMode == "bubble" {
		reason := fmt.Sprintf("tool %q denied: PermissionMode 'bubble' is not yet implemented (tier-2)", toolName)
		emitAuditDeny(AuditEvent{Hook: "spawn_permission", Tool: toolName, Reason: reason, DeniedValue: toolName})
		return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
	}

	// 6. Allow — profile gate will decide further.
	return nil, nil
}

// contains returns true if s is present in list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// isWildcard returns true if list is exactly ["*"] (match-all).
func isWildcard(list []string) bool {
	return len(list) == 1 && list[0] == "*"
}

// SpawnPermissionPlugin wraps SpawnPermissionHook as an AgentPlugin so it can
// be added to the plugin set and picked up by BuildHooks() automatically.
type SpawnPermissionPlugin struct {
	hook *SpawnPermissionHook
}

// newSpawnPermissionPlugin always returns a plugin; nil perms is promoted to
// a zero-value SpawnPermissions{} so the fail-closed default gate fires.
func newSpawnPermissionPlugin(perms *agentapi.SpawnPermissions) *SpawnPermissionPlugin {
	return &SpawnPermissionPlugin{hook: NewSpawnPermissionHook(perms)}
}

func (p *SpawnPermissionPlugin) Name() string                 { return "spawn_permissions" }
func (p *SpawnPermissionPlugin) SystemMessage() string        { return "" }
func (p *SpawnPermissionPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *SpawnPermissionPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("spawn_permissions plugin does not handle tool calls")
}
func (p *SpawnPermissionPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{
		OnPreToolExec: p.hook.OnPreToolExec,
	}
}
func (p *SpawnPermissionPlugin) Stable() bool { return true }

// AttachSpawnPermissions adds a SpawnPermissionPlugin to the plugin set.
// Called from the agent factory during spawn — always attaches so the fail-
// closed default gate fires even when the caller passes nil. Callers that
// genuinely need an unrestricted top-level run must set
// SpawnPermissions{AllowAll: true} explicitly.
func AttachSpawnPermissions(plugins *PluginSet, perms *agentapi.SpawnPermissions) *SpawnPermissionPlugin {
	plugin := newSpawnPermissionPlugin(perms)
	plugins.Add(plugin)
	return plugin
}
