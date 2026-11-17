// SPDX-License-Identifier: Apache-2.0

package file_edit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/tools/api/pathtool"
)

func makePermissionFunc(resolver PathResolver) toolsapi.PermissionFunc {
	return func(_ context.Context, raw json.RawMessage) toolsapi.PermissionResult {
		var in EditInput
		if err := json.Unmarshal(raw, &in); err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  fmt.Sprintf("file-edit: malformed input: %v", err),
			}
		}
		if err := validatePaths(in); err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  err.Error(),
			}
		}

		resolved, err := pathtool.Resolve(in.WorkspacePath, in.AbsolutePath, resolver, "file-edit")
		if err != nil {
			return toolsapi.PermissionResult{
				Behavior: toolsapi.PermissionDeny,
				Message:  err.Error(),
			}
		}
		if !resolved.FromAbsoluteField {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}
		root := resolver.WorkspaceRoot()
		if pathtool.ClassifyAgainstWorkspace(resolved.AbsPath, root) {
			return toolsapi.PermissionResult{Behavior: toolsapi.PermissionAllow}
		}
		slashAbs := filepath.ToSlash(resolved.AbsPath)
		return toolsapi.PermissionResult{
			Behavior:    toolsapi.PermissionAsk,
			Message:     fmt.Sprintf("file-edit wants to modify %q, which is outside the workspace %q. Allow?", resolved.AbsPath, root),
			Dimensions:  map[string]string{"absolute_path": slashAbs},
			Suggestions: pathtool.BuildOutsideWorkspaceSuggestions("file-edit", slashAbs),
		}
	}
}

// validatePaths checks path-field exclusivity only — used by the
// permission gate. The full Validate in file_edit.go also checks
// old_string==new_string etc. but the gate only cares about the path.
func validatePaths(in EditInput) error {
	hasWS := in.WorkspacePath != ""
	hasAbs := in.AbsolutePath != ""
	switch {
	case hasWS && hasAbs:
		return errors.New("file-edit: set exactly one of workspace_path or absolute_path, not both")
	case !hasWS && !hasAbs:
		return errors.New("file-edit: workspace_path or absolute_path is required")
	}
	return nil
}
