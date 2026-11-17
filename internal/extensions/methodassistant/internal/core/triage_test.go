// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// TestTriageAsksTheListerItIsGiven: What's Next's requests to triage come
// from the lister local/ wires (the bus there), asked for the module's ready
// requests, keeping those with no intake step ended since they last became
// ready; without one, none.
func TestTriageAsksTheListerItIsGiven(t *testing.T) {
	var asked planapi.ListInput
	ready := func(at ...string) []cwmapi.LifecycleEntry {
		var out []cwmapi.LifecycleEntry
		for _, a := range at {
			out = append(out, cwmapi.LifecycleEntry{To: planapi.PlanReady, At: a})
		}
		return out
	}
	intake := []planapi.StepRecord{{Task: "intake", EndedAt: "2026-10-01T10:05:00Z"}}
	list := func(_ context.Context, in planapi.ListInput) (planapi.ListOutput, error) {
		asked = in
		return planapi.ListOutput{Requests: []planapi.Request{
			// Filed, its intake not yet run.
			{ID: "m.RQ-001", Plan: &planapi.PlanFields{Trail: ready("2026-10-01T10:00:00Z")}},
			// Its intake ran after it became ready: past triage.
			{ID: "m.RQ-002", Plan: &planapi.PlanFields{Trail: ready("2026-10-01T10:00:00Z"), Steps: intake}},
			{ID: "m.RQ-003"},
			// Filed again after its last intake: to triage again.
			{ID: "m.RQ-004", Plan: &planapi.PlanFields{Trail: ready("2026-10-01T10:00:00Z", "2026-10-02T09:00:00Z"), Steps: intake}},
		}}, nil
	}
	got := New(list).triage(context.Background(), "m")
	if asked.Module != "m" || asked.State != "ready" {
		t.Fatalf("the lister was asked %+v, want module m, state ready", asked)
	}
	if len(got) != 2 || got[0].ID != "m.RQ-001" || got[1].ID != "m.RQ-004" {
		t.Fatalf("triage = %v, want m.RQ-001 and m.RQ-004 (no intake since they became ready)", got)
	}
	if New(nil).triage(context.Background(), "m") != nil {
		t.Fatal("without a lister, triage must show none")
	}
}
