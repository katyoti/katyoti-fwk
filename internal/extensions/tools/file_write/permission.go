// SPDX-License-Identifier: Apache-2.0

package file_write

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// makePermissionFunc returns the per-call permission policy for
// file_write. Mirrors file_read's permission policy structure
// almost exactly — only the rule prefix (`file_write` vs
// `file_read`) and error wording differ.
//
// Policy by branch:
//
//   - workspace_path → PermissionAllow. Workspace-internal writes
//     are always safe by construction; the resolver itself rejects
//     absolute paths and `..` escapes.
//
//   - absolute_path inside workspace → PermissionAllow. The user
//     used the wrong field but the result is safe; the handler
//     attaches a "prefer workspace_path" warning to the result.
//
//   - absolute_path outside workspace → PermissionAsk with a message
//     naming the path and the workspace, plus Dimensions and
//     Suggestions for the rules system. Same scope as file_read:
//     the LLM cannot escalate, the user picks the trust radius.
//
// Validation errors and parse errors short-circuit to PermissionDeny
// so the LLM gets an immediate, structured error instead of being
// asked about a malformed call.
//
// **Note**: this policy uses Validate WITHOUT the content size cap
// (passes 0 → cap disabled). The cap belongs to the handler, not
// the gate — the gate's job is to decide whether the call is
// allowed, not whether the content fits. The handler re-runs
// Validate with the real cap to enforce it.
func makePermissionFunc(resolver PathResolver) toolsapi.PermissionFunc {
	return func(_ context.Context, raw json.RawMessage) toolsapi.PermissionResult {
		var in WriteInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  fmt.Sprintf("file-write: malformed input: %v", err),
			}
		}

		// Use 0 as the cap so the gate doesn't reject calls solely
		// for content size — that's the handler's concern.
		if err := Validate(in, 0); err != nil {
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
			// nudge is attached to the write result, not the
			// permission result.
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}

		// Outside workspace: ask the user via the kernel permission gate.
		//
		// All paths in Dimensions and Suggestion rule strings are
		// normalized to forward-slash form via filepath.ToSlash so
		// the doublestar matcher behaves consistently across Windows
		// and POSIX. (See feedback_normalize_paths_for_globs.md.)
		slashAbs := filepath.ToSlash(resolved.AbsPath)
		return toolsapi.PermissionResult{
			Behavior: toolsapi.PermissionAsk,
			Message: fmt.Sprintf(
				"file-write wants to write %q, which is outside the workspace %q. Allow this write?",
				resolved.AbsPath, root,
			),
			Dimensions:  map[string]string{"absolute_path": slashAbs},
			Suggestions: buildOutsideWorkspaceSuggestions(slashAbs),
		}
	}
}

// buildOutsideWorkspaceSuggestions returns a graduated set of
// "always allow" suggestions for an out-of-workspace file_write.
// Three levels:
//
//  1. Allow this exact file only (narrowest, safest)
//  2. Allow any write under the parent directory
//  3. Allow any out-of-workspace write by file-write (broadest)
//
// The slashAbs argument MUST be the absolute path with forward
// slashes (via filepath.ToSlash) so the resulting rule strings are
// portable across operating systems and match the dimension value
// the kernel uses for rule matching.
//
// The rule prefix is `file_write` (not `file_read`) — the rules
// system matches each tool independently, so granting "always allow"
// for file_write under /home/user does NOT also grant file_read
// under /home/user. The user picks each tool's trust radius.
func buildOutsideWorkspaceSuggestions(slashAbs string) []toolsapi.PermissionSuggestion {
	// path.Dir-equivalent on a forward-slash string. Avoid
	// filepath.Dir here because on Windows it would re-introduce
	// backslashes.
	parent := slashAbs
	if i := strings.LastIndex(slashAbs, "/"); i >= 0 {
		parent = slashAbs[:i]
	}
	return []toolsapi.PermissionSuggestion{
		{
			Label: fmt.Sprintf("Always allow writing this exact file (%s)", slashAbs),
			Rule:  "file-write.absolute_path:" + slashAbs,
		},
		{
			Label: fmt.Sprintf("Always allow writes under %s", parent),
			Rule:  "file-write.absolute_path:" + parent + "/**",
		},
		{
			Label: "Always allow any out-of-workspace write by file-write",
			Rule:  "file-write.absolute_path:**",
		},
	}
}
