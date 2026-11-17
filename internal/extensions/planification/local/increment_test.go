// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// INCREMENTS — one sentence of user value holding the goals and features that
// make it; it ships in at most one release, which then ships what it holds;
// one level only; done once each thing it holds is.

func increments(t *testing.T, ctx context.Context) api.IncrementService {
	t.Helper()
	svc, err := application.UseService[api.IncrementService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func increment(t *testing.T, ctx context.Context, title string) api.Increment {
	t.Helper()
	out, err := increments(t, ctx).Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title})
	if err != nil {
		t.Fatal(err)
	}
	return out.Increment
}

func TestAnIncrementShipsWhatItHoldsInARelease(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	rel := releases(t, ctx)
	a := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	b := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.5.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	if in.State != api.IncrementOpen || !strings.HasPrefix(in.ID, "billing.IN-") {
		t.Fatalf("created: %+v; want an open increment", in)
	}
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: a.ID}); err != nil {
		t.Fatal(err)
	}
	s, err := rel.Scope(ctx, api.GetInput{ID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 1 || s.Targets[0].ID != uc || s.Targets[0].Via != in.ID {
		t.Fatalf("the release's scope: %+v; want %s through %s", s.Targets, uc, in.ID)
	}
	// Not ready: it moves to the next release.
	moved, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: b.ID})
	if err != nil || moved.Increment.ShipsIn != b.ID {
		t.Fatalf("moved: %+v, %v; want it in %s", moved.Increment, err, b.ID)
	}
	if s, _ := rel.Scope(ctx, api.GetInput{ID: a.ID}); s.Total != 0 {
		t.Errorf("%s still ships %d target(s) after the increment moved", a.ID, s.Total)
	}
	if got, _ := inc.Scope(ctx, api.GetInput{ID: in.ID}); got.Total != 1 {
		t.Errorf("the increment's scope: %+v; want its use case", got)
	}
}

func TestAnIncrementIsOneLevel(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	inc := increments(t, ctx)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	one, two := increment(t, ctx, "One"), increment(t, ctx, "Two")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: two.ID, Increment: one.ID}); err == nil {
		t.Error("an increment was aimed at an increment")
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: one.ID, In: two.ID}); err == nil {
		t.Error("an increment shipped in an increment")
	}
	if _, err := rel.Target(ctx, api.TargetInput{ID: one.ID, Release: r.ID}); err == nil {
		t.Error("an increment targeted a release; it ships in one")
	}
	q := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: q.ID, Increment: one.ID}); err == nil {
		t.Error("a request was held by an increment; it serves a feature the increment holds")
	}
	if _, err := rel.ShipIn(ctx, api.ShipInInput{ID: one.ID, In: r.ID}); err == nil {
		t.Error("the release service shipped an increment")
	}
	_ = cwm
}

func TestAnIncrementIsDoneOnceWhatItHoldsIs(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	rel := releases(t, ctx)
	r := plan(t, ctx, rel, api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := increment(t, ctx, "Invoices reach their customers by mail")
	uc := usecase(t, ctx, cwm, "Send invoices by mail")
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: uc, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.ShipIn(ctx, api.ShipInInput{ID: in.ID, In: r.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), uc) {
		t.Fatalf("done with %s not done: %v; want refused naming it", uc, err)
	}
	if _, err := rel.Freeze(ctx, api.MoveInput{ID: r.ID, By: intake}); err != nil {
		t.Fatal(err)
	}
	changelogListed(t, ctx, cwm, r.ID)
	finish(t, ctx, cwm, uc)
	if _, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), in.ID) {
		t.Fatalf("released with its increment open: %v; want refused naming %s", err, in.ID)
	}
	if got, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err != nil || got.Increment.State != api.IncrementDone {
		t.Fatalf("done once what it holds is: %+v, %v", got.Increment, err)
	}
	if out, err := rel.Release(ctx, api.MoveInput{ID: r.ID, By: person}); err != nil || out.Release.State != api.ReleaseReleased {
		t.Fatalf("released with its increment done: %+v, %v", out.Release, err)
	}
}

// An increment's pane shows its title and the bar of its requests with their
// counts; it opens on its Open requests panel.
func TestAnIncrementsPaneShowsWhatItHoldsAndItsProgress(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	for _, want := range []string{"Invoices reach their customers by mail", `"data-view":"increment-page-counts"`, `"kind":"gauge"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the pane lacks %s", want)
		}
	}
	pane, err := application.UseService[objectpaneapi.ObjectPaneService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	d, err := pane.Detail(ctx, objectpaneapi.DetailInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(d.Panels, func(p objectpaneapi.ObjectPanel) bool { return p.ID == "tab:increment-open-requests" }) {
		t.Errorf("the Open requests panel is not on its pane: %+v", d.Panels)
	}
}

// An increment carries a short title - the name lists, boards and its pane
// show - and a description giving its intent; both are given at creation and
// changed by Rename, an empty one kept; one made before has an empty
// description.
func TestIncrementTitleAndDescription(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	inc := increments(t, ctx)
	made, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices by mail",
		Description: "Each invoice reaches its customer by mail the day it is issued, so no one sends them by hand."})
	if err != nil {
		t.Fatal(err)
	}
	in := made.Increment
	if in.Title != "Invoices by mail" || !strings.HasPrefix(in.Description, "Each invoice reaches") {
		t.Fatalf("created: %+v; want its short title and its intent", in)
	}
	list, err := inc.List(ctx, api.ListIncrementsInput{Module: "billing"})
	if err != nil || len(list.Increments) != 1 || list.Increments[0].Title != "Invoices by mail" {
		t.Fatalf("listed: %+v, %v; want it by its short title", list.Increments, err)
	}
	page, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "page"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if !strings.Contains(string(raw), "Invoices by mail") || !strings.Contains(string(raw), "the day it is issued") {
		t.Errorf("its pane lacks its title or its intent: %s", raw)
	}
	if _, err := inc.Rename(ctx, api.RenameIncrementInput{ID: in.ID, By: person}); err == nil {
		t.Error("a rename with neither a title nor a description was accepted")
	}
	got, err := inc.Rename(ctx, api.RenameIncrementInput{ID: in.ID, Title: "Mailed invoices", By: person})
	if err != nil || got.Increment.Title != "Mailed invoices" || got.Increment.Description != in.Description {
		t.Fatalf("renamed: %+v, %v; want the new title and the description kept", got.Increment, err)
	}
	got, err = inc.Rename(ctx, api.RenameIncrementInput{ID: in.ID, Description: "Invoices leave by themselves.", By: person})
	if err != nil || got.Increment.Title != "Mailed invoices" || got.Increment.Description != "Invoices leave by themselves." {
		t.Fatalf("a new description: %+v, %v; want the title kept", got.Increment, err)
	}
	old, _ := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Reminders"})
	if old.Increment.Description != "" {
		t.Errorf("one made with a title only: description %q; want none", old.Increment.Description)
	}
}

// A request ended dropped (superseded, or given up by decision) holds nothing
// back: the increment it targeted is done once its other work is, while one
// still open refuses it.
func TestADroppedRequestDoesNotHoldItsIncrementBack(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	inc, plans := increments(t, ctx), plansOf(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	moved := file(t, ctx, req, api.FileInput{Title: "Faster mail proofs", Module: "billing", Kind: api.KindFix, Increment: in.ID})
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); !errors.Is(err, cwmapi.ErrRefusedByHook) || !strings.Contains(err.Error(), moved.ID) {
		t.Fatalf("done with %s open: %v; want refused naming it", moved.ID, err)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: moved.ID, Outcome: api.PlanDropped, By: person, Note: "moved to the polish increment"}); err != nil {
		t.Fatal(err)
	}
	if got, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err != nil || got.Increment.State != api.IncrementDone {
		t.Fatalf("done with its only request dropped: %+v, %v; want done", got.Increment, err)
	}
}
