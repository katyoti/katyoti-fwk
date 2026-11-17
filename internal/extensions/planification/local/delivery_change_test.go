// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN INCREMENT'S GOALS CHANGE AT ANY TIME — before its proposal is applied,
// a reworded goal's operations not yet applied give way and the proposal is
// planned again for that goal alone, the other operations kept; after it is
// applied, rewording a goal files a follow-up proposal for it, what was made
// kept and still linked to the goal in its new words on the pane; a round's
// reconcile replaces only what it answers, its apply only what waits; a
// person amends one operation before it is applied - never one applied, never
// an agent.
func TestAnIncrementsGoalsChangeAtAnyTime(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shop")
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	accepted(t, ctx, cwm, g)
	paid, mailed := "My customers pay me on time", "My invoices reach them by mail"
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid", Goals: []string{paid, mailed}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	runIncrement(t, ctx, out.Increment.ID)
	in := out.Increment
	if st := stepOf(t, ctx, in.ID, api.StepIntake); st.ID != "" {
		pullAndReport(t, ctx, st.ID, session, schedapi.Result{Summary: "an increment's proposal"})
	}
	s := schedOf(t, ctx)
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	approve := func() {
		t.Helper()
		if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, in.ID, "approve").ID,
			Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
			t.Fatal(err)
		}
	}
	delta := func() api.RequestFields {
		t.Helper()
		got, err := req.Get(ctx, api.GetInput{ID: in.ID})
		if err != nil {
			t.Fatal(err)
		}
		return api.RequestFields{Delta: got.Request.Delta, Applied: got.Request.Applied, Details: got.Request.Details}
	}
	// A round planned again analyses its changed goals first.
	analysed := func() {
		t.Helper()
		if st := stepOf(t, ctx, in.ID, "analyse"); st.ID != "" && notDone(st) {
			if err := report(st.ID, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	setGoals := func(goals ...string) {
		t.Helper()
		if _, err := inc.SetGoals(ctx, api.SetGoalsInput{ID: in.ID, Goals: goals, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	// Run: its first step analyses the goals against the day's state.
	if err := report(stepOf(t, ctx, in.ID, "analyse").ID, nil); err != nil {
		t.Fatal(err)
	}
	if err := report(stepOf(t, ctx, in.ID, api.StepReconcile).ID, map[string]any{api.DeltaKey: []map[string]any{
		{"op": api.OpIncrementTarget, "object": g, "increment": in.ID, "for": paid, "why": "the module already wants it"},
		{"op": api.OpFeatureAdd, "module": "shop", "title": "Mail the invoices", "under": g, "for": mailed, "why": "no module mails them"},
	}}); err != nil {
		t.Fatal(err)
	}
	approve()

	// Before apply: the mail goal reworded - its operation gives way, the
	// proposal is planned again for it alone.
	byPost := "My invoices reach them by post"
	setGoals(paid, byPost)
	d := delta()
	if len(d.Delta) != 1 || d.Delta[0].For != paid {
		t.Fatalf("the delta after rewording a goal before apply: %+v; want the paid operation alone, kept", d.Delta)
	}
	if !strings.Contains(d.Details, byPost) || strings.Contains(d.Details, "- "+paid) {
		t.Errorf("the round's details %q; want the reworded goal alone", d.Details)
	}
	rec := stepOf(t, ctx, in.ID, api.StepReconcile)
	if rec.State != schedapi.StateWaiting && rec.State != schedapi.StateReady {
		t.Fatalf("the reconcile after the change is %s; want planned again", rec.State)
	}
	analysed()
	if err := report(rec.ID, map[string]any{api.DeltaKey: []map[string]any{
		{"op": api.OpFeatureAdd, "module": "shop", "title": "Post the invoices", "under": g, "for": byPost, "why": "no module posts them"},
	}}); err != nil {
		t.Fatal(err)
	}
	if d := delta(); len(d.Delta) != 2 || d.Delta[0].For != paid || d.Delta[1].Title != "Post the invoices" {
		t.Fatalf("the merged delta: %+v; want the kept operation, then the round's", d.Delta)
	}
	approve()
	posted := shopFeature(t, ctx, cwm, g, "Post the invoices")
	apply := stepOf(t, ctx, in.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": g}, {"op": 1, "id": posted}}}); err != nil {
		t.Fatal(err)
	}

	// After apply: the paid goal reworded - a follow-up proposal for it, what
	// was made kept and linked to the goal in its new words.
	onTime := "My customers pay me within thirty days"
	if st := planState(t, ctx, in.ID); st != api.PlanDone {
		t.Fatalf("the proposal after its apply is %s; want done (its last step ends it)", st)
	}
	setGoals(onTime, byPost)
	d = delta()
	if len(d.Delta) != 2 || d.Delta[0].For != onTime || len(d.Applied) != 2 {
		t.Fatalf("after rewording an applied goal: delta %+v applied %+v; want both kept, the first answering %q", d.Delta, d.Applied, onTime)
	}
	if st := planState(t, ctx, in.ID); st == api.PlanDone {
		t.Fatal("no follow-up proposal was filed for the reworded goal")
	}
	page, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "goals"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.DSLTree)
	if !strings.Contains(string(raw), onTime) || !strings.Contains(string(raw), `"data-open":"`+g+`"`) {
		t.Errorf("the pane lost the reworded goal's link; want %q becoming %s in %s", onTime, g, raw)
	}
	// The follow-up is a request of its own: its intake first.
	if st := stepOf(t, ctx, in.ID, api.StepIntake); notDone(st) {
		pullAndReport(t, ctx, st.ID, session, schedapi.Result{Summary: "a follow-up proposal"})
	}
	analysed()
	if err := report(stepOf(t, ctx, in.ID, api.StepReconcile).ID, map[string]any{api.DeltaKey: []map[string]any{
		{"op": api.OpGoalAdd, "module": "shop", "title": "Late payers are reminded", "for": onTime, "why": "thirty days needs reminders"},
		{"op": api.OpFeatureAdd, "module": "shop", "title": "Remind late payers", "under": "#0", "for": onTime, "why": "the reminder itself"},
	}}); err != nil {
		t.Fatal(err)
	}
	if d := delta(); len(d.Delta) != 4 || d.Delta[3].Under != "#2" {
		t.Fatalf("the round's reference after the merge: %+v; want its feature under #2, the goal it adds after the two kept", d.Delta)
	}

	// A person amends the round's operation before it is applied; an applied
	// one, or an agent, is refused.
	if _, err := req.AmendDelta(ctx, api.AmendDeltaInput{ID: in.ID, Op: 3, Title: "Remind the late payers by mail", Why: "clearer", By: person}); err != nil {
		t.Fatal(err)
	}
	if d := delta(); d.Delta[3].Title != "Remind the late payers by mail" {
		t.Errorf("the amended operation: %+v", d.Delta[3])
	}
	if _, err := req.AmendDelta(ctx, api.AmendDeltaInput{ID: in.ID, Op: 1, Title: "x", Why: "y", By: person}); err == nil {
		t.Error("an applied operation was amended")
	}
	if _, err := req.AmendDelta(ctx, api.AmendDeltaInput{ID: in.ID, Op: 3, Title: "x", Why: "y", By: session}); err == nil {
		t.Error("an agent amended a proposal")
	}
	approve()
	reminded := want(t, ctx, cwm, "goal", "Late payers are reminded")
	remind := shopFeature(t, ctx, cwm, reminded, "Remind the late payers by mail")
	again := stepOf(t, ctx, in.ID, api.StepApplyDelta).ID
	if err := report(again, map[string]any{api.AppliedKey: []map[string]any{{"op": 2, "id": reminded}, {"op": 3, "id": remind}}}); err != nil {
		t.Fatalf("the round's apply naming only what waited was sent back: %v", err)
	}
	if d := delta(); len(d.Applied) != 4 {
		t.Errorf("applied after the round: %+v; want the earlier two kept and the round's", d.Applied)
	}

	// A list collapsed to one new goal: refused while
	// the goals it loses are not named in drop; named, they are dropped and
	// said, and no applied operation takes the new goal.
	owed := "A customer sees what they owe"
	if _, err := inc.SetGoals(ctx, api.SetGoalsInput{ID: in.ID, Goals: []string{owed}, By: person}); err == nil ||
		!strings.Contains(err.Error(), onTime) || !strings.Contains(err.Error(), byPost) {
		t.Fatalf("a collapsed list with no drop: %v; want refused naming %q and %q", err, onTime, byPost)
	}
	got, err := inc.SetGoals(ctx, api.SetGoalsInput{ID: in.ID, Goals: []string{owed}, Drop: []string{onTime, byPost}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Dropped) != 2 {
		t.Errorf("the answer names dropped %v; want the two goals", got.Dropped)
	}
	d = delta()
	for _, x := range d.Applied {
		if d.Delta[x.Op].For == owed {
			t.Errorf("an applied operation took the new goal: %+v", d.Delta[x.Op])
		}
	}
}

// shopFeature makes a feature in shop under g, as an apply would.
func shopFeature(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, g, title string) string {
	t.Helper()
	fo := cwmapi.Object{Type: "feature"}
	fo.Parent, fo.Name = "shop", title
	fo.Edges = []cwmapi.ObjectEdge{{Kind: api.EdgeServes, To: g}}
	_ = fo.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shop", Object: fo})
	if err != nil {
		t.Fatal(err)
	}
	return made.Object.ID
}

// notDone is a to-do not yet reported.
func notDone(t api.WorkTodo) bool {
	return t.State == schedapi.StateWaiting || t.State == schedapi.StateReady
}
