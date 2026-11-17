// SPDX-License-Identifier: Apache-2.0

package modulehealth

import (
	"testing"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
)

func TestDerive(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		want gcapi.ModuleHealth
	}{
		{
			name: "empty inputs → healthy (zero-value module)",
			in:   Inputs{},
			want: gcapi.ModuleHealthHealthy,
		},
		{
			name: "all UCs pass, no findings → healthy",
			in: Inputs{
				UCTotals: gcapi.TestRunTotals{UCTotal: 10, Pass: 10},
			},
			want: gcapi.ModuleHealthHealthy,
		},
		{
			name: "any UC failing → broken",
			in: Inputs{
				UCTotals: gcapi.TestRunTotals{UCTotal: 10, Pass: 9, Fail: 1},
			},
			want: gcapi.ModuleHealthBroken,
		},
		{
			name: "critical audit finding → broken even when UCs pass",
			in: Inputs{
				UCTotals:      gcapi.TestRunTotals{UCTotal: 10, Pass: 10},
				AuditCritOpen: 1,
			},
			want: gcapi.ModuleHealthBroken,
		},
		{
			name: "high audit finding → broken",
			in: Inputs{
				UCTotals:      gcapi.TestRunTotals{UCTotal: 10, Pass: 10},
				AuditHighOpen: 1,
			},
			want: gcapi.ModuleHealthBroken,
		},
		{
			name: "build failing → broken even when UCs pass",
			in: Inputs{
				UCTotals:     gcapi.TestRunTotals{UCTotal: 10, Pass: 10},
				BuildFailing: true,
			},
			want: gcapi.ModuleHealthBroken,
		},
		{
			name: "UCs no-test but no failure → degraded",
			in: Inputs{
				UCTotals: gcapi.TestRunTotals{UCTotal: 10, Pass: 7, NoTest: 3},
			},
			want: gcapi.ModuleHealthDegraded,
		},
		{
			name: "fail-deferred UCs → degraded (not broken)",
			in: Inputs{
				UCTotals: gcapi.TestRunTotals{UCTotal: 10, Pass: 9, FailDeferred: 1},
			},
			want: gcapi.ModuleHealthDegraded,
		},
		{
			name: "lint broken alone → degraded",
			in: Inputs{
				UCTotals:   gcapi.TestRunTotals{UCTotal: 10, Pass: 10},
				LintBroken: true,
			},
			want: gcapi.ModuleHealthDegraded,
		},
		{
			name: "broken trumps degraded (fail + no-test together)",
			in: Inputs{
				UCTotals: gcapi.TestRunTotals{UCTotal: 10, Pass: 5, Fail: 2, NoTest: 3},
			},
			want: gcapi.ModuleHealthBroken,
		},
		{
			name: "broken trumps degraded (critical + lint together)",
			in: Inputs{
				UCTotals:      gcapi.TestRunTotals{UCTotal: 10, Pass: 10},
				AuditCritOpen: 1,
				LintBroken:    true,
			},
			want: gcapi.ModuleHealthBroken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(tt.in)
			if got != tt.want {
				t.Errorf("Derive(%+v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
