// SPDX-License-Identifier: Apache-2.0

package core

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a plan blocked before its block had a kind gets it
// from the note its blocker wrote, read once; a plan not blocked, or already
// carrying its kind, is left as it is; a second pass changes nothing.
func TestABlockedPlanGetsItsKindFromItsNote(t *testing.T) {
	plan := func(id, state, note string) *api.Object {
		return &api.Object{ID: id, Type: "request", Capabilities: map[string]map[string]any{"plan": {"state": state, "note": note}}}
	}
	for _, c := range []struct {
		o    *api.Object
		want string
	}{
		{plan("m.IN-1", "blocked", "its to-dos are done, but m.IN-1 cannot become done: cwm: transition refused"), "move-refused"},
		{plan("m.RQ-1", "blocked", "its to-dos are done, but m.F-1 cannot become realized: refused"), "end-refused"},
		{plan("m.RQ-2", "blocked", `its to-dos are done, but cwm: invalid input: Finish: m.RQ-2 is not done — deliverable "x" is not verified`), "end-refused"},
		{plan("m.RQ-3", "blocked", "a hand test failed: the page is blank"), "hand-test-failed"},
		{plan("m.RQ-4", "blocked", "an agent reviewer sent it back past the cap of 3 rounds (3 already): fix it"), "parked"},
		{plan("m.RQ-5", "blocked", "waiting on the vendor"), "manual"},
	} {
		changed, err := planBlockKind(c.o)
		if err != nil || !changed {
			t.Errorf("%s: not given its kind: %v %v", c.o.ID, changed, err)
			continue
		}
		if got := c.o.Capabilities["plan"]["block"]; got != c.want {
			t.Errorf("%s: block %v, want %s", c.o.ID, got, c.want)
		}
		if again, _ := planBlockKind(c.o); again {
			t.Errorf("%s: the step is not idempotent", c.o.ID)
		}
	}
	for _, o := range []*api.Object{
		plan("m.RQ-6", "ready", "its to-dos are done, but x is not verified"),
		{ID: "m.F-1", Type: "feature", Capabilities: map[string]map[string]any{"feature": {}}},
	} {
		if changed, _ := planBlockKind(o); changed {
			t.Errorf("%s: rewritten though not blocked", o.ID)
		}
	}
	if !slices.ContainsFunc(migrationSteps, func(s MigrationStep) bool {
		return s.Object != nil && s.Pack == "planification" &&
			s.Title == "a blocked plan says why it is blocked: end-refused, move-refused, hand-test-failed, parked or manual"
	}) {
		t.Error("the step is not shipped")
	}
}
