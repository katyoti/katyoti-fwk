// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ResolveVariables builds the built-in variable map from runtime context.
func ResolveVariables(workspaceRoot, configDir, taskName, sessionID string) map[string]string {
	vars := make(map[string]string)

	// Built-in variables
	if home, err := os.UserHomeDir(); err == nil {
		vars["user_home_dir"] = home
	}
	if cwd, err := os.Getwd(); err == nil {
		vars["cwd"] = cwd
	}
	vars["workspace"] = workspaceRoot
	vars["config_dir"] = configDir
	vars["task_name"] = taskName
	vars["session_id"] = sessionID

	return vars
}

// ExpandVariables replaces ${xxx} placeholders in a string using the variable map.
// Supports ${env:VAR_NAME} for environment variables.
// Unresolved variables are left as-is.
func ExpandVariables(s string, vars map[string]string) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		key := match[2 : len(match)-1] // strip ${ and }

		// ${env:VAR_NAME}
		if strings.HasPrefix(key, "env:") {
			envName := key[4:]
			if val, ok := os.LookupEnv(envName); ok {
				return val
			}
			return match // unresolved env var
		}

		if val, ok := vars[key]; ok {
			return val
		}
		return match // unresolved variable
	})
}

// ExpandPath resolves variables in a path string and cleans the result.
func ExpandPath(path string, vars map[string]string) string {
	expanded := ExpandVariables(path, vars)
	return filepath.Clean(expanded)
}

// MergeTaskContext merges task-declared context (from frontmatter) with CLI context.
// Task context provides defaults; CLI context overrides.
// Variable expansion is applied to string values in task context.
func MergeTaskContext(taskCtx map[string]any, cliCtx map[string]any, vars map[string]string) map[string]any {
	merged := make(map[string]any)
	for k, v := range taskCtx {
		if s, ok := v.(string); ok {
			merged[k] = ExpandVariables(s, vars)
		} else {
			merged[k] = v
		}
	}
	for k, v := range cliCtx {
		merged[k] = v
	}
	return merged
}
