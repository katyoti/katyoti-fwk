// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// NOTHING AN INCREMENT'S OPEN WORK WAITS ON IS UNDELIVERABLE
// : usable-delivered refuses an
// increment whose open work - a request serving one of its features, still
// open once the feature is done - depends on a dropped request or on one
// held by an increment of a later release, naming each; repointed to a done
// request, the increment is accepted.
func TestUsableDeliveredRefusesWorkWaitingOnTheUndeliverable(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, _, in := reviewWorld(t)
	later := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.5.0"})
	elsewhere := incrementIn(t, ctx, "Reminders", later.ID)
	inLater := file(t, ctx, req, api.FileInput{Title: "Mail the reminders", Module: "billing", Kind: api.KindFix, Increment: elsewhere.ID})
	gone := file(t, ctx, req, api.FileInput{Title: "Print the invoices on paper", Module: "billing", Kind: api.KindFix})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: gone.ID, Outcome: api.PlanDropped, By: person, Note: "no paper"}); err != nil {
		t.Fatal(err)
	}
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	aim(t, ctx, f, in.ID)
	open := file(t, ctx, req, api.FileInput{Title: "Mail the invoices in batches", Module: "billing", Kind: api.KindFix})
	edge := func(from, to, kind string, add bool) {
		t.Helper()
		var err error
		if add {
			_, err = cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: from, To: to, Kind: cwmapi.EdgeType(kind)})
		} else {
			_, err = cwm.EdgeDelete(ctx, cwmapi.EdgeDeleteInput{From: from, To: to, Kind: cwmapi.EdgeType(kind)})
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	edge(open.ID, f, api.EdgeServes, true)
	edge(open.ID, gone.ID, api.EdgeDependsOn, true)
	edge(open.ID, inLater.ID, api.EdgeDependsOn, true)
	confirmDone(t, ctx, cwm, in.ID, f)

	names := []string{open.ID + " waits on " + gone.ID + ", which is dropped", open.ID + " waits on " + inLater.ID + ", which ships in " + later.ID}
	s := schedOf(t, ctx)
	id := stepOf(t, ctx, in.ID, "delivery-review").ID
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the delivery review %s: %v %+v", id, err, got)
	}
	_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "usable",
		Verdict: api.VerdictApproved, By: session, More: map[string]any{"review": map[string]any{"demo": demoPath}}}})
	for _, n := range names {
		if err == nil || !strings.Contains(err.Error(), n) {
			t.Errorf("a review approving %s whose work waits on the undeliverable: %v; want it refused naming %q", in.ID, err, n)
		}
	}
	inc := increments(t, ctx)
	got, err := inc.CheckUsableDelivered(ctx, api.UsableDeliveredInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if got.Accept || !strings.Contains(got.Reason, n) {
			t.Errorf("usable-delivered: %+v; want it refused naming %q", got, n)
		}
	}

	if _, err := s.Release(ctx, schedapi.ReleaseInput{ID: id, By: session, Note: "its work waits on the undeliverable"}); err != nil {
		t.Fatal(err)
	}
	done := file(t, ctx, req, api.FileInput{Title: "Address the invoices", Module: "billing", Kind: api.KindFix})
	workFix(t, ctx, done.ID)
	edge(open.ID, gone.ID, api.EdgeDependsOn, false)
	edge(open.ID, inLater.ID, api.EdgeDependsOn, false)
	edge(open.ID, done.ID, api.EdgeDependsOn, true)
	if out := reviewDelivery(t, ctx, in.ID, nil); !strings.HasPrefix(out.Note, "collected") {
		t.Fatalf("the review once its work waits on a done request: %q; want it collected", out.Note)
	}
	if got, err := inc.CheckUsableDelivered(ctx, api.UsableDeliveredInput{ID: in.ID}); err != nil || !got.Accept {
		t.Fatalf("usable-delivered once its work waits on a done request: %+v %v; want it accepted", got, err)
	}
}

// A DONE FEATURE IS NOT DROPPED BY ITS DROPPED ROUND:
// a feature whose lifecycle is done but whose last round of work - the
// request it carries - was dropped is done for the delivery gate: work
// depending on it is not refused as « waiting on … which is dropped ».
func TestUsableDeliveredReadsADoneFeatureWithADroppedRoundAsDone(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, _, in := reviewWorld(t)
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	g := want(t, ctx, cwm, "feature", "Invoices are addressed")
	aim(t, ctx, f, in.ID)
	aim(t, ctx, g, in.ID)
	open := file(t, ctx, req, api.FileInput{Title: "Mail the invoices in batches", Module: "billing", Kind: api.KindFix})
	for _, e := range []cwmapi.EdgeCreateInput{{From: open.ID, To: f, Kind: cwmapi.EdgeType(api.EdgeServes)}, {From: open.ID, To: g, Kind: cwmapi.EdgeType(api.EdgeDependsOn)}} {
		if _, err := cwm.EdgeCreate(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	// g's verify round - the request g itself carries - is dropped as
	// having nothing left; g is confirmed done all the same.
	if _, err := req.File(ctx, api.FileInput{On: g, Kind: api.KindReview, Title: "Verify the realized feature", By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: g, Outcome: api.PlanDropped, By: person, Note: "nothing left"}); err != nil {
		t.Fatal(err)
	}
	confirmDone(t, ctx, cwm, in.ID, f, g)

	got, err := increments(t, ctx).CheckUsableDelivered(ctx, api.UsableDeliveredInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if n := open.ID + " waits on " + g + ", which is dropped"; strings.Contains(got.Reason, n) {
		t.Errorf("usable-delivered: %+v; want no %q - %s is done, only its round was dropped", got, n, g)
	}
}
