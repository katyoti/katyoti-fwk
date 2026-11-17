// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

func TestResolveProfile(t *testing.T) {
	kinds := map[string]string{
		"task.complete": "celebrate",
		"task.*":        "default",
		"agent.error":   "urgent",
		"agent.*":       "default",
		"security":      "critical",
		"*":             "fallback",
	}

	tests := []struct {
		kind     string
		expected string
	}{
		// Exact match
		{"task.complete", "celebrate"},
		{"agent.error", "urgent"},
		{"security", "critical"},

		// Wildcard match
		{"task.start", "default"},    // matches "task.*"
		{"task.progress", "default"}, // matches "task.*"
		{"agent.start", "default"},   // matches "agent.*"

		// Root wildcard
		{"unknown.event", "fallback"}, // matches "*"
		{"random", "fallback"},        // matches "*"

		// Empty kind → default profile
		{"", "default-profile"},

		// Deep hierarchy — walks up
		{"task.sub.deep", "default"}, // matches "task.*"
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := ResolveProfile(tt.kind, kinds, "default-profile")
			if got != tt.expected {
				t.Errorf("ResolveProfile(%q) = %q, want %q", tt.kind, got, tt.expected)
			}
		})
	}
}
