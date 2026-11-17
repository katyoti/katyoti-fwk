// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A BUILT SCREEN FEATURE FREES WHAT DEPENDS ON IT WHEREVER ITS TEST WAITS
// (rule): A realized, its unit
// criterion passed, its live GUI test not run. C depends on A: C's work is
// ready at once - no batch holds the test yet - and stays ready once the test
// is batched in another increment's request; the depends-on edge is kept. A's
// verify round waits on that batch, wherever it is. Not yet built (its unit
// criterion not passed), A holds C.
func TestABuiltScreenFeatureFreesItsDependentsWhereverItsTestWaits(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	other := increment(t, ctx, "The end batch of the release")
	script := "app/ktw-gui/frontend/tests/live/invoice-lines.sh"
	a := want(t, ctx, cwm, "feature", "The invoice pane lists its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceLines.tsx"})
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: a, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	unit := criterionFor(t, ctx, cwm, "billing", a, "go test ./internal/extensions/billing/...")
	criterionFor(t, ctx, cwm, "billing", a, "bash "+script)

	if _, err := req.File(ctx, api.FileInput{On: a, Kind: api.KindReview, Increment: in.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, a, api.StepIntake).ID, session, schedapi.Result{Summary: "a review"})

	c := file(t, ctx, req, api.FileInput{Title: "Total the invoice from its lines", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: a}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, c.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	if st := stepOf(t, ctx, c.ID, "implement"); st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, a) {
		t.Fatalf("A's unit criterion has not passed - A is not built: the fix of C %s (%q); want waiting on %s", st.State, st.Waits, a)
	}

	passVerdict(t, ctx, cwm, "billing", unit)
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: a}); err != nil { // settles C again
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, c.ID, "implement"); st.State != schedapi.StateReady {
		t.Fatalf("A is built, its GUI test in no batch yet: the fix of C %s (%q); want ready", st.State, st.Waits)
	}

	title := "The GUI test of the invoice lines by element ids"
	b := file(t, ctx, req, api.FileInput{Title: "Run the release's GUI tests together", Module: "billing", Kind: api.KindBuild, Increment: other.ID,
		Deliverables: []api.Deliverable{{Title: title, Check: "bash " + script}}})
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: a}); err != nil {
		t.Fatal(err)
	}
	if st := stepOf(t, ctx, c.ID, "implement"); st.State != schedapi.StateReady {
		t.Fatalf("A's GUI test batched in another increment's request: the fix of C %s (%q); want ready", st.State, st.Waits)
	}
	if st := stepOf(t, ctx, a, "verify"); st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, b.ID) {
		t.Fatalf("A's verify round while its test waits in %s: %s (%q); want waiting on it", b.ID, st.State, st.Waits)
	}
	co, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	kept := false
	for _, e := range co.Object.Edges {
		kept = kept || (e.Kind == api.EdgeDependsOn && e.To == a)
	}
	if !kept {
		t.Fatalf("C's depends-on %s was removed; the order is kept", a)
	}
	if st := lifeOf(t, ctx, cwm, a); st != "realized" {
		t.Fatalf("A is %s; a built feature stays realized until its GUI test passed", st)
	}
}

// AN INCREMENT WAITS FOR ITS BUILT FEATURE'S GUI TEST:
// A, a built screen feature of the increment (its unit criterion passed, its
// live GUI test not run), frees what depends on it but not its increment: the
// increment refuses to end while A is realized, and ends once A's GUI test
// passed and A moved done.
func TestAnIncrementWaitsForItsBuiltFeaturesGUITest(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	a := want(t, ctx, cwm, "feature", "The invoice pane shows its total", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceTotal.tsx"})
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: a, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	passVerdict(t, ctx, cwm, "billing", criterionFor(t, ctx, cwm, "billing", a, "go test ./internal/extensions/billing/..."))
	live := criterionFor(t, ctx, cwm, "billing", a, "bash app/ktw-gui/frontend/tests/live/invoice-total.sh")

	if _, err := increments(t, ctx).Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err == nil || !strings.Contains(err.Error(), a) {
		t.Fatalf("the increment ended while %s waits for its GUI test (err %v); want it refused naming %s", a, err, a)
	}

	// Its live verdict passes: every criterion of A passed, nothing tested by
	// hand - planification's observer verifies it without the person. The
	// test does not move it itself: the observer runs on its own goroutine and
	// would race that move (« no transition from done to done » under load,
	//
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{a: "pass"}})
	passVerdict(t, ctx, cwm, "billing", live)
	eventually(t, "A done once its GUI test passed, verified without the person", func() (string, bool) {
		st := stateOf(t, ctx, cwm, a)
		return st, st == "done"
	})
	if _, err := increments(t, ctx).Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatalf("the increment once A's GUI test passed and A is done: %v", err)
	}
}

// « À FAIRE » OMITS A BUILT FEATURE AWAITING ITS GUI TEST:
// a built screen feature's plan stands blocked by its refused end (gui-tested
// is unmet): the person is not asked to unblock it - its GUI test runs in the
// batch. The same feature not built (its unit criterion not passed) is asked.
func TestThePersonQueueOmitsABuiltFeatureAwaitingItsBatch(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "The invoice pane prints", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoicePrint.tsx"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	unit := criterionFor(t, ctx, cwm, "billing", f, "go test ./internal/extensions/billing/...")
	criterionFor(t, ctx, cwm, "billing", f, "bash app/ktw-gui/frontend/tests/live/invoice-print.sh")
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	refused := "its to-dos are done, but " + f + " cannot become done: cwm: transition refused: verify: ready-to-test: gui-tested"
	if _, err := plansOf(t, ctx).Block(ctx, api.BlockInput{ID: f, Note: refused, By: session}); err != nil {
		t.Fatal(err)
	}
	setBlockKind(t, ctx, cwm, f, api.BlockMoveRefused)
	asked := func() bool {
		for _, it := range personQueue(t, ctx, "").Items {
			if it.Action == api.ActionUnblock && it.Request == f {
				return true
			}
		}
		return false
	}
	if !asked() {
		t.Fatalf("%s is not built (its unit criterion has not passed): « À faire » should ask to unblock it", f)
	}
	passVerdict(t, ctx, cwm, "billing", unit)
	if asked() {
		t.Fatalf("« À faire » asks the person to unblock %s, built and waiting only for its GUI test: %+v", f, personQueue(t, ctx, "").Items)
	}
}

// THE GUI GATE LISTS A BUILT FEATURE'S TEST IN ITS RELEASE'S BATCH
// : A and B, built screen features of an increment
// shipping in a release, are refused done by gui-tested. A's refusal files the
// release's GUI batch, listing A's live test; B's adds its test to that same
// batch - one batch per release, no request of their own asking their test.
func TestTheGUIGateListsTheTestInItsReleaseBatch(t *testing.T) {
	// Not in parallel: its observer's moves race the other worlds' on a loaded machine.
	ctx, cwm, req := world(t)
	rel := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers")
	if _, err := increments(t, ctx).ShipIn(ctx, api.ShipInInput{ID: in.ID, In: rel.ID}); err != nil {
		t.Fatal(err)
	}
	built := func(title, screen, script string) string {
		t.Helper()
		f := want(t, ctx, cwm, "feature", title, cwmapi.ObjectEdge{Kind: "realizes", To: screen})
		if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
			t.Fatal(err)
		}
		passVerdict(t, ctx, cwm, "billing", criterionFor(t, ctx, cwm, "billing", f, "go test ./internal/extensions/billing/..."))
		criterionFor(t, ctx, cwm, "billing", f, "bash "+script)
		return f
	}
	a := built("The invoice pane shows its due date", "app/ktw-gui/frontend/src/sections/InvoiceDue.tsx", "app/ktw-gui/frontend/tests/live/invoice-due.sh")
	b := built("The invoice pane shows its payer", "app/ktw-gui/frontend/src/sections/InvoicePayer.tsx", "app/ktw-gui/frontend/tests/live/invoice-payer.sh")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{a: "pass", b: "pass"}})

	batches := func() (out []api.Request, own int) {
		t.Helper()
		l, err := req.List(ctx, api.ListInput{Module: "billing"})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range l.Requests {
			if strings.Contains(r.Title, "Write and run the GUI test") {
				own++
			}
			for _, d := range r.Deliverables {
				if strings.Contains(d.Check, "tests/live/") {
					out = append(out, r)
					break
				}
			}
		}
		return out, own
	}
	refuse := func(f string, scripts int) api.Request {
		t.Helper()
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "done", Transition: "verify", By: session, Note: "its criteria pass"}); err == nil {
			t.Fatalf("%s moved done before its GUI test passed", f)
		}
		var got []api.Request
		var own int
		for i := 0; i < 100; i++ {
			got, own = batches()
			if len(got) == 1 && len(got[0].Deliverables) == scripts {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if len(got) != 1 || len(got[0].Deliverables) != scripts || own != 0 {
			t.Fatalf("after %s's refusal: batches %+v, requests of their own %d; want one batch listing %d tests, none of their own", f, got, own, scripts)
		}
		return got[0]
	}
	first := refuse(a, 1)
	if !strings.Contains(first.Deliverables[0].Check, "invoice-due.sh") {
		t.Fatalf("the batch lists %+v; want A's live test", first.Deliverables)
	}
	second := refuse(b, 2)
	if second.ID != first.ID {
		t.Fatalf("B's test went to %s; want the release's one batch %s", second.ID, first.ID)
	}
	refuse(a, 2) // a second refusal of A lists nothing twice
}

// A FAILED BATCHED GUI TEST FILES FIXES FOR THE FEATURE AND ITS DEPENDENTS
// (RQ-340, RQ-394): A, built, waits only for its live
// test, listed in B, the release's GUI batch; D was built on A (realized), F
// is being built on it (active) and so is the request G (its implement
// reported); E and the request C depend on A but are not started. B's check
// runs no walk: with no verdict recorded it files
// nothing; once the batch script recorded A's test failed, a fix about A and a
// « Check again » about D, F and G are filed, none about E or C - once, a
// second check files nothing more.
func TestAFailedBatchedGUITestFilesFixesForTheFeatureAndItsDependents(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	script := "app/ktw-gui/frontend/tests/live/no-such-invoice-test.sh"
	a := want(t, ctx, cwm, "feature", "The invoice pane shows its currency", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceCurrency.tsx"})
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	passVerdict(t, ctx, cwm, "billing", criterionFor(t, ctx, cwm, "billing", a, "go test ./internal/extensions/billing/..."))
	walk := criterionFor(t, ctx, cwm, "billing", a, "bash "+script)
	if _, err := req.File(ctx, api.FileInput{On: a, Kind: api.KindReview, By: session}); err != nil {
		t.Fatal(err)
	}
	c := file(t, ctx, req, api.FileInput{Title: "Convert the invoice's currency", Module: "billing", Kind: api.KindFix})
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: a}); err != nil {
		t.Fatal(err)
	}
	dependsOnA := cwmapi.ObjectEdge{Kind: api.EdgeDependsOn, To: a}
	d := want(t, ctx, cwm, "feature", "The invoice total shows its currency", dependsOnA)
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: d, To: "realized", By: session, Note: "built on A"}); err != nil {
		t.Fatal(err)
	}
	want(t, ctx, cwm, "feature", "The invoice export shows its currency", dependsOnA)
	f := want(t, ctx, cwm, "feature", "The invoice reminder shows its currency", dependsOnA)
	accepted(t, ctx, cwm, f)
	for _, to := range []string{"specified", "ready"} {
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: to, By: person, Force: true, Note: "forced for the test"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := move(ctx, cwm, f, "active", intake); err != nil {
		t.Fatal(err)
	}
	g := file(t, ctx, req, api.FileInput{Title: "Round the invoice's currency", Module: "billing", Kind: api.KindFix})
	if _, err := req.Depend(ctx, api.DependInput{ID: g.ID, On: a}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, g.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	pullAndReport(t, ctx, stepOf(t, ctx, g.ID, "implement").ID, session, schedapi.Result{Summary: "rounded"})
	b := file(t, ctx, req, api.FileInput{Title: "Run the release's GUI tests together", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "The invoice currency by element ids", Check: "bash " + script}}})

	fixes := func() (aboutA int, checks map[string]int) {
		t.Helper()
		l, err := req.List(ctx, api.ListInput{Module: "billing"})
		if err != nil {
			t.Fatal(err)
		}
		checks = map[string]int{}
		for _, r := range l.Requests {
			switch {
			case strings.HasPrefix(r.Title, "Fix ") && strings.Contains(r.Title, "GUI test failed"):
				aboutA++
			case strings.HasPrefix(r.Title, "Check ") && strings.Contains(r.Title, a+"'s GUI test failed"):
				checks[strings.TrimSuffix(strings.TrimPrefix(r.Title, "Check "), " again: "+a+"'s GUI test failed")]++
			}
		}
		return
	}
	if _, err := req.Check(ctx, api.CheckInput{ID: b.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if fa, checks := fixes(); fa != 0 || len(checks) != 0 {
		t.Fatalf("no verdict recorded: fixes about A %d, checks again %v; want none - a check runs no walk", fa, checks)
	}
	verdictAt(t, ctx, cwm, walk, "fail", time.Now().UTC().Add(time.Minute).Format(time.RFC3339))
	for run := 1; run <= 2; run++ {
		out, err := req.Check(ctx, api.CheckInput{ID: b.ID, By: session})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Results) != 1 || out.Results[0].Passed {
			t.Fatalf("run %d: %+v; want the batch's one check failed", run, out.Results)
		}
		fa, checks := fixes()
		if fa != 1 || len(checks) != 3 || checks["The invoice total shows its currency"] != 1 ||
			checks["The invoice reminder shows its currency"] != 1 || checks["Round the invoice's currency"] != 1 {
			t.Fatalf("run %d: fixes about A %d, checks again %v; want one fix about A and one check each of D, F and %s only - E and %s are not started (%s)", run, fa, checks, g.ID, c.ID, out.Results[0].Error)
		}
	}
}
