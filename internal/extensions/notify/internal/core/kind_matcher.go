// SPDX-License-Identifier: Apache-2.0

package core

import "strings"

// ResolveProfile finds the best-matching profile for a notification kind.
// Matching order (most specific wins):
//  1. Exact match: "task.complete" matches "task.complete"
//  2. Wildcard: "task.*" matches "task.complete"
//  3. Parent: "task" matches "task.complete"
//  4. Root wildcard: "*" matches anything
//  5. Default profile
func ResolveProfile(kind string, kinds map[string]string, defaultProfile string) string {
	if kind == "" {
		return defaultProfile
	}

	// 1. Exact match
	if profile, ok := kinds[kind]; ok {
		return profile
	}

	// 2. Walk up the hierarchy, checking wildcards at each level
	parts := strings.Split(kind, ".")
	for i := len(parts) - 1; i >= 1; i-- {
		prefix := strings.Join(parts[:i], ".")

		// Try wildcard at this level: "task.*"
		if profile, ok := kinds[prefix+".*"]; ok {
			return profile
		}

		// Try parent prefix: "task"
		if profile, ok := kinds[prefix]; ok {
			return profile
		}
	}

	// 3. Root wildcard
	if profile, ok := kinds["*"]; ok {
		return profile
	}

	return defaultProfile
}
