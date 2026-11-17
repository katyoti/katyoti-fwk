// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// AN INCREMENT PLANS ITS OWN WORK — one of a periodic or on-change stream
// ships in no release until it is delivered: increments plan-work walks it
// alone, as a release walks its increments. A feature it holds gets its
// specification, labelled with the increment; run again it files nothing; a
// dropped increment plans nothing.
func TestAnIncrementInNoReleasePlansItsOwnWork(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	accepted(t, ctx, cwm, f)
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		gradedVerification{status: map[string]string{}})

	if _, err := releases(t, ctx).PlanWork(ctx, api.ReleasePlanWorkInput{ID: in.ID, By: person}); err == nil {
		t.Error("releases plan-work on an increment: accepted; want it refused (increments plan-work)")
	}
	out, err := inc.PlanWork(ctx, api.IncrementPlanWorkInput{ID: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Filed) != 1 || out.Filed[0].Object != f {
		t.Fatalf("filed %v; want the feature's specification", out.Filed)
	}
	if l := stepOf(t, ctx, f, api.StepIntake).Labels; !slices.Contains(l, "increment:"+in.ID) {
		t.Errorf("the feature's intake: labels %v; want increment:%s", l, in.ID)
	}
	again, err := inc.PlanWork(ctx, api.IncrementPlanWorkInput{ID: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Filed) != 0 || len(again.Reused) != 1 {
		t.Errorf("run again: filed %v, reused %v; want nothing new, the open work reused", again.Filed, again.Reused)
	}
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.PlanWork(ctx, api.IncrementPlanWorkInput{ID: in.ID, By: person}); err == nil {
		t.Error("a dropped increment planned its work; want it refused")
	}
}

// A GOAL THE PROPOSAL ADDED IS DECOMPOSED — an increment holding a goal still
// in its first state (the approved proposal that added it is its acceptance)
// plans its decomposition, as it specifies such a feature; a dropped goal
// gets nothing.
func TestAnIncrementDecomposesAGoalItsProposalAdded(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Customers keep their tasks in a file")
	g := want(t, ctx, cwm, "goal", "A person keeps their tasks in a file")
	dropped := want(t, ctx, cwm, "goal", "Tasks sync to a phone")
	for _, id := range []string{g, dropped} {
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: id, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if err := move(ctx, cwm, dropped, "dropped", person); err != nil {
		t.Fatal(err)
	}
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		gradedVerification{status: map[string]string{}})
	out, err := inc.PlanWork(ctx, api.IncrementPlanWorkInput{ID: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	var filed []string
	for _, x := range out.Filed {
		filed = append(filed, x.Object)
	}
	if !slices.Contains(filed, g) {
		t.Errorf("filed %v; want the decomposition of %s, a goal in its first state", filed, g)
	}
	if slices.Contains(filed, dropped) {
		t.Errorf("filed %v; want nothing for the dropped goal %s", filed, dropped)
	}
}
