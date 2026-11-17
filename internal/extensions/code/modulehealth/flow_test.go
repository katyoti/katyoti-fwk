// SPDX-License-Identifier: Apache-2.0

package modulehealth

import (
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

// TestDerive_FlipsToBrokenOnVerifyRegression simulates a verify
// run that flips a previously-healthy module to broken: zero
// failing UCs → at-least-one failing UC. This is the precondition
// the REGRESSED cascade keys off (modules/code/concepts.md §4.3).
func TestDerive_FlipsToBrokenOnVerifyRegression(t *testing.T) {
	// Module was healthy: 5 UCs all pass.
	before := Derive(Inputs{
		UCTotals: gcapi.TestRunTotals{UCTotal: 5, Pass: 5},
	})
	if before != gcapi.ModuleHealthHealthy {
		t.Fatalf("before regression: health = %q want healthy", before)
	}

	// Verify run regresses one UC: 4 pass, 1 fail.
	after := Derive(Inputs{
		UCTotals: gcapi.TestRunTotals{UCTotal: 5, Pass: 4, Fail: 1},
	})
	if after != gcapi.ModuleHealthBroken {
		t.Fatalf("after regression: health = %q want broken", after)
	}
}

// TestDerive_RecoveryFromBroken — the reverse-cascade precondition.
// A broken module recovers to healthy/degraded once UCs return to
// pass (or fail-defer) state and no audit findings remain.
func TestDerive_RecoveryFromBroken(t *testing.T) {
	cases := []struct {
		name string
		in   Inputs
		want gcapi.ModuleHealth
	}{
		{
			name: "fail UC fixed, all pass → healthy",
			in:   Inputs{UCTotals: gcapi.TestRunTotals{UCTotal: 5, Pass: 5}},
			want: gcapi.ModuleHealthHealthy,
		},
		{
			name: "fail UC deferred → degraded",
			in: Inputs{UCTotals: gcapi.TestRunTotals{
				UCTotal: 5, Pass: 4, FailDeferred: 1,
			}},
			want: gcapi.ModuleHealthDegraded,
		},
		{
			name: "audit critical cleared but build still failing → broken",
			in: Inputs{
				UCTotals:     gcapi.TestRunTotals{UCTotal: 5, Pass: 5},
				BuildFailing: true,
			},
			want: gcapi.ModuleHealthBroken,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Derive(tc.in); got != tc.want {
				t.Errorf("Derive = %q want %q", got, tc.want)
			}
		})
	}
}
