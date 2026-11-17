// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE REVERSED PLANNING EDGE (rule):
// an increment or a release lists the module work it takes; the module
// object carries no edge to it.

func edgesOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id, kind string) []string {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range got.Object.Edges {
		if e.Kind == kind {
			out = append(out, e.To)
		}
	}
	return out
}

func scopeIDs(t *testing.T, ctx context.Context, inc api.IncrementService, id string) []string {
	t.Helper()
	sc, err := inc.Scope(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, tg := range sc.Targets {
		out = append(out, tg.ID)
	}
	return out
}

// (1) An increment's work is read from its takes - written by its verb or as
// an applied proposal leaves it -, the requests targeting it beside.
func TestAnIncrementsWorkIsReadFromTakes(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	old := want(t, ctx, cwm, "feature", "Invoices are printed")
	taken(t, ctx, cwm, in.ID, old)
	rq := file(t, ctx, req, api.FileInput{Title: "Mail the invoices", Module: "billing", Kind: api.KindBuild, Increment: in.ID})

	got := scopeIDs(t, ctx, inc, in.ID)
	for _, id := range []string{f, old, rq.ID} {
		if !slices.Contains(got, id) {
			t.Errorf("the increment's scope %v misses %s", got, id)
		}
	}
	if len(got) != 3 {
		t.Errorf("the scope %v; want each once", got)
	}
	// The labels of the feature's work follow the increment it is taken by.
	b := file(t, ctx, req, api.FileInput{On: f, Kind: api.KindBuild, By: person})
	todos, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range todos.Todos {
		if !slices.Contains(td.Labels, "increment:"+in.ID) {
			t.Errorf("the build's %s carries %v; want the label of %s, which takes its feature", td.Packet.Step, td.Labels, in.ID)
		}
	}
}

// (2) Aiming a goal or a feature writes takes on the increment, never targets
// on the feature: moved, the old increment gives it up; taken out, none
// holds it. A use case aimed at a release is taken by the release.
func TestAimingWorkWritesTakesOnTheIncrement(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc, rel := increments(t, ctx), releases(t, ctx)
	a := increment(t, ctx, "Invoices reach their customers by mail")
	b := increment(t, ctx, "Invoices reach their customers by post")
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: a.ID}); err != nil {
		t.Fatal(err)
	}
	if got := edgesOf(t, ctx, cwm, f, api.EdgeTargets); len(got) != 0 {
		t.Errorf("the feature carries targets %v; want none (rule R-007)", got)
	}
	if got := edgesOf(t, ctx, cwm, a.ID, api.EdgeTakes); !slices.Equal(got, []string{f}) {
		t.Errorf("%s takes %v; want [%s]", a.ID, got, f)
	}
	out, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: b.ID})
	if err != nil {
		t.Fatal(err)
	}
	if out.Replaced != a.ID || out.Release != b.ID {
		t.Errorf("moved: %+v; want it replacing %s by %s", out, a.ID, b.ID)
	}
	if got := edgesOf(t, ctx, cwm, a.ID, api.EdgeTakes); len(got) != 0 {
		t.Errorf("%s still takes %v after the move", a.ID, got)
	}
	if got := edgesOf(t, ctx, cwm, b.ID, api.EdgeTakes); !slices.Equal(got, []string{f}) {
		t.Errorf("%s takes %v; want [%s]", b.ID, got, f)
	}
	if _, err := inc.Untarget(ctx, api.UntargetInput{ID: f}); err != nil {
		t.Fatal(err)
	}
	if got := edgesOf(t, ctx, cwm, b.ID, api.EdgeTakes); len(got) != 0 {
		t.Errorf("%s still takes %v after untarget", b.ID, got)
	}

	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := rel.Target(ctx, api.TargetInput{ID: uc, Release: r.ID}); err != nil {
		t.Fatal(err)
	}
	if got := edgesOf(t, ctx, cwm, r.ID, api.EdgeTakes); !slices.Equal(got, []string{uc}) {
		t.Errorf("the release takes %v; want [%s]", got, uc)
	}
	sc, err := rel.Scope(ctx, api.GetInput{ID: r.ID})
	if err != nil || len(sc.Targets) != 1 || sc.Targets[0].ID != uc {
		t.Errorf("the release's scope %+v (%v); want the use case it takes", sc.Targets, err)
	}
}

// (3) At most one open taker: moving work leaves an ended increment (done or
// dropped) its takes - its scope stays readable -; two open takers
// written behind the service are reported by planning check.
func TestAnObjectHasOneOpenTakerAtMost(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	a := increment(t, ctx, "Invoices reach their customers by mail")
	b := increment(t, ctx, "Invoices reach their customers by post")
	f := want(t, ctx, cwm, "feature", "Invoices are mailed")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a.ID, To: api.IncrementDropped, By: person, Force: true, Note: "ended"}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: b.ID}); err != nil {
		t.Fatal(err)
	}
	if got := edgesOf(t, ctx, cwm, a.ID, api.EdgeTakes); !slices.Equal(got, []string{f}) {
		t.Errorf("the ended %s takes %v; want it to keep [%s] as its record", a.ID, got, f)
	}
	if got := scopeIDs(t, ctx, inc, a.ID); !slices.Equal(got, []string{f}) {
		t.Errorf("the ended increment's scope %v; want [%s]", got, f)
	}
	if got := edgesOf(t, ctx, cwm, b.ID, api.EdgeTakes); !slices.Equal(got, []string{f}) {
		t.Errorf("%s takes %v; want [%s]", b.ID, got, f)
	}

	// Behind the service: a third open increment takes it too.
	c := increment(t, ctx, "Invoices reach their customers by fax")
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: c.ID, To: f, Kind: api.EdgeTakes}); err != nil {
		t.Fatal(err)
	}
	pl, err := application.UseService[api.PlanningService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	chk, err := pl.Check(ctx, api.PlanningCheckInput{Increment: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, bl := range chk.Blockers {
		if bl.Work == f && bl.Kind == api.BlockerIncident && strings.Contains(bl.Reason, b.ID) && strings.Contains(bl.Reason, c.ID) {
			found = true
		}
	}
	if !found {
		t.Errorf("planning check %+v; want an incident on %s, taken by two open increments", chk.Blockers, f)
	}
}

// taken writes an increment's or a release's takes edge to a piece of module
// work behind its verb, as an applied proposal leaves it.
func taken(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, holder, id string) {
	t.Helper()
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: holder, To: id, Kind: api.EdgeTakes}); err != nil {
		t.Fatal(err)
	}
}
