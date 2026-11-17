// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"testing"
	"time"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// TestDeriveStatus_TransitionTable covers every documented transition
// in modules/code-gocode/concepts.md §4. Adding a new state or rule MUST add a row here.
func TestDeriveStatus_TransitionTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		prev      gcapi.UCStatus
		runStates []gcapi.TestRunStatus
		deferOpen bool
		want      gcapi.UCStatus
	}{
		// Deferral wins over everything else.
		{"deferred + no tests", "", nil, true, gcapi.UCStatusFailDeferred},
		{"deferred + passing", gcapi.UCStatusCovered, []gcapi.TestRunStatus{gcapi.TestRunPass}, true, gcapi.UCStatusFailDeferred},
		{"deferred + failing", gcapi.UCStatusRegressed, []gcapi.TestRunStatus{gcapi.TestRunFail}, true, gcapi.UCStatusFailDeferred},

		// No test-cases at all → no-test
		{"no tests, no prev", "", nil, false, gcapi.UCStatusNoTest},
		{"no tests, was covered", gcapi.UCStatusCovered, nil, false, gcapi.UCStatusNoTest},

		// All not-run → no-test (covers freshly-created test-case fixtures)
		{"all not-run", "", []gcapi.TestRunStatus{gcapi.TestRunNotRun, gcapi.TestRunNotRun}, false, gcapi.UCStatusNoTest},
		{"all empty (== not-run)", "", []gcapi.TestRunStatus{"", ""}, false, gcapi.UCStatusNoTest},

		// Any fail when previously passing → regressed
		{"was validated, now any fail → regressed", gcapi.UCStatusValidated, []gcapi.TestRunStatus{gcapi.TestRunPass, gcapi.TestRunFail}, false, gcapi.UCStatusRegressed},
		{"was covered, now any fail → regressed", gcapi.UCStatusCovered, []gcapi.TestRunStatus{gcapi.TestRunFail}, false, gcapi.UCStatusRegressed},
		{"was pass (projected), now any fail → regressed", gcapi.UCStatusPass, []gcapi.TestRunStatus{gcapi.TestRunFail}, false, gcapi.UCStatusRegressed},

		// Any fail when not previously passing → in-dev
		{"was no-test, now failing → in-dev", gcapi.UCStatusNoTest, []gcapi.TestRunStatus{gcapi.TestRunFail}, false, gcapi.UCStatusInDev},
		{"was empty, now failing → in-dev", "", []gcapi.TestRunStatus{gcapi.TestRunFail}, false, gcapi.UCStatusInDev},
		{"was regressed, still failing → in-dev", gcapi.UCStatusRegressed, []gcapi.TestRunStatus{gcapi.TestRunFail}, false, gcapi.UCStatusInDev},

		// Harness-error treated as failure
		{"harness error → in-dev (from empty)", "", []gcapi.TestRunStatus{gcapi.TestRunHarnessError}, false, gcapi.UCStatusInDev},
		{"harness error + previously passing → regressed", gcapi.UCStatusCovered, []gcapi.TestRunStatus{gcapi.TestRunHarnessError}, false, gcapi.UCStatusRegressed},

		// All passing
		{"all pass, was no-test → covered", gcapi.UCStatusNoTest, []gcapi.TestRunStatus{gcapi.TestRunPass}, false, gcapi.UCStatusCovered},
		{"all pass, was in-dev → covered (newly green)", gcapi.UCStatusInDev, []gcapi.TestRunStatus{gcapi.TestRunPass, gcapi.TestRunPass}, false, gcapi.UCStatusCovered},
		{"all pass, was regressed → covered (recovered)", gcapi.UCStatusRegressed, []gcapi.TestRunStatus{gcapi.TestRunPass}, false, gcapi.UCStatusCovered},
		{"all pass, was validated → stays validated", gcapi.UCStatusValidated, []gcapi.TestRunStatus{gcapi.TestRunPass, gcapi.TestRunPass}, false, gcapi.UCStatusValidated},

		// Mixed pass + not-run (some layers not exercised) — counts as
		// passing because no test failed.
		{"pass + not-run mix", gcapi.UCStatusNoTest, []gcapi.TestRunStatus{gcapi.TestRunPass, gcapi.TestRunNotRun}, false, gcapi.UCStatusCovered},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DeriveStatus(tc.prev, tc.runStates, tc.deferOpen)
			if got != tc.want {
				t.Errorf("DeriveStatus(prev=%q, runStates=%v, deferOpen=%v) = %q, want %q",
					tc.prev, tc.runStates, tc.deferOpen, got, tc.want)
			}
		})
	}
}

// TestVerdictOf covers the 4-verdict projection.
func TestVerdictOf(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   gcapi.UCStatus
		want gcapi.UCStatus
	}{
		{gcapi.UCStatusValidated, gcapi.UCStatusPass},
		{gcapi.UCStatusCovered, gcapi.UCStatusPass},
		{gcapi.UCStatusPass, gcapi.UCStatusPass},
		{gcapi.UCStatusInDev, gcapi.UCStatusFail},
		{gcapi.UCStatusRegressed, gcapi.UCStatusFail},
		{gcapi.UCStatusFail, gcapi.UCStatusFail},
		{gcapi.UCStatusFailDeferred, gcapi.UCStatusFailDeferred},
		{gcapi.UCStatusNoTest, gcapi.UCStatusNoTest},
		{gcapi.UCStatusSkipped, gcapi.UCStatusSkipped},
		{"", gcapi.UCStatusNoTest}, // empty defaults to no-test
	}
	for _, c := range cases {
		c := c
		t.Run(string(c.in), func(t *testing.T) {
			t.Parallel()
			if got := VerdictOf(c.in); got != c.want {
				t.Errorf("VerdictOf(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// --- TCID matcher ----------------------------------------------------------

func TestUCIDFromTCID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tcid   string
		wantUC string
		wantOK bool
	}{
		{"L1-uc001-executor", "UC-001", true},
		{"L2-uc053-spawn-resume", "UC-053", true},
		{"L3-uc150-end-to-end-magentic", "UC-150", true},
		{"l1-uc007-lower-case", "UC-007", true},
		{"L1-uc1-short-digits", "UC-001", true}, // padded
		{"L1-uc1000-four-digits", "UC-1000", true},
		{"freeform-no-prefix", "", false},
		{"L4-uc001-bad-layer", "", false},
		{"L1-no-uc-prefix", "", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.tcid, func(t *testing.T) {
			t.Parallel()
			got, ok := UCIDFromTCID(c.tcid)
			if ok != c.wantOK {
				t.Errorf("UCIDFromTCID(%q) ok = %v, want %v", c.tcid, ok, c.wantOK)
			}
			if got != c.wantUC {
				t.Errorf("UCIDFromTCID(%q) = %q, want %q", c.tcid, got, c.wantUC)
			}
		})
	}
}

func TestNormalizeUCID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		wantUC string
		wantOK bool
	}{
		{"UC-001", "UC-001", true},
		{"uc-053", "UC-053", true},
		{"UC-1", "UC-001", true},
		{"uc7", "UC-007", true},
		{"  UC-042  ", "UC-042", true},
		{"UC-1000", "UC-1000", true},
		{"", "", false},
		{"not-a-uc", "", false},
		{"UC-abc", "", false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			got, ok := NormalizeUCID(c.in)
			if ok != c.wantOK {
				t.Errorf("ok = %v, want %v", ok, c.wantOK)
			}
			if got != c.wantUC {
				t.Errorf("got = %q, want %q", got, c.wantUC)
			}
		})
	}
}

// --- Aggregate end-to-end --------------------------------------------------

func TestAggregate_HappyPath(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	in := AggregateInputs{
		Module: gcapi.Module{Name: "acceptance"},
		UseCases: []gcapi.UseCase{
			{UCID: "UC-001", Severity: gcapi.UCSeverityCore, StatusCache: gcapi.UCStatusNoTest},
			{UCID: "UC-002", Severity: gcapi.UCSeverityCore, StatusCache: gcapi.UCStatusValidated},
			{UCID: "UC-003", Severity: gcapi.UCSeverityQoL, StatusCache: gcapi.UCStatusFailDeferred, FailDeferredBy: "OD-001"},
			{UCID: "UC-004", Severity: gcapi.UCSeveritySecurity, StatusCache: gcapi.UCStatusCovered},
		},
		TestCases: []gcapi.TestCase{
			// UC-001: L1 passes → covered
			{TCID: "L1-uc001-foo", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
			// UC-002: was validated, still passing → stays validated
			{TCID: "L1-uc002-bar", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
			{TCID: "L2-uc002-baz", Layer: gcapi.UCLayerL2, LastRunStatus: gcapi.TestRunPass},
			// UC-003: deferred — doesn't matter
			{TCID: "L1-uc003-quux", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunFail},
			// UC-004: was covered, now fails → regressed
			{TCID: "L1-uc004-broken", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunFail},
			// Orphan: doesn't match a UC
			{TCID: "freeform-orphan", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
		},
		OpenODByID: map[string]gcapi.OpenDecision{
			"OD-001": {ODID: "OD-001", State: gcapi.ODStateOpen},
		},
		RanAt: now,
	}

	out := Aggregate(in)

	if got, want := out.Totals.UCTotal, 4; got != want {
		t.Errorf("UCTotal = %d, want %d", got, want)
	}
	if got, want := out.Totals.Pass, 2; got != want {
		t.Errorf("Pass = %d, want %d", got, want)
	}
	if got, want := out.Totals.Fail, 1; got != want {
		t.Errorf("Fail = %d, want %d", got, want)
	}
	if got, want := out.Totals.FailDeferred, 1; got != want {
		t.Errorf("FailDeferred = %d, want %d", got, want)
	}
	if got, want := out.Totals.NoTest, 0; got != want {
		t.Errorf("NoTest = %d, want %d", got, want)
	}

	// Per-UC verdicts
	if v := out.PerUC["UC-001"]; v.NewStatus != gcapi.UCStatusCovered || v.Verdict != gcapi.UCStatusPass {
		t.Errorf("UC-001 = %+v", v)
	}
	if v := out.PerUC["UC-002"]; v.NewStatus != gcapi.UCStatusValidated || v.Verdict != gcapi.UCStatusPass {
		t.Errorf("UC-002 = %+v (should stay validated)", v)
	}
	if v := out.PerUC["UC-003"]; v.NewStatus != gcapi.UCStatusFailDeferred || v.Verdict != gcapi.UCStatusFailDeferred {
		t.Errorf("UC-003 = %+v", v)
	}
	if v := out.PerUC["UC-004"]; v.NewStatus != gcapi.UCStatusRegressed || v.Verdict != gcapi.UCStatusFail {
		t.Errorf("UC-004 = %+v (should be regressed)", v)
	}
	if v := out.PerUC["UC-004"]; v.Reason == "" {
		t.Errorf("UC-004 should carry a failure reason; got empty")
	}

	if len(out.OrphanTestCases) != 1 {
		t.Errorf("OrphanTestCases len = %d, want 1", len(out.OrphanTestCases))
	}
}

func TestAggregate_LayerFilter_RestrictsToL1(t *testing.T) {
	t.Parallel()
	in := AggregateInputs{
		UseCases: []gcapi.UseCase{
			{UCID: "UC-010", Severity: gcapi.UCSeverityCore},
		},
		TestCases: []gcapi.TestCase{
			{TCID: "L1-uc010-a", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
			{TCID: "L2-uc010-b", Layer: gcapi.UCLayerL2, LastRunStatus: gcapi.TestRunFail}, // ignored by L1-only filter
		},
		LayerFilter: "L1",
		RanAt:       time.Now(),
	}
	out := Aggregate(in)
	if v := out.PerUC["UC-010"]; v.Verdict != gcapi.UCStatusPass {
		t.Errorf("UC-010 with L1-only filter = %+v, want pass (L2 failure should be ignored)", v)
	}
}

func TestAggregate_SeverityFilter_SecurityOnly(t *testing.T) {
	t.Parallel()
	in := AggregateInputs{
		UseCases: []gcapi.UseCase{
			{UCID: "UC-020", Severity: gcapi.UCSeverityCore},
			{UCID: "UC-021", Severity: gcapi.UCSeveritySecurity},
		},
		TestCases: []gcapi.TestCase{
			{TCID: "L1-uc020-x", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
			{TCID: "L1-uc021-y", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
		},
		SeverityFilter: "security",
		RanAt:          time.Now(),
	}
	out := Aggregate(in)
	if v := out.PerUC["UC-020"]; !v.Skipped {
		t.Errorf("UC-020 should be skipped (core not in security-only filter); got %+v", v)
	}
	if v := out.PerUC["UC-021"]; v.Skipped {
		t.Errorf("UC-021 should be included (security); got %+v", v)
	}
}

func TestAggregate_UCFilter_PrefixWildcard(t *testing.T) {
	t.Parallel()
	in := AggregateInputs{
		UseCases: []gcapi.UseCase{
			{UCID: "UC-040", Severity: gcapi.UCSeverityCore},
			{UCID: "UC-041", Severity: gcapi.UCSeverityCore},
			{UCID: "UC-053", Severity: gcapi.UCSeverityCore},
		},
		TestCases: []gcapi.TestCase{
			{TCID: "L1-uc040-a", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
			{TCID: "L1-uc041-b", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
			{TCID: "L1-uc053-c", Layer: gcapi.UCLayerL1, LastRunStatus: gcapi.TestRunPass},
		},
		UCFilter: "UC-04*",
		RanAt:    time.Now(),
	}
	out := Aggregate(in)
	if v := out.PerUC["UC-040"]; v.Skipped {
		t.Errorf("UC-040 should match UC-04* filter; got %+v", v)
	}
	if v := out.PerUC["UC-053"]; !v.Skipped {
		t.Errorf("UC-053 should be skipped (UC-04* filter); got %+v", v)
	}
}
