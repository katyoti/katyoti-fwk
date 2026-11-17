// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// FOLLOWING A DEMAND END TO END — a follow-up a step files is linked to the
// request that filed it and nested under it; that request, its own steps
// done, waits for it (idle, saying on what) and ends once it has.
func TestAFollowUpIsLinkedAndItsOriginWaitsForIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "built",
		More: map[string]any{"follow_ups": []map[string]any{{"own": true, "title": "Mail the invoices", "module": "billing"}}}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	got, err := req.Demands(ctx, api.DemandsInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var origin *api.Demand
	for i := range got.Demands {
		if got.Demands[i].ID == r.ID {
			origin = &got.Demands[i]
		}
	}
	if origin == nil || len(origin.Parts) != 1 || origin.Parts[0].FollowsUp != r.ID || origin.Parts[0].Title != "Mail the invoices" {
		t.Fatalf("the demand tree: %+v; want the follow-up nested under %s", got.Demands, r.ID)
	}
	follow := origin.Parts[0].ID
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	p, err := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if p.Plan.State != api.PlanIdle || !strings.Contains(p.Plan.Note, follow) {
		t.Fatalf("its own steps done, its follow-up open: %s %q; want idle waiting on %s", p.Plan.State, p.Plan.Note, follow)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: follow, By: person, Note: "mailed"}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("once its follow-up ended: %s; want done", st)
	}
}

// The Request panel shows the whole demand from its root: each part, where it
// stands, this one marked, how far the whole is and where it was split; a
// part's card on the deck says what it is part of.
func TestTheRequestPanelShowsTheWholeDemand(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	big := file(t, w.ctx, w.req, api.FileInput{Title: "Invoices by mail", Module: "billing", By: person})
	split, err := w.req.Split(w.ctx, api.SplitInput{ID: big.ID, By: intake, Note: "one per module", Parts: []api.SplitPart{
		{Module: "billing", Title: "Mail address on invoices"},
		{Module: "shipping", Title: "Mail slot in the parcel"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	here, other := split.Parts[0].ID, split.Parts[1].ID
	views, err := application.UseService[api.ViewService](w.ctx)
	if err != nil {
		t.Fatal(err)
	}
	panel := func() string {
		t.Helper()
		def, err := views.Request(w.ctx, api.RequestViewInput{ID: here})
		if err != nil {
			t.Fatal(err)
		}
		if err := render.ValidatePage(def.DSLTree); err != nil {
			t.Fatalf("the request panel is off the page grammar: %v", err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		return string(raw)
	}
	page := panel()
	for _, want := range []string{
		"The whole demand", `"id":"` + big.ID + `"`, // from its root
		`"id":"` + here + `"`, `"id":"` + other + `"`, `"parent":"` + big.ID + `"`, // each part, nested
		"▸ this request", "0 of 2 ended", "was split by", // this one, how far, where it was split
		`"panel":"request"`, // a click opens one
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the whole demand lacks %s", want)
		}
	}
	if _, err := w.plans.Finish(w.ctx, api.FinishInput{ID: other, Outcome: api.PlanDropped, By: person, Note: "no slot"}); err != nil {
		t.Fatal(err)
	}
	if page := panel(); !strings.Contains(page, "1 of 2 ended") {
		t.Errorf("after a part was dropped, the whole demand does not count it ended")
	}
	if c := deckTab(t, w.ctx, "by-state"); !strings.Contains(c, "part of "+big.ID) {
		t.Errorf("a part's card does not say what it is part of")
	}
}

// An origin's own end waits for its follow-ups however it is asked: finishing
// it by hand is refused while one is open; a follow-up dropped still lets it
// end done (its own work was done); an ended plan refuses a late block.
func TestAnOriginEndsOnlyAfterItsFollowUps(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "built",
		More: map[string]any{"follow_ups": []map[string]any{{"own": true, "title": "Mail the invoices", "module": "billing"}}}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	got, err := req.Demands(ctx, api.DemandsInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	follow := ""
	for _, d := range got.Demands {
		if d.ID == r.ID && len(d.Parts) == 1 {
			follow = d.Parts[0].ID
		}
	}
	if follow == "" {
		t.Fatalf("no follow-up under %s: %+v", r.ID, got.Demands)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, By: person, Note: "done by hand"}); err == nil || !strings.Contains(err.Error(), follow) {
		t.Fatalf("finishing it by hand with its follow-up open: %v; want refused, naming %s", err, follow)
	}
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: follow, Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("its follow-up dropped: %s; want done (its own work was done)", st)
	}
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: r.ID, By: person, Note: "late"}); err == nil {
		t.Error("a block on an ended plan: want refused")
	}
	if p, _ := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: r.ID}); strings.Contains(p.Plan.Note, "late") {
		t.Errorf("a refused block wrote its note on the ended plan: %q", p.Plan.Note)
	}
}

// A FOLLOW-UP NEVER WAITS ON ITS ORIGIN (deadlock) -
// an origin whose own steps are done waits on its follow-ups; one of them
// depending on it would wait for ever. A request filed from another (a
// follow-up, or a part split from one) is not held by a depends-on on it.
func TestAFollowUpNeverWaitsOnItsOrigin(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Write the letter", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "built",
		More: map[string]any{"follow_ups": []map[string]any{{"own": true, "title": "Publish the letter", "module": "billing"}}}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Demands(ctx, api.DemandsInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	follow := ""
	for _, d := range got.Demands {
		if d.ID == r.ID && len(d.Parts) == 1 {
			follow = d.Parts[0].ID
		}
	}
	if follow == "" {
		t.Fatalf("no follow-up under %s", r.ID)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanIdle {
		t.Fatalf("the origin, its own steps done: %s; want idle, waiting on its follow-up", st)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: follow, To: r.ID, Kind: api.EdgeDependsOn}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, follow, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if next := stepOf(t, ctx, follow, "implement"); next.State != schedapi.StateReady {
		t.Fatalf("the follow-up's implement: %s (%s); want ready - its origin waits on it", next.State, next.Waits)
	}
}

// Other work found on the way (a follow-up not marked own) is filed, naming
// where it was found, and never holds the request that filed it: the request
// ends at its last step while that follow-up is still open.
func TestAFollowUpNotTheRequestsOwnWorkDoesNotHoldIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "built",
		More: map[string]any{"follow_ups": []map[string]any{{"title": "The portfolio test fails", "module": "billing", "kind": "fix"}}}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	got, err := req.List(ctx, api.ListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var found *api.Request
	for i := range got.Requests {
		if got.Requests[i].Title == "The portfolio test fails" {
			found = &got.Requests[i]
		}
	}
	if found == nil {
		t.Fatalf("the work found on the way was not filed: %+v", got.Requests)
	}
	if !strings.Contains(found.Details, "Found while working on "+r.ID) {
		t.Errorf("the work found on the way: %q; want it to name where it was found", found.Details)
	}
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("its own steps done, an unrelated follow-up open: %s; want done", st)
	}
	if st := planState(t, ctx, found.ID); planEndedState(st) {
		t.Errorf("the unrelated follow-up ended with the request: %s", st)
	}
}

// planEndedState says whether a plan state is an end.
func planEndedState(st string) bool { return st == api.PlanDone || st == api.PlanDropped }
