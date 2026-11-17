// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A DEMAND RECONCILES — it does no work: its reconcile answers a delta that
// planification checks (an unknown operation or a missing object sends it
// back), keeps on the demand; the person approves it once; the apply answers
// what each operation changed or created (every one, or it goes back); the
// demand ends linked to them (realized-by), its requester told.
func TestADemandReconcilesIntoADeltaThePersonApprovesAndAnAgentApplies(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	d := file(t, ctx, req, api.FileInput{Title: "Customers get their invoices by mail", Module: "billing", Kind: api.KindDemand, By: person})
	if got := strings.Join(steps(t, ctx, d.ID), ","); got != "intake,reconcile,approve,apply" {
		t.Fatalf("a demand's steps: %s; want intake, reconcile, approve, apply", got)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	bad := map[string]any{api.DeltaKey: []map[string]any{{"op": "goal-invent", "object": "billing", "module": "billing", "title": "x", "why": "y"}}}
	if err := report(rec, bad); err == nil {
		t.Fatal("a delta with an unknown operation was collected; want it sent back")
	}
	// An increment is always created in its stream.
	noStream := map[string]any{api.DeltaKey: []map[string]any{{"op": api.OpIncrementCreate, "module": "billing", "title": "By mail", "why": "y"}}}
	if err := report(rec, noStream); err == nil || !strings.Contains(err.Error(), "names the stream") {
		t.Fatalf("an increment-create naming no stream: %v; want it sent back naming the stream", err)
	}
	notStream := map[string]any{api.DeltaKey: []map[string]any{{"op": api.OpIncrementCreate, "module": "billing", "stream": "billing", "title": "By mail", "why": "y"}}}
	if err := report(rec, notStream); err == nil || !strings.Contains(err.Error(), "is not a stream") {
		t.Fatalf("an increment-create naming a module as its stream: %v; want it sent back", err)
	}
	delta := []map[string]any{
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "why": "what the demand asks"},
		{"op": api.OpIncrementCreate, "module": "billing", "stream": streamIn(t, ctx, "billing"), "title": "Customers get their invoices by mail", "why": "its value"},
	}
	if err := report(rec, map[string]any{api.DeltaKey: delta}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Request.Delta) != 2 || got.Request.Delta[0].Op != api.OpFeatureAdd {
		t.Fatalf("the demand's delta: %+v; want the two operations kept", got.Request.Delta)
	}
	// The person approves it once.
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	// The agent applies it: a feature and an increment created.
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	in := increment(t, ctx, "Customers get their invoices by mail")
	apply := stepOf(t, ctx, d.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": f}}}); err == nil {
		t.Fatal("an apply that left an operation unapplied was collected; want it sent back")
	}
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": f}, {"op": 1, "id": in.ID}}}); err != nil {
		t.Fatal(err)
	}
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	var realized []string
	for _, e := range o.Object.Edges {
		if e.Kind == api.EdgeRealizedBy {
			realized = append(realized, e.To)
		}
	}
	slices.Sort(realized)
	want := []string{f, in.ID}
	slices.Sort(want)
	if !slices.Equal(realized, want) {
		t.Errorf("realized-by %v; want %v", realized, want)
	}
	tr, err := inboxOf(t, ctx).Triage(ctx, inboxapi.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	told := slices.IndexFunc(tr.Entries, func(e inboxapi.Entry) bool { return strings.Contains(e.Title, "Your demand "+d.ID) })
	if told < 0 {
		t.Error("the requester was not told what the demand became")
	} else if e := tr.Entries[told]; e.Expects != inboxapi.ExpectsNotice {
		// An information, never an approval asked.
		t.Errorf("what the demand became is told as expecting %q; want a notice", e.Expects)
	}
	if st := planState(t, ctx, d.ID); st != api.PlanDone {
		t.Errorf("the demand: %s; want done", st)
	}
}

// A DEMAND ABOUT AN INCREMENT LANDS WHAT IT MAKES IN IT — its feature
// operations take that increment when they name none, an operation naming
// something that is not an increment is sent back, and once applied the new
// feature is aimed at the increment.
func TestADemandAboutAnIncrementLandsWhatItMakesInIt(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increment(t, ctx, "Invoices reach the customers")
	d := file(t, ctx, req, api.FileInput{Title: "Invoices are also mailed", Module: "billing", Kind: api.KindDemand, About: inc.ID, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	notAnIncrement := []map[string]any{{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "increment": "billing", "why": "y"}}
	if err := report(rec, map[string]any{api.DeltaKey: notAnIncrement}); err == nil {
		t.Fatal("a feature aimed at a module, not an increment, was collected; want it sent back")
	}
	if err := report(rec, map[string]any{api.DeltaKey: []map[string]any{
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "why": "what the demand asks"}}}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Request.Delta) != 1 || got.Request.Delta[0].Increment != inc.ID {
		t.Fatalf("the kept delta: %+v; want its feature-add aimed at %s, the increment the demand is about", got.Request.Delta, inc.ID)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	apply := stepOf(t, ctx, d.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": f}}}); err != nil {
		t.Fatal(err)
	}
	scope, err := increments(t, ctx).Scope(ctx, api.GetInput{ID: inc.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(scope.Targets, func(x api.Target) bool { return x.ID == f }) {
		t.Errorf("the increment holds %+v; want the feature the demand made, %s", scope.Targets, f)
	}
}

// A DELTA BUILDS ON ITSELF — an operation names what an earlier one of the
// same delta makes (#N): a feature under the goal the delta adds, aimed at the
// increment it creates, an increment-target of that goal. A reference to a
// later operation, or to one making the wrong kind of object, is sent back;
// once applied, the reference resolves to the id that operation made.
func TestADeltaNamesWhatItsEarlierOperationsMake(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	d := file(t, ctx, req, api.FileInput{Title: "Customers get their invoices by mail", Module: "billing", Kind: api.KindDemand, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	later := []map[string]any{
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "under": "#1", "why": "y"},
		{"op": api.OpGoalAdd, "module": "billing", "title": "Customers are reached", "why": "y"},
	}
	if err := report(rec, map[string]any{api.DeltaKey: later}); err == nil {
		t.Fatal("a feature under a later operation was collected; want it sent back")
	}
	wrongKind := []map[string]any{
		{"op": api.OpIncrementCreate, "module": "billing", "stream": streamIn(t, ctx, "billing"), "title": "By mail", "why": "y"},
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "under": "#0", "why": "y"},
	}
	if err := report(rec, map[string]any{api.DeltaKey: wrongKind}); err == nil {
		t.Fatal("a feature under an increment-create was collected; want it sent back")
	}
	dropNew := []map[string]any{
		{"op": api.OpGoalAdd, "module": "billing", "title": "Customers are reached", "why": "y"},
		{"op": api.OpRequestDrop, "object": "#0", "why": "y"},
	}
	if err := report(rec, map[string]any{api.DeltaKey: dropNew}); err == nil {
		t.Fatal("a request-drop of what the delta adds was collected; want it sent back")
	}
	delta := []map[string]any{
		{"op": api.OpIncrementCreate, "module": "billing", "stream": streamIn(t, ctx, "billing"), "title": "Customers get their invoices by mail", "why": "its value"},
		{"op": api.OpGoalAdd, "module": "billing", "title": "Customers are reached", "why": "the want"},
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "under": "#1", "increment": "#0", "why": "what the demand asks"},
		{"op": api.OpIncrementTarget, "object": "#1", "increment": "#0", "why": "the goal is delivered there too"},
	}
	if err := report(rec, map[string]any{api.DeltaKey: delta}); err != nil {
		t.Fatalf("a delta naming its earlier operations was sent back: %v", err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	in := increment(t, ctx, "Customers get their invoices by mail")
	g := want(t, ctx, cwm, "goal", "Customers are reached")
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	applied := []map[string]any{{"op": 0, "id": in.ID}, {"op": 1, "id": g}, {"op": 2, "id": f}, {"op": 3, "id": g}}
	apply := stepOf(t, ctx, d.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: applied}); err != nil {
		t.Fatal(err)
	}
	scope, err := increments(t, ctx).Scope(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(scope.Targets, func(x api.Target) bool { return x.ID == f }) {
		t.Errorf("the increment the delta created holds %+v; want %s, aimed at #0", scope.Targets, f)
	}
}

// A REFUSED DELTA NAMES EVERY PROBLEM — not the first only: an unknown
// operation, a module that is not there and a missing why are each named with
// their operation, so the reconcile fixes them in one go.
func TestARefusedDeltaNamesEveryProblem(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	d := file(t, ctx, req, api.FileInput{Title: "Customers get their invoices by mail", Module: "billing", Kind: api.KindDemand, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: rec, By: session}); err != nil {
		t.Fatal(err)
	}
	bad := []map[string]any{
		{"op": "goal-invent", "module": "billing", "title": "x", "why": "y"},
		{"op": api.OpFeatureAdd, "module": "nowhere", "title": "Invoices are mailed", "why": "y"},
		{"op": api.OpGoalAdd, "module": "billing", "title": "Customers are reached"},
	}
	_, err := s.Report(ctx, schedapi.ReportInput{ID: rec, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: map[string]any{api.DeltaKey: bad}}})
	if err == nil {
		t.Fatal("a delta with three problems was collected")
	}
	for _, want := range []string{"3 problems", "delta[0] (goal-invent)", "delta[1] (feature-add)", "delta[2] (goal-add): says why"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal %q does not name %q", err, want)
		}
	}
}

// A REWORKED RECONCILE'S DELTA IS THE ONE APPLIED — the person sends the
// delta back with a note; the reconcile done again answers another delta,
// kept in place of the first; the apply is checked against it. A redone step
// is named "<step> (rework)", and its delta was once dropped for it.
func TestAReworkedReconcileReplacesTheDelta(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	d := file(t, ctx, req, api.FileInput{Title: "Customers get their invoices by mail", Module: "billing", Kind: api.KindDemand, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	first := []map[string]any{
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "why": "what the demand asks"},
		{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are faxed", "why": "a second way"},
	}
	if err := report(stepOf(t, ctx, d.ID, api.StepReconcile).ID, map[string]any{api.DeltaKey: first}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "no fax", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	redo := stepOf(t, ctx, d.ID, api.StepReconcile+" (rework)")
	second := []map[string]any{{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed as PDF", "why": "no fax, the person said"}}
	if err := report(redo.ID, map[string]any{api.DeltaKey: second}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Request.Delta) != 1 || got.Request.Delta[0].Title != "Invoices are mailed as PDF" {
		t.Fatalf("the demand's delta after its rework: %+v; want the reworked one only", got.Request.Delta)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	f := want(t, ctx, cwm, "feature", "Invoices are mailed as PDF")
	apply := stepOf(t, ctx, d.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": f}}}); err != nil {
		t.Fatalf("the apply of the reworked delta: %v", err)
	}
	if st := planState(t, ctx, d.ID); st != api.PlanDone {
		t.Errorf("the demand: %s; want done", st)
	}
}
