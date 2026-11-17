// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WORK JOINS A RUNNING INCREMENT — a correction filed straight into it
// (requests file --increment) is aimed at it as direct work: labelled with
// it, in its scope, counted in its progress - done once its plan is; work
// filed on a feature joins it through that feature; an increment that is no
// longer open takes nothing.
func TestWorkJoinsARunningIncrement(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	fix, err := req.File(ctx, api.FileInput{Title: "The mailed invoice lacks its due date", Module: "billing", Kind: api.KindFix, Increment: in.ID, By: person})
	if err != nil {
		t.Fatal(err)
	}
	f := want(t, ctx, cwm, "feature", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: f, Kind: api.KindBuild, Increment: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	scope, err := inc.Scope(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, x := range scope.Targets {
		ids = append(ids, x.ID)
	}
	if !slices.Contains(ids, fix.Request.ID) || !slices.Contains(ids, f) || scope.Total != 2 || scope.Done != 0 {
		t.Fatalf("the increment's scope: %v (%d done of %d); want the fix and the feature, none done", ids, scope.Done, scope.Total)
	}
	// Only the request of its own is recorded joined: the feature carries no
	// edge to the increment (rule) - its takes records it.
	if got := edgesOf(t, ctx, cwm, fix.Request.ID, api.EdgeJoined); !slices.Contains(got, in.ID) {
		t.Errorf("the fix's joined edges %v; want %s", got, in.ID)
	}
	if got := edgesOf(t, ctx, cwm, f, api.EdgeJoined); len(got) != 0 {
		t.Errorf("the feature joined to the increment carries joined edges %v; want none (R-007)", got)
	}
	if td := stepOf(t, ctx, fix.Request.ID, api.StepIntake); !slices.Contains(td.Labels, "increment:"+in.ID) {
		t.Errorf("the fix's to-do labels %v; want increment:%s (k-loop --increment takes it)", td.Labels, in.ID)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: fix.Request.ID, Outcome: api.PlanDone, By: person, Note: "fixed"}); err != nil {
		t.Fatal(err)
	}
	if scope, _ := inc.Scope(ctx, api.GetInput{ID: in.ID}); scope.Done != 1 {
		t.Errorf("the fix done, the increment's progress: %d done of %d; want it counted", scope.Done, scope.Total)
	}
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: in.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{Title: "Too late", Module: "billing", Kind: api.KindFix, Increment: in.ID, By: person}); err == nil {
		t.Error("work joined a dropped increment")
	}
}
