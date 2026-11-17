// SPDX-License-Identifier: Apache-2.0

package methodsservice

import (
	"context"
	"testing"
)

// TestRollup pins the METHOD's grading rule, now owned by the method pack rather
// than the substrate: a composite is green ONLY when every check passed, and
// anything unproven blocks green — so "no criteria" and "not yet probed" can never
// masquerade as success.
func TestRollup(t *testing.T) {
	svc := New()
	ctx := context.Background()

	cases := []struct {
		name     string
		in       []CheckStatus
		status   string
		passed   int
		total    int
		unmetLen int
	}{
		{
			name:   "all pass -> green",
			in:     []CheckStatus{{ID: "a", Status: "pass"}, {ID: "b", Status: "pass"}},
			status: "green", passed: 2, total: 2, unmetLen: 0,
		},
		{
			name:   "one fail -> unmet",
			in:     []CheckStatus{{ID: "a", Status: "pass"}, {ID: "b", Status: "fail"}},
			status: "unmet", passed: 1, total: 2, unmetLen: 1,
		},
		{
			// The load-bearing case: unproven is NOT a pass.
			name:   "no-probe blocks green",
			in:     []CheckStatus{{ID: "a", Status: "pass"}, {ID: "b", Status: "no-probe"}},
			status: "unmet", passed: 1, total: 2, unmetLen: 1,
		},
		{
			name:   "harness-error blocks green",
			in:     []CheckStatus{{ID: "a", Status: "harness-error"}},
			status: "unmet", passed: 0, total: 1, unmetLen: 1,
		},
		{
			// No checks at all is never green by omission.
			name:   "no checks -> ungraded",
			in:     nil,
			status: "ungraded", passed: 0, total: 0, unmetLen: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := svc.Rollup(ctx, RollupInput{Checks: tc.in})
			if err != nil {
				t.Fatalf("Rollup: %v", err)
			}
			if out.Status != tc.status {
				t.Errorf("status = %q, want %q", out.Status, tc.status)
			}
			if out.Passed != tc.passed || out.Total != tc.total {
				t.Errorf("passed/total = %d/%d, want %d/%d", out.Passed, out.Total, tc.passed, tc.total)
			}
			if len(out.Unmet) != tc.unmetLen {
				t.Errorf("unmet = %v, want %d entries", out.Unmet, tc.unmetLen)
			}
		})
	}
}

// TestRollup_ProgressIsNotAGrade pins that progress is a steering signal only —
// 2 of 3 is real information, but it is still not green.
func TestRollup_ProgressIsNotAGrade(t *testing.T) {
	out, err := New().Rollup(context.Background(), RollupInput{Checks: []CheckStatus{
		{ID: "a", Status: "pass"}, {ID: "b", Status: "pass"}, {ID: "c", Status: "fail"},
	}})
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if out.Progress <= 0.66 || out.Progress >= 0.67 {
		t.Errorf("progress = %v, want ~0.666", out.Progress)
	}
	if out.Status == "green" {
		t.Error("2/3 reported green — progress must never substitute for done")
	}
}

// TestRollup_MissingMemberBlocksGreen pins the one rule a milestone's caller
// must honour: a member resolving to no object is passed as a non-pass outcome,
// never omitted. Omitting it shrinks the denominator, so a rollup over 4 of 5
// members reads greener than the truth.
//
// It also records WHY there is no RollupMilestone verb: none is needed. The
// grade of a milestone is just its members' outcomes through this function.
func TestRollup_MissingMemberBlocksGreen(t *testing.T) {
	s := &service{}
	out, err := s.Rollup(context.Background(), RollupInput{Checks: []CheckStatus{
		{ID: "m.UC-007", Status: "pass"},
		{ID: "m.UC-003", Status: "fail"},
		{ID: "m.GHOST", Status: "missing"},
	}})
	if err != nil {
		t.Fatalf("Rollup: %v", err)
	}
	if out.Status == "green" {
		t.Error("a milestone with a missing member must not be green")
	}
	if out.Total != 3 {
		t.Errorf("total = %d — the missing member must count in the denominator", out.Total)
	}
	if len(out.Unmet) != 2 {
		t.Errorf("unmet = %v, want the failure AND the missing member", out.Unmet)
	}
}
