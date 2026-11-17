// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN INCREMENT IS DEFINED BY ITS DELIVERY GOALS — kept on it in the user's
// words; defined, nothing is planned (define now, plan when run - rule
// ): an agent cannot run it, a person runs it once, and its
// proposal is filed (analyse, reconcile, approve, apply); a delta naming
// a module that is not there is sent back; applied, the increment aims at a
// feature added in another module beside the goal it targeted, its release
// plans their work - or, shipping in none (a periodic or on-change stream's
// increment), it plans its own - and its pane says what each delivery goal
// became.
func TestAnIncrementIsDefinedByItsDeliveryGoals(t *testing.T) {
	t.Parallel()
	t.Run("shipping in a release", func(t *testing.T) { deliveryGoals(t, true) })
	t.Run("in no release", func(t *testing.T) { deliveryGoals(t, false) })
}

func deliveryGoals(t *testing.T, shipped bool) {
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shop")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		gradedVerification{status: map[string]string{}})
	shipsIn := ""
	if shipped {
		shipsIn = plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0"}).ID
	}
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	accepted(t, ctx, cwm, g)
	paid, mailed := "My customers pay me on time", "My invoices reach them by mail"

	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid", ShipsIn: shipsIn,
		Goals: []string{paid, " ", mailed}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	in := out.Increment
	if !slices.Equal(in.Delivery, []string{paid, mailed}) {
		t.Errorf("delivery goals %q; want the two, in the user's words", in.Delivery)
	}
	if got := steps(t, ctx, in.ID); in.Running || len(got) > 0 {
		t.Fatalf("created with goals, it is planned (running %v, steps %v): nothing is planned until a person runs it", in.Running, got)
	}
	if _, err := inc.Run(ctx, api.IncrementRunInput{ID: in.ID, By: session}); err == nil {
		t.Fatal("an agent ran the increment; want only a person")
	}
	ran, err := inc.Run(ctx, api.IncrementRunInput{ID: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if !ran.Increment.Running {
		t.Error("run, the increment does not say it is running")
	}
	if _, err := inc.Run(ctx, api.IncrementRunInput{ID: in.ID, By: person}); err == nil {
		t.Error("an increment ran twice; want the second run refused")
	}
	if got := strings.Join(steps(t, ctx, in.ID), ","); !strings.HasSuffix(got, "analyse,reconcile,approve,apply") {
		t.Fatalf("its proposal's steps: %s; want analyse, reconcile, approve, apply", got)
	}
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
	// Its first step analyses the goals against the day's state.
	if err := report(stepOf(t, ctx, in.ID, "analyse").ID, nil); err != nil {
		t.Fatal(err)
	}
	rec := stepOf(t, ctx, in.ID, api.StepReconcile).ID
	if err := report(rec, map[string]any{api.DeltaKey: []map[string]any{
		{"op": api.OpFeatureAdd, "module": "nowhere", "title": "Mail the invoices", "for": mailed, "why": "y"}}}); err == nil {
		t.Fatal("a delta naming a module that is not there was collected; want it sent back")
	}
	delta := []map[string]any{
		{"op": api.OpIncrementTarget, "object": g, "increment": in.ID, "for": paid, "why": "the module already wants it"},
		{"op": api.OpFeatureAdd, "module": "shop", "title": "Mail the invoices", "under": g, "for": mailed, "why": "no module mails them"},
	}
	if err := report(rec, map[string]any{api.DeltaKey: delta}); err != nil {
		t.Fatal(err)
	}
	// Reconciled, not approved yet: naming the demand does not aim it.
	if early, err := inc.Target(ctx, api.IncrementTargetInput{ID: g, Increment: in.ID, Demand: in.ID, By: person}); err != nil || early.Proposed == "" {
		t.Errorf("before the approval, an aim naming the demand = %+v (%v); want a proposal, nothing aimed", early, err)
	} else if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: early.Proposed, Outcome: api.PlanDropped, By: person, Note: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, in.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}

	// The agent applies it: the goal aimed, a feature added in shop.
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: g, Increment: in.ID, Demand: in.ID}); err != nil {
		t.Fatal(err)
	}
	fo := cwmapi.Object{Type: "feature"}
	fo.Parent, fo.Name = "shop", "Mail the invoices"
	fo.Edges = []cwmapi.ObjectEdge{{Kind: api.EdgeServes, To: g}}
	_ = fo.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shop", Object: fo})
	if err != nil {
		t.Fatal(err)
	}
	f := made.Object.ID
	apply := stepOf(t, ctx, in.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": g}, {"op": 1, "id": f}}}); err != nil {
		t.Fatal(err)
	}

	// The increment aims at what was added; its release planned their work.
	scope, err := inc.Scope(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	var held []string
	for _, x := range scope.Targets {
		held = append(held, x.ID)
	}
	if !slices.Contains(held, g) || !slices.Contains(held, f) {
		t.Errorf("the increment holds %v; want %s and the feature added in shop %s", held, g, f)
	}
	for _, id := range []string{g, f} {
		if got, err := req.Get(ctx, api.GetInput{ID: id}); err != nil || got.Request.ID != id {
			t.Errorf("%s carries no request: its work was not planned (%v)", id, err)
		}
	}
	// Its pane's Goals tab: each delivery goal, what it
	// became - each opening its object - and the request each carries.
	page, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "goals"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.DSLTree)
	for _, want := range []string{paid, mailed, `"data-open":"` + g + `"`, `"data-open":"` + f + `"`, `"data-request":"` + f + `"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the Goals tab lacks %s: %s", want, raw)
		}
	}
	// Its Overview: where it is assigned, its goals and dependency editors.
	if page, err = inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "page"}); err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(page.DSLTree)
	for _, want := range []string{`"label":"Edit goals"`, "ktw-agents:increments/set-goals", `"label":"Add a dependency"`, "ktw-agents:requests/depend"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the pane lacks %s", want)
		}
	}
	assigned := "ships in 1.4.0"
	if !shipped {
		if strings.Contains(string(raw), "ships in ") {
			t.Error("the pane of an increment in no release says it ships in one")
		}
		return
	}
	if !strings.Contains(string(raw), assigned) {
		t.Errorf("the pane does not say where it is assigned (%s)", assigned)
	}
}

// AN INCREMENT GIVEN DELIVERY GOALS STAYS A DEMAND — its intake may set its
// priority and estimate, never make it direct work: an assessment of another
// kind, or of depth quick, is refused and its proposal's steps stay planned.
func TestAnIncrementWithDeliveryGoalsStaysADemand(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	out, err := increments(t, ctx).Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid",
		Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Increment.ID
	runIncrement(t, ctx, id)
	for _, in := range []api.AssessInput{
		{ID: id, Kind: api.KindBuild, By: session},
		{ID: id, Kind: api.KindFix, Depth: api.DepthQuick, By: session},
		{ID: id, Depth: api.DepthQuick, By: session},
	} {
		if _, err := req.Assess(ctx, in); err == nil {
			t.Errorf("assessed %s kind %q depth %q: accepted; want it refused - it carries delivery goals", id, in.Kind, in.Depth)
		}
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: id, Kind: api.KindDemand, Priority: "P1", Estimate: 2, By: session}); err != nil {
		t.Fatalf("assessed as a demand: %v", err)
	}
	if got := strings.Join(steps(t, ctx, id), ","); !strings.HasSuffix(got, "reconcile,approve,apply") {
		t.Errorf("its proposal's steps: %s; want reconcile, approve, apply", got)
	}
}

// A RELEASE RUNS ITS INCREMENTS — a person's go: each increment shipping in it
// that has delivery goals and has not run is run (its proposal filed); one
// with no goals is skipped, saying why; an agent cannot run a release.
func TestARunReleaseRunsItsIncrementsThatCanRun(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	rel := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "2.0.0"}).ID
	inc := increments(t, ctx)
	with, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Paid on time", ShipsIn: rel, Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	bare, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: "Not described yet", ShipsIn: rel, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := releases(t, ctx).Run(ctx, api.ReleaseRunInput{ID: rel, By: session}); err == nil {
		t.Fatal("an agent ran a release; want only a person")
	}
	out, err := releases(t, ctx).Run(ctx, api.ReleaseRunInput{ID: rel, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Ran, []string{with.Increment.ID}) {
		t.Errorf("ran %v; want only %s", out.Ran, with.Increment.ID)
	}
	if why := out.Skipped[bare.Increment.ID]; !strings.Contains(why, "no delivery goals") {
		t.Errorf("the increment with no goals: %q; want it skipped for its missing goals", why)
	}
	if got := strings.Join(steps(t, ctx, with.Increment.ID), ","); !strings.HasSuffix(got, "analyse,reconcile,approve,apply") {
		t.Errorf("the run increment's steps: %s; want its proposal", got)
	}
	if got := steps(t, ctx, bare.Increment.ID); len(got) > 0 {
		t.Errorf("the increment not run has steps %v", got)
	}
}

// runIncrement is the person's go for an increment created with its goals
// (define now, plan when run): its proposal is filed.
func runIncrement(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	if _, err := increments(t, ctx).Run(ctx, api.IncrementRunInput{ID: id, By: person}); err != nil {
		t.Fatalf("run %s: %v", id, err)
	}
}

// : an increment keeps the priority a person gave it when
// an agent assesses it again - the intake of a reconcile; its first intake
// sets one, and a person still moves it.
func TestAnIncrementKeepsThePriorityAPersonGaveIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	out, err := increments(t, ctx).Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid",
		Goals: []string{"My customers pay me on time"}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	id := out.Increment.ID
	runIncrement(t, ctx, id)
	priority := func() string {
		t.Helper()
		got, err := increments(t, ctx).Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		return got.Increment.Priority
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: id, Kind: api.KindDemand, Priority: "P2", Estimate: 2, By: session}); err != nil {
		t.Fatal(err)
	}
	if p := priority(); p != "P2" {
		t.Fatalf("its first intake set %q; want P2", p)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: id, Priority: "P0", By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: id, Kind: api.KindDemand, Priority: "P2", Estimate: 1, By: session}); err != nil {
		t.Fatal(err)
	}
	if p := priority(); p != "P0" {
		t.Fatalf("an agent's intake moved the person's priority to %q; want P0", p)
	}
}
