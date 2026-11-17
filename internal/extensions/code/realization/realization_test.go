// SPDX-License-Identifier: Apache-2.0

package realization

import (
	"testing"
)

func TestDeriveUCStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		statusCache string
		want        Status
	}{
		{"pass → done", "pass", StatusDone},
		{"validated → done", "validated", StatusDone},
		{"covered → done", "covered", StatusDone},
		{"regressed → regressed", "regressed", StatusRegressed},
		{"fail → in-progress", "fail", StatusInProgress},
		{"in-dev → in-progress", "in-dev", StatusInProgress},
		{"fail-deferred → in-progress", "fail-deferred", StatusInProgress},
		{"no-test → not-started", "no-test", StatusNotStarted},
		{"skipped → not-started", "skipped", StatusNotStarted},
		{"empty → not-started", "", StatusNotStarted},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DeriveUCStatus(tc.statusCache); got != tc.want {
				t.Errorf("DeriveUCStatus(%q) = %q, want %q", tc.statusCache, got, tc.want)
			}
		})
	}
}

func TestAggregateFeatureStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ucs  []Status
		want Status
	}{
		{
			name: "no wanted UCs → not-started (US unclaimed)",
			ucs:  nil,
			want: StatusNotStarted,
		},
		{
			name: "all done → done",
			ucs:  []Status{StatusDone, StatusDone, StatusDone},
			want: StatusDone,
		},
		{
			name: "regressed dominates",
			ucs:  []Status{StatusDone, StatusRegressed, StatusInProgress},
			want: StatusRegressed,
		},
		{
			name: "not-started beats in-progress + done",
			ucs:  []Status{StatusInProgress, StatusDone, StatusNotStarted},
			want: StatusNotStarted,
		},
		{
			name: "in-progress beats verifying + done",
			ucs:  []Status{StatusVerifying, StatusInProgress, StatusDone},
			want: StatusInProgress,
		},
		{
			name: "verifying alone",
			ucs:  []Status{StatusVerifying, StatusVerifying, StatusDone},
			want: StatusVerifying,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := AggregateFeatureStatus(tc.ucs)
			if got != tc.want {
				t.Errorf("AggregateFeatureStatus(%v) = %q, want %q", tc.ucs, got, tc.want)
			}
		})
	}
}
