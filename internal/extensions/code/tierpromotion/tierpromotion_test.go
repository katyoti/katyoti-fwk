// SPDX-License-Identifier: Apache-2.0

package tierpromotion

import (
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

func TestEvaluateGates_AllClean_Passes(t *testing.T) {
	out := EvaluateGates(GateInputs{
		CharterVersion: "0.1",
		UCTotal:        5,
	})
	if !out.Passes {
		t.Fatalf("gates should pass; blockers = %v", out.Blockers)
	}
	if len(out.Blockers) != 0 {
		t.Errorf("expected 0 blockers; got %v", out.Blockers)
	}
	// 2 pending entries (lint + drift) until those tasks land.
	if len(out.Pending) != 2 {
		t.Errorf("expected 2 pending entries; got %d: %v", len(out.Pending), out.Pending)
	}
}

func TestEvaluateGates_BlocksOnEveryGate(t *testing.T) {
	cases := []struct {
		name        string
		in          GateInputs
		wantBlocker string
	}{
		{
			name:        "no charter",
			in:          GateInputs{},
			wantBlocker: "charter version < 0.1",
		},
		{
			name:        "UC failing",
			in:          GateInputs{CharterVersion: "0.1", UCTotalFail: 3},
			wantBlocker: "3 UC failing",
		},
		{
			name:        "UC no-test",
			in:          GateInputs{CharterVersion: "0.1", UCTotalNoTest: 2},
			wantBlocker: "2 UC with no test",
		},
		{
			name:        "audit critical",
			in:          GateInputs{CharterVersion: "0.1", AuditCritOpen: 1},
			wantBlocker: "1 critical audit finding",
		},
		{
			name:        "audit high",
			in:          GateInputs{CharterVersion: "0.1", AuditHighOpen: 2},
			wantBlocker: "2 high-severity audit finding",
		},
		{
			name:        "lint broken",
			in:          GateInputs{CharterVersion: "0.1", LintBroken: true},
			wantBlocker: "lint rule violations open",
		},
		{
			name:        "drift",
			in:          GateInputs{CharterVersion: "0.1", DriftFound: true},
			wantBlocker: "API/code-symbol drift detected",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := EvaluateGates(tc.in)
			if out.Passes {
				t.Fatalf("gates should NOT pass for %s", tc.name)
			}
			found := false
			for _, b := range out.Blockers {
				if containsString(b, tc.wantBlocker) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("blocker %q not found in %v", tc.wantBlocker, out.Blockers)
			}
		})
	}
}

func TestEvaluateGates_TracksBlockerKinds(t *testing.T) {
	// UC blocker but no audit blocker.
	out := EvaluateGates(GateInputs{
		CharterVersion: "0.1",
		UCTotalFail:    1,
	})
	if !out.HasUCBlocker {
		t.Error("HasUCBlocker should be true when UC failing")
	}
	if out.HasAuditBlocker {
		t.Error("HasAuditBlocker should be false when no audit findings")
	}

	// Audit blocker but no UC blocker.
	out = EvaluateGates(GateInputs{
		CharterVersion: "0.1",
		AuditCritOpen:  1,
	})
	if out.HasUCBlocker {
		t.Error("HasUCBlocker should be false when UCs clean")
	}
	if !out.HasAuditBlocker {
		t.Error("HasAuditBlocker should be true when critical findings")
	}
}

func TestNextStateAfter(t *testing.T) {
	cases := []struct {
		name    string
		verb    string
		prev    gcapi.TierState
		gates   GateOutcome
		charter string
		ucTotal int
		want    gcapi.TierState
	}{
		{
			name: "plan-tier always → planning",
			verb: "plan-tier",
			want: gcapi.TierStatePlanning,
		},
		{
			name:  "verify with UC blocker → gated",
			verb:  "verify",
			gates: GateOutcome{HasUCBlocker: true},
			want:  gcapi.TierStateGated,
		},
		{
			name: "verify with clean UCs → verifying",
			verb: "verify",
			want: gcapi.TierStateVerifying,
		},
		{
			name:  "audit with audit blocker → gated",
			verb:  "audit",
			gates: GateOutcome{HasAuditBlocker: true},
			want:  gcapi.TierStateGated,
		},
		{
			name: "audit clean → auditing",
			verb: "audit",
			want: gcapi.TierStateAuditing,
		},
		{
			name:  "promote gates pass → promoted",
			verb:  "promote",
			gates: GateOutcome{Passes: true},
			want:  gcapi.TierStatePromoted,
		},
		{
			name:  "promote gates fail → gated",
			verb:  "promote",
			gates: GateOutcome{Passes: false},
			want:  gcapi.TierStateGated,
		},
		{
			name: "no-verb, no charter → bootstrapping",
			verb: "",
			want: gcapi.TierStateBootstrapping,
		},
		{
			name:    "no-verb, charter but no UCs → specifying",
			verb:    "",
			charter: "0.1",
			ucTotal: 0,
			want:    gcapi.TierStateSpecifying,
		},
		{
			name:    "no-verb, has charter + UCs + prev → preserves prev",
			verb:    "",
			charter: "0.1",
			ucTotal: 5,
			prev:    gcapi.TierStateExecuting,
			want:    gcapi.TierStateExecuting,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NextStateAfter(tc.verb, tc.prev, tc.gates, tc.charter, tc.ucTotal)
			if got != tc.want {
				t.Errorf("NextStateAfter = %q want %q", got, tc.want)
			}
		})
	}
}

func containsString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
