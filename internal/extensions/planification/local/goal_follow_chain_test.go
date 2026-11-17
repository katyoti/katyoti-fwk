// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A GOAL'S FOLLOW WAITS ON ITS FEATURES, NEVER THE REVERSE
// : an increment holds a goal and the two features
// serving it, as an applied proposal leaves them. Its work is planned: the
// goal's follow (goal-follow) is out of plan-work's chain - no feature
// depends on it, the second still on the first - and depends on each
// feature's work. Its follow waits while that work is open; reported done
// once the features' builds ended (realized) it does not end the goal's
// request while a feature is not done; the person confirms them and the
// goal's request ends. An agent that reports its follow blocked instead - a
// feature not done yet - has it offered again once every one is done.
func TestAGoalsFollowWaitsOnItsFeatures(t *testing.T) {
	t.Parallel()
	t.Run("reported done", func(t *testing.T) { goalFollow(t, false) })
	t.Run("reported blocked", func(t *testing.T) { goalFollow(t, true) })
}

func goalFollow(t *testing.T, blocked bool) {
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "A person tags a note and lists the notes of a tag")
	g := want(t, ctx, cwm, "goal", "Notes are organised by tags")
	f1 := want(t, ctx, cwm, "feature", "Tag a note in one command", cwmapi.ObjectEdge{Kind: api.EdgeServes, To: g})
	f2 := want(t, ctx, cwm, "feature", "List the notes of a tag", cwmapi.ObjectEdge{Kind: api.EdgeServes, To: g})
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: g, Increment: in.ID}); err != nil {
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
	if !slices.Contains(filed, g) || !slices.Contains(filed, f1) || !slices.Contains(filed, f2) {
		t.Fatalf("filed %v; want the goal's follow and both features' work", filed)
	}
	deps := func(id string) []string {
		t.Helper()
		o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, e := range o.Object.Edges {
			if e.Kind == api.EdgeDependsOn {
				out = append(out, e.To)
			}
		}
		return out
	}
	if d := deps(f1); slices.Contains(d, g) {
		t.Errorf("%s depends on its goal's follow (%v): the goal waits on it", f1, d)
	}
	if d := deps(f2); !slices.Contains(d, f1) {
		t.Errorf("%s depends on %v; want the chain kept between the features (on %s)", f2, d, f1)
	}
	if d := deps(g); !slices.Contains(d, f1) || !slices.Contains(d, f2) {
		t.Errorf("the goal's follow depends on %v; want each feature's work (%s, %s)", d, f1, f2)
	}

	pullAndReport(t, ctx, stepOf(t, ctx, g, api.StepIntake).ID, session, schedapi.Result{Summary: "a goal"})
	if st := stepOf(t, ctx, g, "follow"); st.State == schedapi.StateReady {
		t.Fatalf("the goal's follow is ready while its features' work is open; want it waiting")
	}
	build := func(f string) {
		t.Helper()
		pullAndReport(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, session, schedapi.Result{Summary: "a feature"})
		for _, step := range []string{"implement", "module-update"} {
			pullAndReport(t, ctx, stepOf(t, ctx, f, step).ID, session, schedapi.Result{Summary: step + " done"})
		}
		pullAndReport(t, ctx, stepOf(t, ctx, f, "verify").ID, session, schedapi.Result{Summary: "built"})
	}
	build(f1)
	build(f2)
	// The features' builds ended (recorded realized, a person confirms them):
	// the follow is takeable now; an agent reports it - done early, or
	// blocked on the features not done yet.
	follow := schedapi.Result{Summary: "its features are built"}
	if blocked {
		follow = schedapi.Result{Status: schedapi.StatusBlocked, Summary: "its features are not done yet"}
	}
	pullAndReport(t, ctx, stepOf(t, ctx, g, "follow").ID, session, follow)
	if st := planState(t, ctx, g); st == api.PlanDone {
		t.Fatalf("the goal's request ended while %s and %s are not done", f1, f2)
	}
	for _, f := range []string{f1, f2} {
		if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
			t.Fatalf("the person confirms %s: %v", f, err)
		}
	}
	if blocked {
		// Offered again once its features are done: reported done, it ends.
		for i := 0; i < 250 && stepOf(t, ctx, g, "follow").State != schedapi.StateReady; i++ {
			time.Sleep(40 * time.Millisecond)
		}
		st := stepOf(t, ctx, g, "follow")
		if st.State != schedapi.StateReady {
			t.Fatalf("the follow reported blocked, its features now done: %s (%q); want it offered again", st.State, st.Waits)
		}
		pullAndReport(t, ctx, st.ID, session, schedapi.Result{Summary: "its features are done"})
	}
	for i := 0; i < 250 && planState(t, ctx, g) != api.PlanDone; i++ {
		time.Sleep(40 * time.Millisecond)
	}
	if st := planState(t, ctx, g); st != api.PlanDone {
		t.Errorf("the goal's request once its features are done: %s; want done", st)
	}

}
