// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// The workspace slot is the project's id, a valid subject token; anything
// else - no id, an id no subject could carry - is the unbound slot.
func TestWorkspaceTokenIsTheProjectIDOrTheUnboundSlot(t *testing.T) {
	for in, want := range map[string]string{
		"00000000-0000-7000-8000-000000000001":   "00000000-0000-7000-8000-000000000001",
		" 0000000A-0000-7000-8000-0000000000BC ": "0000000a-0000-7000-8000-0000000000bc",
		"":                                       UnboundWorkspace,
		"repo.a":                                 UnboundWorkspace,
		"a b":                                    UnboundWorkspace,
	} {
		if got := WorkspaceToken(in); got != want {
			t.Errorf("WorkspaceToken(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ProjectWorkspaceToken(t.TempDir()); got != UnboundWorkspace {
		t.Errorf("a folder with no project object: %q, want %q", got, UnboundWorkspace)
	}
	if got, want := ProjectEventsPattern("local", "dev", "w1"), "ktw.local.universe.dev.workspace.w1.>"; got != want {
		t.Errorf("ProjectEventsPattern = %q, want %q", got, want)
	}
	if got, want := UniverseWorkspacesPattern("local", "dev"), "ktw.local.universe.dev.workspace.>"; got != want {
		t.Errorf("UniverseWorkspacesPattern = %q, want %q", got, want)
	}
}
