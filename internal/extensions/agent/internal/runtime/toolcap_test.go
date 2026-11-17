// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"testing"
)

// TestToolCap_BootstrappedTools verifies the init() declarations land in
// the registry with the expected capabilities. Regression guard for the
// canonical tool→capability mapping.
func TestToolCap_BootstrappedTools(t *testing.T) {
	cases := []struct {
		tool string
		caps []Capability
	}{
		{"file-read", []Capability{CapReadsPaths}},
		{"glob", []Capability{CapReadsPaths}},
		{"grep", []Capability{CapReadsPaths}},
		{"file-write", []Capability{CapWritesPaths}},
		{"file-edit", []Capability{CapReadsPaths, CapWritesPaths}},
		{"bash", []Capability{CapExecutesShell, CapReadsPaths, CapWritesPaths}},
		{"web-fetch", []Capability{CapIssuesNetworkRequests}},
		{"rest-api-fetch", []Capability{CapIssuesNetworkRequests}},
		{"delegate", []Capability{CapSpawnsChild}},
		{"spawn-teammate", []Capability{CapSpawnsChild}},
	}
	for _, tc := range cases {
		for _, c := range tc.caps {
			if !HasCapability(tc.tool, c) {
				t.Errorf("%s: missing capability %s", tc.tool, c)
			}
		}
	}
}

// TestToolCap_IsDangerousTool — the central predicate used by R4's fail-
// closed gate. Read-only tools are NOT dangerous; everything else with
// Writes/Executes/Network/Spawn capability IS.
func TestToolCap_IsDangerousTool(t *testing.T) {
	safe := []string{"file-read", "glob", "grep"}
	dangerous := []string{"file-write", "file-edit", "bash", "web-fetch", "rest-api-fetch", "delegate", "spawn-teammate"}

	for _, name := range safe {
		if IsDangerousTool(name) {
			t.Errorf("%s: should NOT be flagged dangerous (read-only)", name)
		}
	}
	for _, name := range dangerous {
		if !IsDangerousTool(name) {
			t.Errorf("%s: should be flagged dangerous", name)
		}
	}

	// Unknown tool → not dangerous (matches pre-R3 behavior: only registered
	// tools can be dangerous). Tier-2 may flip the default.
	if IsDangerousTool("never-registered-tool") {
		t.Error("unknown tool should not be flagged dangerous")
	}
}

// TestToolCap_RegisterCapabilitiesIsAdditive — repeated calls merge.
// Third-party plugins relying on idempotent registration won't double-up.
func TestToolCap_RegisterCapabilitiesIsAdditive(t *testing.T) {
	name := "test-tool-additive"
	defer func() {
		toolCapsMu.Lock()
		delete(toolCapabilities, name)
		toolCapsMu.Unlock()
	}()

	RegisterCapabilities(name, CapReadsPaths)
	RegisterCapabilities(name, CapWritesPaths)
	RegisterCapabilities(name, CapReadsPaths) // duplicate — no-op

	if !HasCapability(name, CapReadsPaths) || !HasCapability(name, CapWritesPaths) {
		t.Error("registration should be additive")
	}
	if HasCapability(name, CapExecutesShell) {
		t.Error("unregistered capability should not be reported")
	}
}

// TestToolCap_HasAnyCapability — short-circuit OR across the cap list.
func TestToolCap_HasAnyCapability(t *testing.T) {
	if !HasAnyCapability("file-edit", CapExecutesShell, CapWritesPaths) {
		t.Error("file_edit has CapWritesPaths → HasAnyCapability should be true")
	}
	if HasAnyCapability("file-read", CapExecutesShell, CapSpawnsChild) {
		t.Error("file_read has neither CapExecutesShell nor CapSpawnsChild → should be false")
	}
	if HasAnyCapability("never-registered", CapReadsPaths) {
		t.Error("unknown tool should be false")
	}
}

// TestToolCap_DangerousToolNamesSorted — diagnostic helper returns stable
// sorted output. Useful for snapshot-style tests + audit reports.
func TestToolCap_DangerousToolNamesSorted(t *testing.T) {
	names := DangerousToolNames()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("DangerousToolNames not sorted: %q before %q", names[i-1], names[i])
		}
	}
	// Spot-check: bash and file_write must appear (they're in init()).
	have := map[string]bool{}
	for _, n := range names {
		have[n] = true
	}
	for _, want := range []string{"bash", "file-write", "delegate"} {
		if !have[want] {
			t.Errorf("DangerousToolNames missing %q (found %v)", want, names)
		}
	}
}
