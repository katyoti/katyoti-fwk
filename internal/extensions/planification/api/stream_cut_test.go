// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A STREAM'S NEXT CUT - computed from its release mode
// by one function: periodic, its last cut plus its period (from now when it
// never cut); on change, not before its last cut plus its minimum interval (a
// day when none), its next delivered increment cutting it once that passed;
// manual, none.
func TestAStreamsNextCutFollowsItsMode(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name     string
		f        api.StreamFields
		at, says string
	}{
		{"periodic, cut before", api.StreamFields{ReleaseMode: api.ModePeriodic, Every: "14d", LastCut: "2026-10-10T08:00:00Z"}, "2026-10-24", "every 14d"},
		{"periodic, never cut", api.StreamFields{ReleaseMode: api.ModePeriodic, Every: "7d"}, "2026-10-25", "every 7d"},
		{"periodic, no valid period", api.StreamFields{ReleaseMode: api.ModePeriodic, Every: "soon"}, "", "does not cut"},
		{"on change, within its interval", api.StreamFields{ReleaseMode: api.ModeOnChange, MinInterval: "2d", LastCut: "2026-10-18T06:00:00Z"}, "2026-10-20", "not before 2026-10-20"},
		{"on change, its interval passed", api.StreamFields{ReleaseMode: api.ModeOnChange, MinInterval: "2d", LastCut: "2026-10-01T06:00:00Z"}, "", "next delivered increment"},
		{"on change, a day by default", api.StreamFields{ReleaseMode: api.ModeOnChange, LastCut: "2026-10-18T06:00:00Z"}, "2026-10-19", "every 1d"},
		{"manual", api.StreamFields{ReleaseMode: api.ModeManual, LastCut: "2026-10-01T06:00:00Z"}, "", "a person cuts it"},
		{"no mode: manual", api.StreamFields{}, "", "manual"},
	} {
		got := api.NextCut(c.f, now)
		if got.At != c.at || !strings.Contains(got.Text, c.says) {
			t.Errorf("%s: next cut %q, %q; want %q, saying %q", c.name, got.At, got.Text, c.at, c.says)
		}
	}
}
