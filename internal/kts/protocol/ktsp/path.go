// SPDX-License-Identifier: Apache-2.0

package ktsp

import "strings"

// ParsePath splits a three-level KTSP path into its components.
//
// Three segments: "container_id/slot/method" → (containerID, slot, method)
// Two segments:   "slot/method"             → ("", slot, method) — local dispatch
// One segment:    "slot"                    → ("", slot, "")
func ParsePath(path string) (containerID, slot, method string) {
	parts := strings.SplitN(path, "/", 3)
	switch len(parts) {
	case 3:
		return parts[0], parts[1], parts[2]
	case 2:
		return "", parts[0], parts[1]
	case 1:
		return "", parts[0], ""
	default:
		return "", "", ""
	}
}

// FormatPath builds a KTSP path from components.
// If containerID is empty, returns "slot/method" (local path).
// If method is empty, returns "containerID/slot" or just "slot".
func FormatPath(containerID, slot, method string) string {
	if containerID != "" {
		if method != "" {
			return containerID + "/" + slot + "/" + method
		}
		return containerID + "/" + slot
	}
	if method != "" {
		return slot + "/" + method
	}
	return slot
}

// IsLocalPath returns true if the path has no container_id prefix (two segments or less).
func IsLocalPath(path string) bool {
	return strings.Count(path, "/") < 2
}

// SMPath returns a path targeting the SM's virtual
// Example: SMPath("session", "create") → "sm/session/create"
func SMPath(slot, method string) string {
	return FormatPath("sm", slot, method)
}
