// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// BUILT WORK SHOWS AS BUILT — a feature with no lifecycle state yet, built to
// the end of its request (intake, implement, module-update, verify), is
// recorded realized - built, for a person to confirm - never left stateless
// with its work done.
func TestABuiltFeatureWithNoStateIsRecordedRealized(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "implement", "module-update", "verify"} {
		td := stepOf(t, ctx, f, step)
		by, r := session, schedapi.Result{Summary: step + " done"}
		if td.For == schedapi.ForPerson {
			by, r.Verdict = person, api.VerdictApproved
		}
		pullAndReport(t, ctx, td.ID, by, r)
	}
	if st := planState(t, ctx, f); st != api.PlanDone {
		t.Fatalf("the feature's request: %s; want done (steps left %v)", st, steps(t, ctx, f))
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: f})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := got.Object.Lifecycle(); l.State != "realized" {
		t.Errorf("the built feature's state: %q; want realized (built, a person confirms)", l.State)
	}
}

// A BUILD WHOSE END IS OUT OF REACH SAYS SO AT ONCE — planned on a feature
// still proposed, whose workflow has no way to done from there, the build's
// plan says from the start that it ends with the feature recorded realized;
// planned on a feature with no state, the same. A build on an accepted
// feature says nothing of the kind.
func TestABuildWhoseEndIsOutOfReachSaysSoWhenPlanned(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	note := func(id string) string {
		t.Helper()
		got, err := req.Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if got.Request.Plan == nil {
			return ""
		}
		return got.Request.Plan.Note
	}
	proposed := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: proposed, To: "proposed", By: person, Note: "put forward"}); err != nil {
		t.Fatal(err)
	}
	stateless := want(t, ctx, cwm, "feature", "Print them")
	ok := want(t, ctx, cwm, "feature", "Archive them")
	accepted(t, ctx, cwm, ok)
	for _, f := range []string{proposed, stateless, ok} {
		if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{proposed, stateless} {
		if n := note(f); !strings.Contains(n, "recorded realized") {
			t.Errorf("%s's build plan note %q; want it to say at once that it ends recorded realized", f, n)
		}
	}
	if n := note(ok); strings.Contains(n, "recorded realized") {
		t.Errorf("an accepted feature's build plan note %q; want no out-of-reach warning", n)
	}
}

// A FEATURE IS BORN PROVEN — the build of a feature ends with the checks its
// request verified attached to the feature as its acceptance criteria, each
// once, so what proved the work goes on proving the feature.
func TestABuiltFeatureCarriesTheChecksThatProvedIt(t *testing.T) {
	// Not in parallel: its observer's moves race the other worlds' on a loaded machine.
	ctx, cwm, req := world(t)
	attached = nil
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	check := "go test ./billing/ -run TestPrint"
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{{Title: "Invoices print", Check: check}}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "implement", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, f, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := req.Verify(ctx, api.VerifyInput{ID: f, Deliverable: "Invoices print", Evidence: "it printed", By: person}); err != nil {
		t.Fatal(err)
	}
	td := stepOf(t, ctx, f, "verify")
	by, r := session, schedapi.Result{Summary: "verified"}
	if td.For == schedapi.ForPerson {
		by, r.Verdict = person, api.VerdictApproved
	}
	pullAndReport(t, ctx, td.ID, by, r)
	if len(attached) != 1 || !strings.Contains(attached[0], check) || !strings.Contains(attached[0], f) {
		t.Fatalf("attached at the build's end: %v; want the verified check on %s, once", attached, f)
	}
}

// A FEATURE BUILT IN AN OPEN INCREMENT IS VERIFIED AT ITS END — recorded
// realized as its build ends, it has its verification filed at once in the
// same increment, at the build's priority: an agent's review planned by
// feature-verify, no person's step - so it never stays realized holding its
// increment open. Built outside any increment, it
// files nothing.
func TestAFeatureBuiltInAnOpenIncrementIsVerifiedAtItsEnd(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, Priority: "P1", Increment: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "implement", "module-update", "verify"} {
		td := stepOf(t, ctx, f, step)
		by, r := session, schedapi.Result{Summary: step + " done"}
		if td.For == schedapi.ForPerson {
			by, r.Verdict = person, api.VerdictApproved
		}
		pullAndReport(t, ctx, td.ID, by, r)
	}
	built, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: f})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := built.Object.Lifecycle(); l.State != "realized" {
		t.Fatalf("the built feature's state: %q; want realized", l.State)
	}
	got, err := req.Get(ctx, api.GetInput{ID: f})
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.Kind != api.KindReview || got.Request.Priority != "P1" {
		t.Fatalf("the feature's verification: kind %q priority %q; want a review at the build's P1", got.Request.Kind, got.Request.Priority)
	}
	// The end of the build, heard late, leaves the verification's work.
	time.Sleep(time.Second)
	// The verification's own intake: the build's is done.
	var intake api.WorkTodo
	got2, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: f})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range got2.Todos {
		if td.Packet.Step == api.StepIntake && td.State == schedapi.StateReady {
			intake = td
		}
	}
	if intake.ID == "" {
		t.Fatalf("no ready intake for the verification in %+v", got2.Todos)
	}
	if !slices.Contains(intake.Labels, "increment:"+in.ID) {
		t.Errorf("the verification is not the increment's work: labels %v", intake.Labels)
	}
	pullAndReport(t, ctx, intake.ID, session, schedapi.Result{Summary: "a review"})
	eventually(t, "the verification's verify step", func() (string, bool) {
		names := steps(t, ctx, f)
		return fmt.Sprintf("steps %v, plan %s", names, planState(t, ctx, f)), slices.Contains(names, "verify")
	})
	got3, _ := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: f})
	for _, td := range got3.Todos {
		if td.Packet.Step == "verify" && td.State == schedapi.StateWaiting && td.For == schedapi.ForPerson {
			t.Errorf("the verify step is a person's; a realized feature is verified by an agent")
		}
	}

	// Outside any increment, the same build files nothing.
	g := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: g, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "implement", "module-update", "verify"} {
		td := stepOf(t, ctx, g, step)
		by, r := session, schedapi.Result{Summary: step + " done"}
		if td.For == schedapi.ForPerson {
			by, r.Verdict = person, api.VerdictApproved
		}
		pullAndReport(t, ctx, td.ID, by, r)
	}
	if st := planState(t, ctx, g); st != api.PlanDone {
		t.Errorf("a build outside an increment: its request %s; want done, nothing filed after it", st)
	}
}

// A FEATURE ENDED AGAIN ADDS NO COPY — the criteria a
// feature's earlier end attached (children of the feature, no serves edge, as
// attach-criterion makes them) count as carried: the same Go test, reworded
// with -short or -count=1 or another package spelling, is not attached again;
// only a check no criterion runs yet is.
func TestAFeatureEndedAgainKeepsOneCriterionPerCheck(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	attached = nil
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	ac := cwmapi.Object{Type: "acceptance-criterion", Parent: f, Name: "Invoices print"}
	_ = ac.SetCapability("checkable", map[string]any{"kind": "command", "probe": "go test ./billing/ -run TestPrint -count=1"})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: ac}); err != nil {
		t.Fatal(err)
	}
	again, fresh := "go test -short ./billing -run TestPrint", "go test ./billing/ -run TestMail"
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{{Title: "Invoices print", Check: again}, {Title: "Invoices mail", Check: fresh}}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "implement", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, f, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	for _, d := range []string{"Invoices print", "Invoices mail"} {
		if _, err := req.Verify(ctx, api.VerifyInput{ID: f, Deliverable: d, Evidence: "it ran", By: person}); err != nil {
			t.Fatal(err)
		}
	}
	td := stepOf(t, ctx, f, "verify")
	by, r := session, schedapi.Result{Summary: "verified"}
	if td.For == schedapi.ForPerson {
		by, r.Verdict = person, api.VerdictApproved
	}
	pullAndReport(t, ctx, td.ID, by, r)
	if len(attached) != 1 || !strings.Contains(attached[0], "TestMail") {
		t.Fatalf("attached at the feature's second end: %v; want only the new check (TestMail), the reworded TestPrint carried already", attached)
	}
}

// A DONE FEATURE HAS NOTHING TO REACH — planned again once its feature is done
// (a verify round that ended it, a person who confirmed it), a build's plan
// carries no out-of-reach note: the note written while the feature had no way
// to done is cleared, and none is written anew.
func TestADoneFeaturesPlanCarriesNoOutOfReachNote(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	note := func() string {
		t.Helper()
		got, err := req.Get(ctx, api.GetInput{ID: f})
		if err != nil {
			t.Fatal(err)
		}
		if got.Request.Plan == nil {
			return ""
		}
		return got.Request.Plan.Note
	}
	if n := note(); !strings.Contains(n, "recorded realized") {
		t.Fatalf("a stateless feature's build plan note %q; want the out-of-reach note first", n)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "already in the code"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", By: person, Force: true, Note: "verified by hand"}); err != nil {
		t.Fatal(err)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: f, Again: true, By: person}); err != nil {
		t.Fatal(err)
	}
	if n := note(); strings.Contains(n, "recorded realized") || strings.Contains(n, "no way to done") {
		t.Errorf("a done feature's plan note %q; want no out-of-reach note", n)
	}
}
