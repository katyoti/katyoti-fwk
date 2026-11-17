// SPDX-License-Identifier: Apache-2.0

package local

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

func TestBuildNextStepBlock(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		status    api.ModuleStatusOutput
		wantEmpty bool
		mustHave  []string
		mustNot   []string
	}{
		{
			name: "healthy + at-target + clean → empty (no suggestion)",
			status: api.ModuleStatusOutput{
				Module: api.Module{
					ID:          "mod-1",
					Health:      api.ModuleHealthHealthy,
					TierCurrent: 1,
					TierTarget:  1,
					TierState:   api.TierStatePromoted,
				},
				UCTotals: api.TestRunTotals{UCTotal: 5, Pass: 5},
			},
			wantEmpty: true,
		},
		{
			name: "broken health takes priority",
			status: api.ModuleStatusOutput{
				Module:   api.Module{ID: "mod-1", Health: api.ModuleHealthBroken},
				UCTotals: api.TestRunTotals{UCTotal: 5, Pass: 3, Fail: 2},
			},
			mustHave: []string{"BROKEN", "2 UC failing", "coder"},
		},
		{
			name: "degraded → reviewer-ish suggestion",
			status: api.ModuleStatusOutput{
				Module:   api.Module{ID: "mod-1", Health: api.ModuleHealthDegraded},
				UCTotals: api.TestRunTotals{UCTotal: 5, Pass: 4, NoTest: 1},
			},
			mustHave: []string{"DEGRADED", "module-lint"},
		},
		{
			name: "audit findings open → reviewer",
			status: api.ModuleStatusOutput{
				Module:        api.Module{ID: "mod-1", Health: api.ModuleHealthBroken},
				AuditCritOpen: 1,
				AuditHighOpen: 2,
				UCTotals:      api.TestRunTotals{UCTotal: 5, Pass: 2, Fail: 3},
			},
			mustHave: []string{"reviewer", "1 critical / 2 high"},
		},
		{
			name: "bootstrapping with no charter → specifier or discover",
			status: api.ModuleStatusOutput{
				Module: api.Module{
					ID:        "mod-1",
					Health:    api.ModuleHealthHealthy,
					TierState: api.TierStateBootstrapping,
				},
			},
			mustHave: []string{"bootstrapping", "specifier", "module-discover"},
		},
		{
			name: "gated with blocking gates → list + promote hint",
			status: api.ModuleStatusOutput{
				Module: api.Module{
					ID:        "mod-1",
					Health:    api.ModuleHealthHealthy,
					TierState: api.TierStateGated,
				},
				BlockingGates: []string{"charter version < 0.1", "API contract drift (pending)"},
			},
			mustHave: []string{"promote gate", "module-promote"},
			mustNot:  []string{"BROKEN", "DEGRADED"},
		},
		{
			name: "clean module with gate items pending → list them",
			status: api.ModuleStatusOutput{
				Module: api.Module{
					ID:        "mod-1",
					Health:    api.ModuleHealthHealthy,
					TierState: api.TierStatePromoted,
				},
				UCTotals:      api.TestRunTotals{UCTotal: 5, Pass: 5},
				BlockingGates: []string{"some pending gate"},
			},
			mustHave: []string{"technically clean", "some pending gate", "module-promote"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildNextStepBlock(tc.status)
			if tc.wantEmpty {
				if got != "" {
					t.Errorf("expected empty block; got:\n%s", got)
				}
				return
			}
			if got == "" {
				t.Errorf("expected non-empty block, got empty")
				return
			}
			if !strings.HasPrefix(got, "## Suggested next step") {
				t.Errorf("block missing canonical header. block:\n%s", got)
			}
			for _, sub := range tc.mustHave {
				if !strings.Contains(got, sub) {
					t.Errorf("block missing %q. block:\n%s", sub, got)
				}
			}
			for _, sub := range tc.mustNot {
				if strings.Contains(got, sub) {
					t.Errorf("block should NOT contain %q. block:\n%s", sub, got)
				}
			}
		})
	}
}
