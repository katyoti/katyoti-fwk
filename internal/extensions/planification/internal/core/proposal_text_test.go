// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT'S PROPOSAL SAYS WHO IT WAITS ON -
// approved by the person and not yet applied, it says its operations await
// their apply, never the person's approval; reconciled and not approved (or
// sent back, or reconciled again for a new round), it awaits the approval.
func TestAnApprovedProposalAwaitsItsApplyNotTheApproval(t *testing.T) {
	step := func(name, outcome, result string) map[string]any {
		return map[string]any{"step": "increment-delivery/" + name, "started_at": "2026-11-06T09:00:00Z",
			"outcome": outcome, "result": result}
	}
	reconciled := step(api.StepReconcile, "success", "reconciled")
	approved := step("approve", "success", api.VerdictApproved+": ok")
	sentBack := step("approve", "success", api.VerdictRejected+": change goal 2")
	inc := func(applied int, steps ...map[string]any) cwmapi.Object {
		delta := []any{}
		for range 3 {
			delta = append(delta, map[string]any{"op": api.OpDirect, "for": "a goal", "why": "x"})
		}
		done := []any{}
		for i := range applied {
			done = append(done, map[string]any{"op": i, "id": "m.RQ-1"})
		}
		ss := []any{}
		for _, s := range steps {
			ss = append(ss, s)
		}
		return cwmapi.Object{Type: api.IncrementType, Capabilities: map[string]map[string]any{
			api.CapRequest: {"delta": delta, "applied": done},
			api.CapPlan:    {"state": api.PlanReady, "steps": ss},
		}}
	}
	for _, c := range []struct {
		name      string
		o         cwmapi.Object
		held      bool
		want, not string
	}{
		{"reconciled, not approved", inc(0, reconciled), false, "3 operation(s) await your approval", "apply"},
		{"approved, nothing applied", inc(0, reconciled, approved), false, "approved - 3 operation(s) await their apply", "approval"},
		{"approved, its apply held", inc(0, reconciled, approved), true, "await their apply: its work waits for the increment to run", "approval"},
		{"approved, partly applied", inc(1, reconciled, approved), false, "approved - 2 operation(s) await their apply", "approval"},
		{"sent back, reconciled again", inc(0, reconciled, sentBack, reconciled), false, "await your approval", "apply"},
		{"approved, then a new round reconciled", inc(0, reconciled, approved, reconciled), false, "await your approval", "apply"},
		{"approved and applied", inc(3, reconciled, approved), false, "approved and applied", "await"},
	} {
		got := proposalText(c.o, "en", c.held)
		if !strings.Contains(got, c.want) || strings.Contains(got, c.not) {
			t.Errorf("%s: proposal says %q; want %q, never %q", c.name, got, c.want, c.not)
		}
	}
	if got := proposalText(inc(0, reconciled, approved), "fr", false); got != "validé - 3 opération(s) attendent leur application" {
		t.Errorf("in French the approved proposal says %q", got)
	}
}
