// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// DEMANDS AS TREES: a module's demands with their parts nested — a part filed
// in another module too, so the requester follows it there; that module sees
// the part as its own root. A ready demand says why the planner holds it back;
// mine is the demands a person asked; history the ended ones.
func TestDemandsAreTreesWithWhyTheyWait(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	other := cwmapi.Actor{ID: "someone-else", Kind: "person"}
	big := file(t, w.ctx, w.req, api.FileInput{Title: "Invoices by mail", Module: "billing", By: person})
	split, err := w.req.Split(w.ctx, api.SplitInput{ID: big.ID, By: intake, Parts: []api.SplitPart{
		{Module: "billing", Title: "Mail address on invoices"},
		{Module: "shipping", Title: "Mail slot in the parcel"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	blocker := file(t, w.ctx, w.req, api.FileInput{Title: "Mail server", Module: "billing", Kind: api.KindBuild, By: other})
	held := file(t, w.ctx, w.req, api.FileInput{Title: "Reminders by mail", Module: "billing", Kind: api.KindBuild, DependsOn: []string{blocker.ID}, By: other})
	// Planned past its intake — a dependency holds back work, never triage.
	if _, err := workOf(t, w.ctx).Plan(w.ctx, api.PlanWorkInput{Request: held.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	gone := file(t, w.ctx, w.req, api.FileInput{Title: "Fax invoices", Module: "billing", By: person})
	if _, err := w.plans.Finish(w.ctx, api.FinishInput{ID: gone.ID, Outcome: api.PlanDropped, By: person, Note: "nobody faxes"}); err != nil {
		t.Fatal(err)
	}

	demands := func(in api.DemandsInput) []api.Demand {
		t.Helper()
		out, err := w.req.Demands(w.ctx, in)
		if err != nil {
			t.Fatal(err)
		}
		return out.Demands
	}
	find := func(ds []api.Demand, id string) *api.Demand {
		for i := range ds {
			if ds[i].ID == id {
				return &ds[i]
			}
		}
		return nil
	}

	before := len(w.tasks.runs())
	open := demands(api.DemandsInput{Module: "billing"})
	if after := len(w.tasks.runs()); after != before {
		t.Fatalf("reading the demands started %d runs; it must only read", after-before)
	}
	root := find(open, big.ID)
	if root == nil || root.State != api.PlanSplit || len(root.Parts) != 2 {
		t.Fatalf("billing's open demands = %+v; want %s split with its two parts", open, big.ID)
	}
	if find(root.Parts, split.Parts[1].ID) == nil || find(open, split.Parts[0].ID) != nil {
		t.Errorf("parts = %+v; want the shipping part nested under its demand, the billing part not a root of its own", root.Parts)
	}
	// Its state is read from its to-dos: its steps wait on the dependency.
	if h := find(open, held.ID); h == nil || h.State != api.PlanIdle || !strings.Contains(h.Why, blocker.ID) || len(h.WaitsOn) != 1 {
		t.Errorf("the held demand = %+v; want waiting, why naming %s", h, blocker.ID)
	}
	if find(open, gone.ID) != nil {
		t.Errorf("an ended demand is not open")
	}

	if ship := demands(api.DemandsInput{Module: "shipping"}); len(ship) != 1 || ship[0].ID != split.Parts[1].ID || ship[0].SplitFrom != big.ID {
		t.Errorf("shipping's demands = %+v; want the part as its root, split from %s", ship, big.ID)
	}
	if all := demands(api.DemandsInput{}); find(all, split.Parts[1].ID) != nil || find(all, big.ID) == nil {
		t.Errorf("every module's demands: the part is under its demand, not a root")
	}

	mine := demands(api.DemandsInput{Module: "billing", View: api.DemandsMine, Person: person.ID})
	if len(mine) != 1 || mine[0].ID != big.ID {
		t.Errorf("mine = %+v; want only %s (the other ones are someone else's or ended)", mine, big.ID)
	}
	hist := demands(api.DemandsInput{Module: "billing", View: api.DemandsHistory})
	if len(hist) != 1 || hist[0].ID != gone.ID || hist[0].State != api.PlanDropped || hist[0].EndedAt == "" || hist[0].Why != "nobody faxes" {
		t.Errorf("history = %+v; want %s dropped, when and why", hist, gone.ID)
	}
	if _, err := w.req.Demands(w.ctx, api.DemandsInput{View: api.DemandsMine}); err == nil {
		t.Errorf("mine with no person: want refused")
	}
}
