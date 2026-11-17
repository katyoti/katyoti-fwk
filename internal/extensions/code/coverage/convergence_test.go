// SPDX-License-Identifier: Apache-2.0

package coverage

import (
	"math/rand"
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// TestDeriveStatus_ConvergesUnderRandomRuns asserts the state machine
// is **stable under repetition**: a UC fed the same run-states twice
// in a row produces the same status both times. The state machine
// must depend only on (prev, runStates, deferOpen) — no hidden inputs.
//
// Property: for any random sequence of (prev, runStates), DeriveStatus
// composed with itself (output of step N becomes prev for step N+1)
// converges to a fixed-point in ≤ 2 steps once runStates stop changing.
//
// Concretely: if runStates is constant for two consecutive iterations,
// the status is identical between them.
func TestDeriveStatus_ConvergesUnderRandomRuns(t *testing.T) {
	t.Parallel()
	const seed = 0x5cf1b
	r := rand.New(rand.NewSource(seed))

	runStateOptions := []gcapi.TestRunStatus{
		gcapi.TestRunPass,
		gcapi.TestRunFail,
		gcapi.TestRunHarnessError,
		gcapi.TestRunNotRun,
		"", // empty / not-run treated as not-run
	}
	prevOptions := []gcapi.UCStatus{
		"",
		gcapi.UCStatusValidated,
		gcapi.UCStatusCovered,
		gcapi.UCStatusInDev,
		gcapi.UCStatusRegressed,
		gcapi.UCStatusFailDeferred,
		gcapi.UCStatusNoTest,
	}

	for iter := 0; iter < 200; iter++ {
		// Random run-states slice of length 0-4.
		n := r.Intn(5)
		runs := make([]gcapi.TestRunStatus, n)
		for i := range runs {
			runs[i] = runStateOptions[r.Intn(len(runStateOptions))]
		}
		prev := prevOptions[r.Intn(len(prevOptions))]
		deferOpen := r.Intn(2) == 0

		// Step 1.
		s1 := DeriveStatus(prev, runs, deferOpen)
		// Step 2: re-derive using s1 as the new prev, SAME runs.
		s2 := DeriveStatus(s1, runs, deferOpen)
		// Step 3: composition must converge after step 2.
		s3 := DeriveStatus(s2, runs, deferOpen)
		if s2 != s3 {
			t.Errorf(
				"non-convergent: prev=%q runs=%v deferOpen=%v → s1=%q s2=%q s3=%q",
				prev, runs, deferOpen, s1, s2, s3)
		}
	}
}

// TestDeriveStatus_FailDeferredOverridesEverything asserts that
// `deferOpen=true` ALWAYS wins, regardless of run-states or prev.
// This is a non-negotiable rule per modules/code-gocode/concepts.md §4 (an open OD
// suppresses the verdict).
func TestDeriveStatus_FailDeferredOverridesEverything(t *testing.T) {
	t.Parallel()
	const seed = 0x5cf1c
	r := rand.New(rand.NewSource(seed))
	runStateOptions := []gcapi.TestRunStatus{
		gcapi.TestRunPass, gcapi.TestRunFail, gcapi.TestRunHarnessError, gcapi.TestRunNotRun,
	}
	prevOptions := []gcapi.UCStatus{
		"", gcapi.UCStatusValidated, gcapi.UCStatusCovered, gcapi.UCStatusInDev,
		gcapi.UCStatusRegressed, gcapi.UCStatusFailDeferred, gcapi.UCStatusNoTest,
	}
	for iter := 0; iter < 100; iter++ {
		n := r.Intn(5)
		runs := make([]gcapi.TestRunStatus, n)
		for i := range runs {
			runs[i] = runStateOptions[r.Intn(len(runStateOptions))]
		}
		prev := prevOptions[r.Intn(len(prevOptions))]
		got := DeriveStatus(prev, runs, true /* deferOpen */)
		if got != gcapi.UCStatusFailDeferred {
			t.Errorf("deferOpen=true must always win: prev=%q runs=%v got=%q",
				prev, runs, got)
		}
	}
}

// TestDeriveStatus_AllPassPreservesValidated asserts the validated
// state is sticky once achieved (the audit-bundle promotes covered →
// validated; subsequent verify runs with all-pass keep it validated
// per modules/code-gocode/concepts.md §4 — "still passing post-audit" semantics).
func TestDeriveStatus_AllPassPreservesValidated(t *testing.T) {
	t.Parallel()
	out := DeriveStatus(
		gcapi.UCStatusValidated,
		[]gcapi.TestRunStatus{gcapi.TestRunPass, gcapi.TestRunPass},
		false,
	)
	if out != gcapi.UCStatusValidated {
		t.Errorf("validated should survive all-pass verify: got %q", out)
	}
}

// TestDeriveStatus_RegressedRequiresPreviousPassing asserts that
// `regressed` only ever arises from a failure following a pass —
// a UC that has never passed flips to `in-dev` when first failing
// per modules/code-gocode/concepts.md §4.
func TestDeriveStatus_RegressedRequiresPreviousPassing(t *testing.T) {
	t.Parallel()
	failing := []gcapi.TestRunStatus{gcapi.TestRunFail}
	cases := []struct {
		prev gcapi.UCStatus
		want gcapi.UCStatus
	}{
		{"", gcapi.UCStatusInDev},
		{gcapi.UCStatusNoTest, gcapi.UCStatusInDev},
		{gcapi.UCStatusInDev, gcapi.UCStatusInDev},
		{gcapi.UCStatusCovered, gcapi.UCStatusRegressed},
		{gcapi.UCStatusValidated, gcapi.UCStatusRegressed},
		{gcapi.UCStatusPass, gcapi.UCStatusRegressed},
	}
	for _, tc := range cases {
		got := DeriveStatus(tc.prev, failing, false)
		if got != tc.want {
			t.Errorf("DeriveStatus(prev=%q, fail) = %q want %q", tc.prev, got, tc.want)
		}
	}
}
