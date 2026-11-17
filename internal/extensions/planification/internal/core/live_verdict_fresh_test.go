// SPDX-License-Identifier: Apache-2.0

package core

import "testing"

// The script a live probe runs is named from the project root, through the
// folder a `cd <dir> &&` before it enters.
func TestProbeScriptPathFollowsTheFolderACdEnters(t *testing.T) {
	for probe, want := range map[string]string{
		"bash app/ktw-gui/frontend/tests/live/x.sh":           "app/ktw-gui/frontend/tests/live/x.sh",
		"cd app/ktw-gui/frontend && bash tests/live/x.sh":     "app/ktw-gui/frontend/tests/live/x.sh",
		`cd "app/ktw-gui/frontend" && bash ./tests/live/x.sh`: "app/ktw-gui/frontend/tests/live/x.sh",
		"go test ./...": "",
	} {
		if got := probeScriptPath(probe); got != want {
			t.Errorf("probeScriptPath(%q) = %q; want %q", probe, got, want)
		}
	}
	if got := laterOf("2026-01-01T00:00:00Z", "2026-01-02T00:00:00+01:00"); got != "2026-01-02T00:00:00+01:00" {
		t.Errorf("laterOf: %q", got)
	}
	if got := laterOf("", "2026-01-02T00:00:00Z"); got != "2026-01-02T00:00:00Z" {
		t.Errorf("laterOf with an empty since: %q", got)
	}
}
