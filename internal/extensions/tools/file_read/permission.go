// SPDX-License-Identifier: Apache-2.0

package file_read

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// makePermissionFunc returns the per-call permission policy for
// file_read. The closure captures the resolver so the policy can read
// the workspace root and resolve workspace_path inputs without going
// through the handler.
//
// Policy by branch:
//
//   - workspace_path → PermissionAllow. Workspace-internal reads are
//     always safe by construction; the resolver itself rejects
//     absolute paths and `..` escapes.
//
//   - absolute_path inside workspace → PermissionAllow. The user used
//     the wrong field but the result is safe; the handler attaches a
//     warning to the result asking the LLM to prefer workspace_path
//     next time. (We do not surface the warning here because the
//     PermissionResult is consumed by the kernel, not the model.)
//
//   - absolute_path outside workspace → PermissionAsk with a message
//     naming both the target path and the workspace root. The kernel
//     gate calls HumanGate.PromptConfirm; on approval the handler
//     proceeds, on denial the kernel emits an error tool_result.
//
// Validation errors short-circuit to PermissionDeny so the LLM gets
// an immediate, structured error instead of being asked about a
// malformed call.
func makePermissionFunc(resolver PathResolver) toolsapi.PermissionFunc {
	return func(_ context.Context, raw json.RawMessage) toolsapi.PermissionResult {
		var in ReadInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  fmt.Sprintf("file-read: malformed input: %v", err),
			}
		}

		if err := Validate(in); err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  err.Error(),
			}
		}

		resolved, err := Resolve(in, resolver)
		if err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  err.Error(),
			}
		}

		// Workspace branch: structurally safe, no boundary check.
		if !resolved.FromAbsoluteField {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}

		// Absolute branch: classify against the workspace root.
		root := resolver.WorkspaceRoot()
		if ClassifyAgainstWorkspace(resolved.AbsPath, root) {
			// Inside workspace: allow. The "prefer workspace_path"
			// nudge is attached to the read result, not the
			// permission result.
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}

		// Outside workspace: ask the user via the kernel permission gate.
		// The Dimensions and Suggestions fields drive the rules system:
		//
		//   - Dimensions exposes absolute_path so profile rules and
		//     "always allow" decisions can match against it. The
		//     value is normalized to forward slashes (filepath.ToSlash)
		//     so doublestar globs match consistently across Windows
		//     and POSIX. This is the canonical form for permission
		//     rules — gitignore and most permission systems do the
		//     same.
		//
		//   - Suggestions are tool-proposed scopes (this exact file,
		//     parent directory, anywhere outside workspace) that the
		//     user can promote to a session-wide allow rule via the
		//     PromptPermission UI. We offer three nested levels so
		//     the user picks the trust radius they want.
		slashAbs := filepath.ToSlash(resolved.AbsPath)
		return toolsapi.PermissionResult{
			Behavior: toolsapi.PermissionAsk,
			Message: fmt.Sprintf(
				"file-read wants to read %q, which is outside the workspace %q. Allow this read?",
				resolved.AbsPath, root,
			),
			Dimensions:  map[string]string{"absolute_path": slashAbs},
			Suggestions: buildOutsideWorkspaceSuggestions(slashAbs),
		}
	}
}

// buildOutsideWorkspaceSuggestions returns a graduated set of "always
// allow" suggestions for an out-of-workspace file_read. Three levels:
//
//  1. Allow this exact file only (narrowest, safest)
//  2. Allow any read under the parent directory
//  3. Allow any out-of-workspace read by file-read (broadest)
//
// The slashAbs argument MUST be the absolute path with forward
// slashes (via filepath.ToSlash) so the resulting rule strings are
// portable across operating systems and match the dimension value the
// kernel uses for rule matching.
//
// The user picks whichever scope they trust. The kernel adds the
// chosen rule to the active session profile so future matching calls
// bypass the prompt entirely.
func buildOutsideWorkspaceSuggestions(slashAbs string) []toolsapi.PermissionSuggestion {
	// path.Dir-equivalent on a forward-slash string. Avoid filepath.Dir
	// here because on Windows it would re-introduce backslashes.
	parent := slashAbs
	if i := strings.LastIndex(slashAbs, "/"); i >= 0 {
		parent = slashAbs[:i]
	}
	return []toolsapi.PermissionSuggestion{
		{
			Label: fmt.Sprintf("Always allow this exact file (%s)", slashAbs),
			Rule:  "file-read.absolute_path:" + slashAbs,
		},
		{
			Label: fmt.Sprintf("Always allow reads under %s", parent),
			Rule:  "file-read.absolute_path:" + parent + "/**",
		},
		{
			Label: "Always allow any out-of-workspace read by file-read",
			Rule:  "file-read.absolute_path:**",
		},
	}
}
