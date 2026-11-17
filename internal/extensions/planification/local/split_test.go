// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// SPLITTING: intake replaces a request by its parts — one per module it
// affects, one per unmet goal — each keeping the requester and what the
// request waited on; the request is dropped, naming them.

// A request affecting two modules becomes one request per module, each
// planned on its own; the request stays, split, naming its parts.
func TestARequestSplitsOnePerModule(t *testing.T) {
	t.Parallel()
	ctx, _, req, _, _ := planWorld(t)
	blocker := file(t, ctx, req, api.FileInput{Title: "Mail server", Module: "billing"})
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild,
		Priority: "P1", Details: "every invoice, every month", DependsOn: []string{blocker.ID}, By: person})
	got, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Note: "two modules", Parts: []api.SplitPart{
		{Module: "billing", Title: "Invoices carry the mail address"},
		{Module: "shipping", Title: "Mail the invoice with the parcel", Priority: "P0"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 2 || got.Parts[0].Module != "billing" || got.Parts[1].Module != "shipping" {
		t.Fatalf("parts = %+v, want one in billing and one in shipping", got.Parts)
	}
	for _, p := range got.Parts {
		if p.State != api.PlanReady || p.SplitFrom != r.ID || p.Requester != person || p.Kind != api.KindBuild || p.Details != "every invoice, every month" {
			t.Errorf("part %s = %+v; want its intake planned, split from %s, asked by the requester, its kind and details", p.ID, p.RequestFields, r.ID)
		}
		if len(p.DependsOn) != 1 || p.DependsOn[0] != blocker.ID {
			t.Errorf("part %s waits on %v, want what the request waited on (%s)", p.ID, p.DependsOn, blocker.ID)
		}
	}
	if got.Parts[0].Priority != "P1" || got.Parts[1].Priority != "P0" {
		t.Errorf("priorities = %s, %s; want the request's unless the part sets one", got.Parts[0].Priority, got.Parts[1].Priority)
	}
	if s := got.Split; s.State != api.PlanSplit || s.Plan == nil || len(s.Plan.Trail) == 0 ||
		!strings.Contains(s.Plan.Trail[len(s.Plan.Trail)-1].Note, got.Parts[0].ID) || !strings.Contains(s.Plan.Trail[len(s.Plan.Trail)-1].Note, got.Parts[1].ID) {
		t.Errorf("split = %s, plan %+v; want split naming both parts", s.State, s.Plan)
	}
}

// A release-sized request becomes one request per unmet goal, filed on it.
func TestARequestSplitsOnePerUnmetGoal(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, _, _ := planWorld(t)
	g1 := usecase(t, ctx, cwm, "Invoices are sent")
	g2 := usecase(t, ctx, cwm, "Invoices are paid")
	r := file(t, ctx, req, api.FileInput{Title: "Billing 2.0", Module: "billing"})
	got, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Parts: []api.SplitPart{{On: g1}, {On: g2}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 2 || got.Parts[0].ID != g1 || got.Parts[1].ID != g2 || got.Parts[0].Title != "Invoices are sent" {
		t.Fatalf("parts = %+v, want the two goals carrying the work", got.Parts)
	}
	if got.Parts[1].SplitFrom != r.ID || got.Parts[1].State != api.PlanReady {
		t.Errorf("part on %s = %+v", g2, got.Parts[1].RequestFields)
	}
}

// A split names, for each part, a module or an object — and never splits what
// has ended.
func TestASplitIsRefusedWhenItsPartsAreUnclear(t *testing.T) {
	t.Parallel()
	ctx, _, req, _, _ := planWorld(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing"})
	for name, parts := range map[string][]api.SplitPart{
		"no part":      nil,
		"both":         {{Module: "billing", On: "billing"}},
		"neither":      {{Title: "x"}},
		"bad priority": {{Module: "billing", Priority: "P7"}},
	} {
		if _, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Parts: parts}); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v, want invalid input", name, err)
		}
	}
	if st := state(t, ctx, req, r.ID); st != api.PlanReady {
		t.Errorf("a refused split left the request %s, want ready", st)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Parts: []api.SplitPart{{Module: "billing"}}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("splitting a dropped request: %v, want invalid input", err)
	}
}

// Any demand splits, not only a request: a feature with no request becomes
// sub-features under it — each a feature of its own, its intake planned,
// split from it — or requests in modules; it stays, split, and ends when its
// parts have.
func TestAFeatureSplitsIntoSubFeatures(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	f := feature(t, w, "Invoices by mail")
	got, err := w.req.Split(w.ctx, api.SplitInput{ID: f, By: person, Parts: []api.SplitPart{
		{Sub: true, Title: "The mail address on the invoice"},
		{Sub: true, Title: "Mail the invoice"},
		{Module: "shipping", Title: "A mail slot in the parcel"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Parts) != 3 {
		t.Fatalf("parts = %+v, want three", got.Parts)
	}
	for _, p := range got.Parts[:2] {
		o, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "billing", ID: p.ID})
		if err != nil || o.Object.Type != "feature" || o.Object.Parent != f || p.SplitFrom != f || p.State != api.PlanReady || p.Requester != person {
			t.Errorf("sub %s = type %s parent %s, split from %s, %s, requester %+v (%v); want a feature under %s, split from it, its intake planned",
				p.ID, o.Object.Type, o.Object.Parent, p.SplitFrom, p.State, p.Requester, err, f)
		}
	}
	if got.Parts[2].Module != "shipping" || got.Parts[2].SplitFrom != f {
		t.Errorf("the module part = %+v, want a request in shipping split from the feature", got.Parts[2])
	}
	if p := w.plan(f); p.State != api.PlanSplit {
		t.Fatalf("the feature's plan = %s, want split", p.State)
	}

	for _, p := range got.Parts {
		if _, err := w.plans.Finish(w.ctx, api.FinishInput{ID: p.ID, By: person}); err != nil {
			t.Fatal(err)
		}
	}
	if p := w.plan(f); p.State != api.PlanDone {
		t.Fatalf("the feature after its parts = %s, want done", p.State)
	}
}

// A part names one form: a module, an object to file on, or sub — and a sub
// part its title.
func TestASplitPartNamesOneForm(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	f := feature(t, w, "Invoices by mail")
	for name, part := range map[string]api.SplitPart{
		"two forms": {Sub: true, Module: "shipping", Title: "x"},
		"none":      {Title: "x"},
		"untitled":  {Sub: true},
	} {
		if _, err := w.req.Split(w.ctx, api.SplitInput{ID: f, By: person, Parts: []api.SplitPart{part}}); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
}

func feature(t *testing.T, w *plannerWorld, title string) string {
	t.Helper()
	o := cwmapi.Object{Type: "feature"}
	o.Parent = "billing"
	o.Name = title
	out, err := w.cwm.ObjectCreate(w.ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatal(err)
	}
	return out.Object.ID
}

// A split into one part is a move: the part keeps the request's deliverables,
// a verified one still verified; the parts of a wider split carry none of
// them — each names its own.
func TestAOnePartSplitKeepsTheDeliverables(t *testing.T) {
	t.Parallel()
	ctx, _, req, _, _ := planWorld(t)
	asked := []api.Deliverable{{Title: "the shape is drawn", Check: "go test ./drawio/..."}, {Title: "seen in the editor"}}
	r := file(t, ctx, req, api.FileInput{Title: "Draw the shape", Module: "billing", Deliverables: asked, By: person})
	if _, err := req.Verify(ctx, api.VerifyInput{ID: r.ID, Deliverable: "seen in the editor", Evidence: "opened it", By: person}); err != nil {
		t.Fatal(err)
	}
	moved, err := req.Split(ctx, api.SplitInput{ID: r.ID, By: intake, Note: "its module", Parts: []api.SplitPart{{Module: "shipping"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(moved.Parts) != 1 {
		t.Fatalf("parts = %+v, want one", moved.Parts)
	}
	got := moved.Parts[0].Deliverables
	if len(got) != 2 || got[0].Title != asked[0].Title || got[0].Check != asked[0].Check || got[0].Verified ||
		got[1].Title != asked[1].Title || !got[1].Verified || got[1].VerifiedBy == nil || got[1].Evidence != "opened it" {
		t.Fatalf("moved part's deliverables = %+v; want both of the request's, the second still verified", got)
	}
	if d := deliverableOf(t, ctx, req, moved.Parts[0].ID, "the shape is drawn"); d.Title == "" {
		t.Fatalf("the part as read back has no deliverable %q", asked[0].Title)
	}

	wide := file(t, ctx, req, api.FileInput{Title: "Draw two shapes", Module: "billing", Deliverables: asked[:1], By: person})
	split, err := req.Split(ctx, api.SplitInput{ID: wide.ID, By: intake, Parts: []api.SplitPart{{Module: "billing"}, {Module: "shipping"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range split.Parts {
		if len(p.Deliverables) != 0 {
			t.Errorf("part %s of a two-part split carries %+v; want none (each part names its own)", p.ID, p.Deliverables)
		}
	}
}
