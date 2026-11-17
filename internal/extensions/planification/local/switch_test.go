// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Pausing planning switches the planning task group off — the planner's and
// its steps' — and resuming switches it on; a move names who.
func TestPlanningPausesAndResumesItsGroup(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	out, err := w.plans.Pause(w.ctx, api.PlanningSwitchInput{By: person})
	if err != nil || out.On || out.Group != api.PlanGroup {
		t.Fatalf("Pause = %+v, %v; want the planning group off", out, err)
	}
	if out, err = w.plans.Resume(w.ctx, api.PlanningSwitchInput{By: person}); err != nil || !out.On {
		t.Fatalf("Resume = %+v, %v; want it on", out, err)
	}
	got := w.tasks.setGroups
	if len(got) != 2 || got[0].Group != api.PlanGroup || *got[0].On || !*got[1].On {
		t.Errorf("the group switches = %+v; want planning off then on", got)
	}
	if _, err := w.plans.Pause(w.ctx, api.PlanningSwitchInput{}); err == nil {
		t.Errorf("a pause by nobody: want refused")
	}
}
