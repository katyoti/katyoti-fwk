// SPDX-License-Identifier: Apache-2.0

package api

import "testing"

// A stream's next release is the earliest planned one by date: never a
// frozen, released or dropped one, however early; one with no date comes
// after those with one, and an equal date is broken by the id.
func TestNextPlannedIsTheEarliestPlannedRelease(t *testing.T) {
	rel := func(id, state, date string) Release {
		return Release{ID: id, State: state, ReleaseFields: ReleaseFields{Date: date}}
	}
	for _, c := range []struct {
		name string
		list []Release
		want string
	}{
		{"a frozen or released one is never next", []Release{rel("RL-1", ReleaseFrozen, "2026-09-18"), rel("RL-2", ReleaseReleased, "2026-10-01"), rel("RL-3", ReleasePlanned, "2026-10-31")}, "RL-3"},
		{"a dropped one is never next", []Release{rel("RL-1", ReleaseDropped, "2026-09-01"), rel("RL-2", ReleasePlanned, "2026-12-31")}, "RL-2"},
		{"the earliest by date, whatever the order", []Release{rel("RL-9", ReleasePlanned, "2026-12-31"), rel("RL-8", ReleasePlanned, "2026-11-30")}, "RL-8"},
		{"one with no date after those with one", []Release{rel("RL-1", ReleasePlanned, ""), rel("RL-2", ReleasePlanned, "2027-01-31")}, "RL-2"},
		{"an equal date broken by the id", []Release{rel("RL-5", ReleasePlanned, "2026-11-30"), rel("RL-4", ReleasePlanned, "2026-11-30")}, "RL-4"},
	} {
		got, ok := NextPlanned(c.list)
		if !ok || got.ID != c.want {
			t.Errorf("%s: next %q (%v), want %s", c.name, got.ID, ok, c.want)
		}
	}
	if got, ok := NextPlanned([]Release{rel("RL-1", ReleaseFrozen, "2026-09-18")}); ok {
		t.Errorf("a stream with no planned release has a next one: %s", got.ID)
	}
}
