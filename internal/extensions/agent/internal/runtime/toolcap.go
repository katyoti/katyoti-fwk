// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"strings"
	"sync"
)

// Capability is a tool's declared kind of side-effect. Hooks consult the
// registry instead of hardcoding tool-name lists, so a single declaration
// per tool propagates to every gate (R3 — replaces R4's hardcoded
// dangerousToolKinds + the four maps in workspace_path_hook).
//
// Capabilities are additive: `file-edit` has both CapReadsPaths AND
// CapWritesPaths. `bash` has CapExecutesShell, CapReadsPaths, and
// CapWritesPaths simultaneously (a shell can do anything its argv allows).
type Capability string

const (
	// CapReadsPaths — the tool reads file/directory contents and accepts a
	// `path`/`workspace_path`/`absolute_path`/`file_path` input key that
	// WorkspacePathHook will gate against ReadPaths.
	CapReadsPaths Capability = "reads_paths"

	// CapWritesPaths — the tool creates/modifies/deletes files via a path
	// input key. WorkspacePathHook gates against WritePaths.
	CapWritesPaths Capability = "writes_paths"

	// CapExecutesShell — the tool runs free-form commands (shell, subprocess).
	// The input shape can't be safely path-gated; the only correct enforcement
	// is a per-tool sandbox (CAP-BT-05 for bash; ms09). Until then, hooks
	// deny these under restrictive WorkspacePaths.
	CapExecutesShell Capability = "executes_shell"

	// CapIssuesNetworkRequests — the tool fetches a URL. URLAllowlist (in
	// WorkspacePaths) gates the host; private-network deny is always-on.
	CapIssuesNetworkRequests Capability = "issues_network_requests"

	// CapSpawnsChild — the tool spawns sub-agents (delegate, spawn-teammate).
	// Dangerous because the child inherits permissions; the fail-closed
	// SpawnPermissionHook denies these without an explicit AllowedTools.
	CapSpawnsChild Capability = "spawns_child"
)

// dangerousCapabilities is the set of capabilities that mark a tool as
// dangerous-by-default for the fail-closed SpawnPermissionHook (R4 step 4).
// A tool with ANY of these gets denied when SpawnPermissions arrive empty.
// Reads-only tools (CapReadsPaths only) are NOT in this set — file_read /
// glob / grep stay allowed under fail-closed defaults.
var dangerousCapabilities = map[Capability]struct{}{
	CapWritesPaths:           {},
	CapExecutesShell:         {},
	CapIssuesNetworkRequests: {},
	CapSpawnsChild:           {},
}

// toolCapsMu guards toolCapabilities for safe init-time registration.
var (
	toolCapsMu       sync.RWMutex
	toolCapabilities = map[string]map[Capability]struct{}{}
)

// RegisterCapabilities declares the capabilities of a tool. Idempotent —
// repeated calls merge capabilities. Tools should self-register at init
// (`runtime.RegisterCapabilities("bash", CapExecutesShell, ...)`); the
// in-tree set is bootstrapped from this file's init().
func RegisterCapabilities(toolName string, caps ...Capability) {
	if toolName == "" {
		return
	}
	toolCapsMu.Lock()
	defer toolCapsMu.Unlock()
	set, ok := toolCapabilities[toolName]
	if !ok {
		set = make(map[Capability]struct{}, len(caps))
		toolCapabilities[toolName] = set
	}
	for _, c := range caps {
		set[c] = struct{}{}
	}
}

// HasCapability reports whether toolName has cap declared.
func HasCapability(toolName string, cap Capability) bool {
	toolCapsMu.RLock()
	defer toolCapsMu.RUnlock()
	set, ok := toolCapabilities[toolName]
	if !ok {
		return false
	}
	_, has := set[cap]
	return has
}

// HasAnyCapability reports whether toolName has at least one of caps.
// Convenience for "is this tool dangerous in any way?" checks.
func HasAnyCapability(toolName string, caps ...Capability) bool {
	toolCapsMu.RLock()
	defer toolCapsMu.RUnlock()
	set, ok := toolCapabilities[toolName]
	if !ok {
		return false
	}
	for _, c := range caps {
		if _, has := set[c]; has {
			return true
		}
	}
	return false
}

// IsDangerousTool reports whether toolName carries any dangerous capability.
// Used by SpawnPermissionHook's fail-closed default gate (R4 step 4) to
// decide whether to deny a tool when no explicit permissions were declared.
// Unknown tools return false — consistent with prior R4 behavior where
// only the hardcoded list mattered; tier-2 may flip the default policy.
func IsDangerousTool(toolName string) bool {
	toolCapsMu.RLock()
	defer toolCapsMu.RUnlock()
	set, ok := toolCapabilities[toolName]
	if !ok {
		return false
	}
	for cap := range set {
		if _, dangerous := dangerousCapabilities[cap]; dangerous {
			return true
		}
	}
	return false
}

// DangerousToolNames returns the names of all registered tools that carry
// a dangerous capability. Used for diagnostics + sorted, stable order for
// test output.
func DangerousToolNames() []string {
	toolCapsMu.RLock()
	defer toolCapsMu.RUnlock()
	var names []string
	for name := range toolCapabilities {
		set := toolCapabilities[name]
		for cap := range set {
			if _, d := dangerousCapabilities[cap]; d {
				names = append(names, name)
				break
			}
		}
	}
	// Sort for stable output (single-line bubble sort is fine for <50 tools).
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && strings.Compare(names[j-1], names[j]) > 0; j-- {
			names[j-1], names[j] = names[j], names[j-1]
		}
	}
	return names
}

// init bootstraps the in-tree tool capability declarations. New tools added
// to the project should either call RegisterCapabilities here (preferred —
// keeps the manifest in one place for audit) or self-register at their
// package init().
func init() {
	// Read-only file tools. Tool names are kebab (the registered names —
	// see tools/file_read/file_read.go etc.); a snake key here matches no
	// tool and the capability silently never fires.
	RegisterCapabilities("file-read", CapReadsPaths)
	RegisterCapabilities("glob", CapReadsPaths)
	RegisterCapabilities("grep", CapReadsPaths)

	// Write file tools — file-edit also reads (read-modify-write).
	RegisterCapabilities("file-write", CapWritesPaths)
	RegisterCapabilities("file-edit", CapReadsPaths, CapWritesPaths)

	// deck-export reads a deck file and writes its export (beside it, or at
	// `output`): a path reader and writer, gated like file-write.
	RegisterCapabilities("deck-export", CapReadsPaths, CapWritesPaths)

	// Shell: can do anything its argv allows. Mark all relevant capabilities
	// so any future hook that gates on Reads/Writes also catches bash.
	RegisterCapabilities("bash", CapExecutesShell, CapReadsPaths, CapWritesPaths)

	// Network fetch tools.
	RegisterCapabilities("web-fetch", CapIssuesNetworkRequests)
	RegisterCapabilities("rest-api-fetch", CapIssuesNetworkRequests)

	// Spawn tools — children inherit permissions, so the spawn itself is
	// dangerous-by-default in the same way as writing files.
	RegisterCapabilities("delegate", CapSpawnsChild)
	RegisterCapabilities("spawn-teammate", CapSpawnsChild)

	// Tools with no dangerous capabilities are intentionally NOT registered
	// (complete, task-status, check-messages, send-message, skill access
	// tools, ask-user, web-search). Registry absence = no capabilities = not
	// dangerous = passes the fail-closed default gate.
}
