// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A RELEASE PLANS ITS WORK FROM THE GAP — plan-work walks the increments
// shipping in it: a goal not decomposed gets its decomposition, a feature not
// specified its specification, a ready one failing its criteria its build, a
// passing one nothing; two requests in one module are ordered; run again it
// files nothing; a feature in no release plans nothing.
func TestAReleasePlansItsIncrementsWorkFromTheGap(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	accepted(t, ctx, cwm, g)
	// A goal a proposal added, not accepted: its request is its follow
	// (goal-follow), out of the chain.
	followed := want(t, ctx, cwm, "goal", "Customers trust their invoices")
	toSpecify := want(t, ctx, cwm, "feature", "Remind late payers")
	accepted(t, ctx, cwm, toSpecify)
	toBuild := specified(t, ctx, cwm)
	planlocal.Settle(ctx, 5*time.Second)
	if _, err := gatesOf(t, ctx).Confirm(ctx, gatesapi.ConfirmInput{ID: toBuild, Gate: api.GateReadyToBuild, Requirement: "feature-challenged", By: person}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, toBuild, "ready", intake); err != nil {
		t.Fatal(err)
	}
	passing := want(t, ctx, cwm, "feature", "Print invoices")
	greenNotDone := want(t, ctx, cwm, "feature", "Number invoices")
	accepted(t, ctx, cwm, greenNotDone)
	outside := want(t, ctx, cwm, "feature", "Archive invoices")
	accepted(t, ctx, cwm, outside)
	for _, id := range []string{g, followed, toSpecify, toBuild, passing, greenNotDone} {
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: id, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
	}
	status := map[string]string{passing: "pass", greenNotDone: "pass", toBuild: "fail"}
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default", planVerification(status))
	verifiedDone(t, ctx, cwm, passing)

	out, err := rel.PlanWork(ctx, api.ReleasePlanWorkInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	objects := func(items []api.GapItem) []string {
		var ids []string
		for _, it := range items {
			ids = append(ids, it.Object)
		}
		slices.Sort(ids)
		return ids
	}
	// A feature whose criteria pass but which is not done is not passing
	// : accepted, it gets its specification.
	wantFiled := []string{g, followed, toSpecify, toBuild, greenNotDone}
	slices.Sort(wantFiled)
	if got := objects(out.Filed); !slices.Equal(got, wantFiled) {
		t.Errorf("filed on %v; want the goal, the features to specify (one green, not done) and the one to build %v", got, wantFiled)
	}
	if got := objects(out.Passing); !slices.Equal(got, []string{passing}) {
		t.Errorf("passing %v; want %s", got, passing)
	}
	// One module: the later requests wait on the earlier - the goal's
	// decomposition among them - but a goal's follow, which waits on its
	// features, is in no chain.
	prev := ""
	for _, f := range out.Filed {
		got, err := req.Get(ctx, api.GetInput{ID: f.Request})
		if err != nil {
			t.Fatal(err)
		}
		if f.Object == followed {
			if len(got.Request.DependsOn) != 0 {
				t.Errorf("the goal's follow %s waits on %v: it is in no chain (it has no feature)", f.Request, got.Request.DependsOn)
			}
			continue
		}
		if prev != "" && !slices.Contains(got.Request.DependsOn, prev) {
			t.Errorf("%s does not wait on %s: two requests in one module are ordered", f.Request, prev)
		}
		if slices.Contains(got.Request.DependsOn, followed) {
			t.Errorf("%s waits on the goal's follow %s: it is in no chain", f.Request, followed)
		}
		prev = f.Request
	}
	// A feature in no release: nothing filed on it.
	if got, _ := req.Get(ctx, api.GetInput{ID: outside}); got.Request.ID == outside {
		t.Errorf("%s, in no release, carries a request", outside)
	}
	// Run again: nothing new, the open work reused.
	again, err := rel.PlanWork(ctx, api.ReleasePlanWorkInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	// (The followed goal, with no lifecycle state and its follow open, reads
	// its plan's state: neither filed again nor listed.)
	if len(again.Filed) != 0 || len(again.Reused) != 4 {
		t.Errorf("run again: filed %v, reused %v; want nothing new, four reused", objects(again.Filed), objects(again.Reused))
	}
}

// A GREEN FEATURE NOT DONE IS PLANNED TO ITS END -
// plan-work never reads a feature whose criteria pass but which is not done
// as passing: realized, it is verified at once when an agent may (its own
// criterion passed, nothing tested by hand); realized with no criterion of
// its own, it gets its verify round (a review, feature-verify) in the
// increment; so no increment hangs on a green feature left not done.
func TestAGreenFeatureNotDoneIsPlannedToItsEnd(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := criterionWorld(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
	in := increment(t, ctx, "Invoices are numbered")
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	unproven := want(t, ctx, cwm, "feature", "Number the invoices")
	verifiable := want(t, ctx, cwm, "feature", "Date the invoices")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		planVerification(map[string]string{unproven: "pass", verifiable: "pass"}))
	// Its criterion's verdict is in before it is recorded realized: nothing
	// verified it on the way.
	crit := verdictCriterion(t, ctx, cwm, verifiable, "The invoices are dated", "pass", "dated")
	for _, f := range []string{unproven, verifiable} {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "already in the code"}); err != nil {
			t.Fatal(err)
		}
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
	}

	out, err := rel.PlanWork(ctx, api.ReleasePlanWorkInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	var passing []string
	for _, it := range out.Passing {
		passing = append(passing, it.Object)
	}
	if !slices.Equal(passing, []string{verifiable}) {
		t.Errorf("passing %v; want only %s, verified at once on %s", passing, verifiable, crit)
	}
	if st := stateOf(t, ctx, cwm, verifiable); st != "done" {
		t.Errorf("%s, realized with its criterion passed: %q; want done", verifiable, st)
	}
	var round string
	for _, it := range out.Filed {
		if it.Object == unproven && strings.Contains(it.Why, "verify round") {
			round = it.Request
		}
	}
	if round == "" {
		t.Fatalf("filed %+v; want the verify round of %s (realized, green, no criterion of its own)", out.Filed, unproven)
	}
	got, err := req.Get(ctx, api.GetInput{ID: round})
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.Kind != api.KindReview {
		t.Errorf("the verify round %s is a %q; want a review", round, got.Request.Kind)
	}
	if st := stateOf(t, ctx, cwm, unproven); st != "realized" {
		t.Errorf("%s with no criterion of its own: %q; want still realized, for its verify round", unproven, st)
	}
	again, err := rel.PlanWork(ctx, api.ReleasePlanWorkInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Filed) != 0 {
		t.Errorf("run again: filed %+v; want its verify round reused", again.Filed)
	}
}

// planVerification answers plan-work's grading (Verify) and the gates'
// verify() (VerifyFunction) by object, from one status map.
func planVerification(status map[string]string) verificationapi.VerificationService {
	return planVerifier{graded: gradedVerification{status: status}, function: functionVerification{status: status}}
}

type planVerifier struct {
	verificationapi.VerificationService
	graded   gradedVerification
	function functionVerification
}

func (v planVerifier) Verify(ctx context.Context, in verificationapi.VerifyInput) (verificationapi.VerifyOutput, error) {
	return v.graded.Verify(ctx, in)
}

func (v planVerifier) VerifyFunction(ctx context.Context, in cwmapi.FunctionCall) (verificationapi.VerifySummary, error) {
	return v.function.VerifyFunction(ctx, in)
}

func (v planVerifier) LintFunction(ctx context.Context, in cwmapi.FunctionCall) (verificationapi.LintSummary, error) {
	return v.function.LintFunction(ctx, in)
}

// verifiedDone records a feature realized, then done by an agent's verify.
func verifiedDone(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) {
	t.Helper()
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "realized", By: session, Note: "already in the code"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "done", Transition: "verify", By: session, Note: "its acceptance criteria pass"}); err != nil {
		t.Fatal(err)
	}
}

// A RELEASE PLANS ONLY WHAT RAN — define now, plan when run: of the increments
// shipping in it, one defined by its delivery goals and not run waits for its
// run (answered waiting, nothing filed for what it holds); one aimed by hand
// (no delivery goals) and one that ran are planned.
func TestAReleasePlansOnlyTheIncrementsThatRan(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "3.0.0"})
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		gradedVerification{status: map[string]string{}})
	holding := func(title string, goals ...string) (string, string) {
		t.Helper()
		out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Title: title, ShipsIn: r.ID, Goals: goals, By: person})
		if err != nil {
			t.Fatal(err)
		}
		f := want(t, ctx, cwm, "feature", "Feature of "+title)
		accepted(t, ctx, cwm, f)
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: out.Increment.ID}); err != nil {
			t.Fatal(err)
		}
		return out.Increment.ID, f
	}
	defined, definedF := holding("Only defined", "My customers pay me on time")
	ran, ranF := holding("Run", "My invoices reach them by mail")
	runIncrement(t, ctx, ran)
	_, byHandF := holding("Aimed by hand")

	out, err := rel.PlanWork(ctx, api.ReleasePlanWorkInput{ID: r.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	filed := map[string]bool{}
	for _, it := range out.Filed {
		filed[it.Object] = true
	}
	if filed[definedF] {
		t.Errorf("the feature of the increment only defined was planned: %+v", out.Filed)
	}
	if !filed[ranF] || !filed[byHandF] {
		t.Errorf("filed %+v; want the features of the run increment (%s) and of the one aimed by hand (%s)", out.Filed, ranF, byHandF)
	}
	var waits bool
	for _, it := range out.Waiting {
		waits = waits || (it.Object == defined && strings.Contains(it.Why, "not run"))
	}
	if !waits {
		t.Errorf("waiting %+v; want %s waiting for its run", out.Waiting, defined)
	}
}
