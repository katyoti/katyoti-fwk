// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
)

// A TO-DO'S LABELS FOLLOW A MOVED INCREMENT OR RELEASE — an increment renamed
// by a CWM move labels its open work with its new id, and its release changed
// past the increments service (its ships-in edge edited by hand) labels it
// with the new release.
func TestATodosLabelsFollowAMovedIncrementOrRelease(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	rel := releases(t, ctx)
	inc := increments(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	b := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
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
	labels := func() []string { return stepOf(t, ctx, f, api.StepIntake).Labels }
	if l := labels(); !slices.Contains(l, "increment:"+in.ID) {
		t.Fatalf("before the move: labels %v; want increment:%s", l, in.ID)
	}

	// Renamed by a CWM move: the open work carries the new id.
	moved, err := cwm.ObjectMove(ctx, cwmapi.ObjectMoveInput{ID: in.ID, NewLocalID: "IN-900", Reason: "renumbered"})
	if err != nil {
		t.Fatal(err)
	}
	if l, ok := labelsMatch(ctx, labels, func(l []string) bool {
		return slices.Contains(l, "increment:"+moved.ID) && !slices.Contains(l, "increment:"+in.ID) && slices.Contains(l, "release:"+a.ID)
	}); !ok {
		t.Errorf("after the increment moved to %s: labels %v; want increment:%s and release:%s, never the old id", moved.ID, l, moved.ID, a.ID)
	}

	// Its release changed by hand: the work follows it.
	if _, err := cwm.EdgeDelete(ctx, cwmapi.EdgeDeleteInput{From: moved.ID, To: a.ID, Kind: cwmapi.EdgeType(api.EdgeShipsIn)}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: moved.ID, To: b.ID, Kind: cwmapi.EdgeType(api.EdgeShipsIn)}); err != nil {
		t.Fatal(err)
	}
	if l, ok := labelsMatch(ctx, labels, func(l []string) bool {
		return slices.Contains(l, "release:"+b.ID) && !slices.Contains(l, "release:"+a.ID)
	}); !ok {
		t.Errorf("after its ships-in was edited by hand: labels %v; want release:%s only", l, b.ID)
	}
}

// labelsMatch re-reads the labels until want holds or eventuallyWithin passes: the relabel
// runs on the observer's goroutine, which a loaded machine can delay well past
// one settle. It answers the last labels seen and whether they matched.
func labelsMatch(ctx context.Context, labels func() []string, want func([]string) bool) ([]string, bool) {
	deadline := time.Now().Add(eventuallyWithin)
	for {
		planlocal.Settle(ctx, time.Second)
		l := labels()
		if want(l) {
			return l, true
		}
		if time.Now().After(deadline) {
			return l, false
		}
		time.Sleep(50 * time.Millisecond)
	}
}
