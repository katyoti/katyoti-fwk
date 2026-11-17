// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// Tool classification used by this hook is now metadata-driven via the
// capability registry in toolcap.go (R3):
//   - Read tools (CapReadsPaths)       — gated against ReadPaths
//   - Write tools (CapWritesPaths)     — gated against WritePaths
//   - Shell tools (CapExecutesShell)   — denied under restrictive paths
//                                        (free-form input can't be path-gated;
//                                        proper fix is per-tool sandbox per
//                                        ms09 `bash-sandbox-cap-bt-05`)
//   - Fetch tools (CapIssuesNetworkRequests) — URL allowlist + private-net deny
//
// file_edit declares BOTH CapReadsPaths and CapWritesPaths. The hook checks
// writes first so file_edit is gated against the (typically tighter)
// WritePaths list.

// pathInputKeys are the keys we check on tool input maps for a path argument.
// Order matters: we check workspace_path first (more common), then absolute_path,
// then file_path / path as fallbacks; then deck-export's `output` (where it
// writes) before its `file` (the deck it reads, beside which it writes when
// no output is given).
var pathInputKeys = []string{"workspace_path", "absolute_path", "file_path", "path", "output", "file"}

// WorkspacePathHook enforces per-spawn path-level access restrictions for file
// tools. Runs in the OnPreToolExec chain before the user-curated permission gate.
//
// Enforcement:
//   - Read tools (file_read, glob, grep): check path against ReadPaths
//   - Write tools (file_write, file_edit): check path against WritePaths
//   - Other tools: pass through (no path enforcement)
//   - Empty/nil list: no restriction
//   - "*" or "**" entry: wildcard, allow all
//   - Otherwise: prefix match on the path argument
type WorkspacePathHook struct {
	paths *agentapi.WorkspacePaths
}

// NewWorkspacePathHook creates a hook from the per-spawn workspace path config.
// Returns nil if paths is nil or both lists are empty/wildcard AND no
// URL allowlist is set. (When URLAllowlist alone is set, the hook is needed
// to gate fetch tools even with unrestricted paths.)
func NewWorkspacePathHook(paths *agentapi.WorkspacePaths) *WorkspacePathHook {
	if paths == nil {
		return nil
	}
	// All three dimensions unrestricted → no-op hook, skip allocation.
	if isUnrestrictedPaths(paths.ReadPaths) && isUnrestrictedPaths(paths.WritePaths) && len(paths.URLAllowlist) == 0 {
		return nil
	}
	return &WorkspacePathHook{paths: paths}
}

// OnPreToolExec matches the agentapi.PluginHooks.OnPreToolExec signature.
func (h *WorkspacePathHook) OnPreToolExec(
	_ agentapi.LoopContext,
	toolName string,
	input map[string]any,
) (*agentapi.ToolDecision, error) {
	if h == nil || h.paths == nil {
		return nil, nil
	}

	// Fetch tools (rest_api, web_fetch) are gated by URLAllowlist + the
	// always-on private-network deny. Closes UC-058's URL/SSRF half +
	// threat-T2-critical. Fires before the bash path because fetch tools
	// CAN be safely gated at the URL layer; they don't need the wholesale
	// deny that bash does.
	if HasCapability(toolName, CapIssuesNetworkRequests) {
		if decision := fetchToolDecision(toolName, input, h.paths); decision != nil {
			return decision, nil
		}
		// fetchToolDecision passed → URL is allowed; fall through to the
		// permission-gate chain.
		return nil, nil
	}

	// Shell tools (CapExecutesShell) cannot be safely gated by per-input-key
	// path matching — their input is free-form. Deny them when the hook is
	// active (= WorkspacePaths is restrictive). The proper fix is a
	// command-level sandbox per the tool's spec (CAP-BT-05 for bash); see
	// follow-up `bash-sandbox-cap-bt-05` in ms09. UC-058 (partial closure).
	if HasCapability(toolName, CapExecutesShell) {
		reason := fmt.Sprintf(
			"tool %q is denied under restrictive WorkspacePaths: its input "+
				"shape can't be path-gated and the per-tool sandbox is not "+
				"yet implemented. Override by widening WorkspacePaths at "+
				"spawn time, or wait for the per-tool sandbox to land.",
			toolName,
		)
		emitAuditDeny(AuditEvent{Hook: "workspace_path", Tool: toolName, Reason: reason, DeniedValue: toolName})
		return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
	}

	// Determine which list applies based on tool capabilities. CapWritesPaths
	// is checked first so file_edit (declares both Reads + Writes) lands on
	// the typically tighter WritePaths list.
	var allowed []string
	switch {
	case HasCapability(toolName, CapWritesPaths):
		allowed = h.paths.WritePaths
	case HasCapability(toolName, CapReadsPaths):
		allowed = h.paths.ReadPaths
	default:
		// Not a path-aware tool — no enforcement.
		return nil, nil
	}

	if isUnrestrictedPaths(allowed) {
		return nil, nil
	}

	// Extract path from the tool input.
	path := extractPath(input)
	if path == "" {
		// No path field found — let the tool handle the missing input itself.
		return nil, nil
	}

	if !pathAllowed(path, allowed) {
		reason := fmt.Sprintf("path %q is not in the spawn's allowed paths for %s", path, toolName)
		emitAuditDeny(AuditEvent{Hook: "workspace_path", Tool: toolName, Reason: reason, DeniedValue: path})
		return &agentapi.ToolDecision{Decision: agentapi.Deny, Reason: reason}, nil
	}

	return nil, nil
}

// isUnrestrictedPaths returns true if the list is empty or contains the wildcard.
func isUnrestrictedPaths(list []string) bool {
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

// pathAllowed returns true if input matches any prefix in allowed after
// normalization AND symlink resolution. Two defence layers:
//
//  1. Textual: ".." segments are rejected outright. Catches the common
//     traversal vector cheaply (UC-057 / threat-T2-second first half).
//  2. Symlink-resolution: `filepath.EvalSymlinks` is applied to both the
//     input and each allowed prefix; the resolved paths are then prefix-
//     compared. Catches the "plant a symlink inside the workspace pointing
//     to /etc" attack (R2 / audit high-finding-1 second half). When the
//     input doesn't exist yet (e.g. file_write to a new path), the parent
//     directory is resolved instead.
//
// Allowed-list entries are written as POSIX-style strings in YAML; we use
// stdlib `path` for the textual normalization step so hook behaviour is
// platform-uniform there. For the resolution step we use `filepath` (OS-
// aware) since symlink resolution is intrinsically a filesystem op.
func pathAllowed(input string, allowed []string) bool {
	if containsTraversal(input) {
		return false
	}
	norm := normalizePath(input)
	if norm == "" || norm == "." {
		return false
	}
	resolvedInput := resolveSymlinks(input)
	for _, p := range allowed {
		// Strip trailing /** or /* — treat as prefix.
		clean := strings.TrimSuffix(p, "/**")
		clean = strings.TrimSuffix(clean, "/*")
		clean = strings.TrimSuffix(clean, "/")
		if clean == "" {
			continue
		}
		cleanNorm := normalizePath(clean)
		if cleanNorm == "" || cleanNorm == "." {
			continue
		}
		// Textual prefix match — fast path, also covers wildcards expressed
		// via the entry pattern (already trimmed above).
		textualMatch := norm == cleanNorm || strings.HasPrefix(norm, cleanNorm+"/")
		if !textualMatch {
			continue
		}
		// Symlink fail-closed: if either side fails to resolve to an
		// allowed prefix (because the symlink lands outside), deny. When
		// both resolve cleanly under the same prefix, allow. When the
		// resolution step is unavailable (test fixtures with empty paths,
		// non-existent dirs that don't resolve), we keep the textual
		// match — the `..` rejection above already covers the common case.
		if resolvedInput == "" {
			return true
		}
		resolvedPrefix := resolveSymlinks(clean)
		if resolvedPrefix == "" {
			return true
		}
		rIn := normalizePath(resolvedInput)
		rPre := normalizePath(resolvedPrefix)
		if rIn == rPre || strings.HasPrefix(rIn, rPre+"/") {
			return true
		}
		// Textual match but symlink landed outside the allowed prefix —
		// fail-closed (R2 / high-finding-1 second half).
		return false
	}
	return false
}

// resolveSymlinks returns the canonical path of `p` with all symlinks
// resolved. For paths that don't exist yet (e.g. a write target), falls
// back to resolving the parent directory + appending the basename. Returns
// "" if the path can't be resolved at all — the caller treats that as
// "no symlink reachable, keep the textual match".
func resolveSymlinks(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	// File doesn't exist yet — resolve the parent dir + reattach basename.
	dir := filepath.Dir(abs)
	if dir == abs || dir == "" {
		return ""
	}
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(resolved, filepath.Base(abs))
	}
	return ""
}

// normalizePath converts Windows-style separators to POSIX and applies
// path.Clean. Result uses forward slashes only.
func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

// containsTraversal reports whether p contains a literal ".." segment in
// either its raw form or after backslash-to-slash conversion. Used to refuse
// inputs with traversal intent regardless of whether path.Clean would have
// folded them inside an allowed prefix.
func containsTraversal(p string) bool {
	if p == "" {
		return false
	}
	canonical := strings.ReplaceAll(p, "\\", "/")
	for _, seg := range strings.Split(canonical, "/") {
		if seg == ".." {
			return true
		}
	}
	return false
}

// extractPath returns the first non-empty path-like field from the tool input.
func extractPath(input map[string]any) string {
	for _, key := range pathInputKeys {
		if v, ok := input[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// ── Plugin wrapper ────────────────────────────────────────────────────

// WorkspacePathPlugin wraps WorkspacePathHook as an AgentPlugin so it can be
// added to the plugin set and picked up by BuildHooks() automatically.
type WorkspacePathPlugin struct {
	hook *WorkspacePathHook
}

func newWorkspacePathPlugin(paths *agentapi.WorkspacePaths) *WorkspacePathPlugin {
	hook := NewWorkspacePathHook(paths)
	if hook == nil {
		return nil
	}
	return &WorkspacePathPlugin{hook: hook}
}

func (p *WorkspacePathPlugin) Name() string                 { return "workspace_paths" }
func (p *WorkspacePathPlugin) SystemMessage() string        { return "" }
func (p *WorkspacePathPlugin) ToolDefs() []agentapi.ToolDef { return nil }
func (p *WorkspacePathPlugin) HandleToolCall(_ context.Context, _ string, _ json.RawMessage) (*agentapi.ToolCallResult, error) {
	return nil, fmt.Errorf("workspace_paths plugin does not handle tool calls")
}
func (p *WorkspacePathPlugin) Hooks() agentapi.PluginHooks {
	return agentapi.PluginHooks{
		OnPreToolExec: p.hook.OnPreToolExec,
	}
}
func (p *WorkspacePathPlugin) Stable() bool { return true }

// AttachWorkspacePaths adds a WorkspacePathPlugin to the plugin set if paths
// is non-nil and restrictive. Called from the agent factory during spawn.
func AttachWorkspacePaths(plugins *PluginSet, paths *agentapi.WorkspacePaths) *WorkspacePathPlugin {
	plugin := newWorkspacePathPlugin(paths)
	if plugin == nil {
		return nil
	}
	plugins.Add(plugin)
	return plugin
}
