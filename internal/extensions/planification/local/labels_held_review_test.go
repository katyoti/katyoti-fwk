// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A REVIEW HOLDING AN INCREMENT'S WORK CARRIES ITS LABELS
// : a feature-changed review filed on a feature no
// increment takes, holding the work on a feature an increment holds, is that
// increment's work - its to-dos carry the increment's, release's and stream's
// labels, so a pull by them hands it out and the increment does not stall.
func TestAReviewHoldingAnIncrementsWorkCarriesItsLabels(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	// A feature of no increment, the increment's feature derived from it.
	g := want(t, ctx, cwm, "feature", "Invoices are numbered")
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: f, To: g, Kind: cwmapi.EdgeTypeDerivedFrom}); err != nil {
		t.Fatal(err)
	}
	raised, err := triggersOf(t, ctx).Raise(ctx, api.RaiseInput{Source: api.RequestSource{Kind: api.SourceTrigger, Name: "feature-changed", Object: g, Change: "its body changed"},
		Kind: api.KindReview, Title: "Review the work on the changed feature", Holds: true})
	if err != nil {
		t.Fatal(err)
	}
	if raised.Held == 0 {
		t.Fatalf("raised %+v: want the increment's work held by the review", raised)
	}
	l := stepOf(t, ctx, raised.Request, api.StepIntake).Labels
	for _, want := range []string{"increment:" + in.ID, "release:" + a.ID} {
		if !slices.Contains(l, want) {
			t.Errorf("the review's intake: labels %v; want %s", l, want)
		}
	}
}
