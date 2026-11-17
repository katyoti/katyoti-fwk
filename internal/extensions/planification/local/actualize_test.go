// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// ACTUALIZING AN OPEN REQUEST — rescoped, superseded or merged, saying why. A
// step not started follows; one a worker holds is marked stale and its report
// recorded saying so; what depended on a superseded request depends on its
// successor; its requester is told.

func TestActualizedARescopedRequestKeepsItsOldWordingAndMarksItsRunningStepStale(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Details: "By mail.", By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "assessed"})
	analyse := stepOf(t, ctx, r.ID, "analyse")
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: analyse.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Rescope(ctx, api.RescopeInput{ID: r.ID, Title: "Send invoices by e-mail", By: session}); err == nil {
		t.Error("a rescope without why: accepted")
	}
	got, err := req.Rescope(ctx, api.RescopeInput{ID: r.ID, Title: "Send invoices by e-mail", Details: "By e-mail, with the PDF.", Note: "paper is gone", By: session})
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.Title != "Send invoices by e-mail" || !strings.Contains(got.Request.Details, "By e-mail, with the PDF.") ||
		!strings.Contains(got.Request.Details, "paper is gone") || !strings.Contains(got.Request.Details, "«Send invoices»") || !strings.Contains(got.Request.Details, "> By mail.") {
		t.Fatalf("rescoped: %q / %q; want the new wording with the old one kept", got.Request.Title, got.Request.Details)
	}
	if impl := stepOf(t, ctx, r.ID, "implement"); !strings.Contains(impl.Title, "Send invoices by e-mail") {
		t.Errorf("a step not started: %q; want the new title", impl.Title)
	}
	if now := stepOf(t, ctx, r.ID, "analyse"); now.Title != analyse.Title {
		t.Errorf("the running step: title %q; want it not rewritten", now.Title)
	}
	if g, err := req.Get(ctx, api.GetInput{ID: r.ID}); err != nil || g.Request.Plan == nil ||
		!slices.ContainsFunc(g.Request.Plan.Stale, func(s api.StaleStep) bool { return s.Todo == analyse.ID }) {
		t.Errorf("the running step is not recorded stale on its plan: %v", err)
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: analyse.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "analysed", By: session}}); err != nil {
		t.Fatal(err)
	}
	g, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	steps := g.Request.Plan.Steps
	if i := slices.IndexFunc(steps, func(s api.StepRecord) bool { return strings.HasSuffix(s.Step, "analyse") }); i < 0 || !strings.Contains(steps[i].Details, "Ran while its request changed") {
		t.Errorf("the stale step's record: %+v; want it to say it ran while its request changed", steps)
	}
}

func TestActualizedASupersededRequestHandsItsDependentsToItsSuccessor(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	x := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild, By: person})
	y := file(t, ctx, req, api.FileInput{Title: "Print and send invoices", Module: "billing", Kind: api.KindBuild, By: person})
	z := file(t, ctx, req, api.FileInput{Title: "Archive printed invoices", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := req.Depend(ctx, api.DependInput{ID: z.ID, On: x.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Supersede(ctx, api.SupersedeInput{ID: x.ID, With: x.ID, Note: "itself", By: session}); err == nil {
		t.Error("a request superseding itself: accepted")
	}
	got, err := req.Supersede(ctx, api.SupersedeInput{ID: x.ID, With: y.ID, Note: "y covers it", By: session})
	if err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, x.ID); st != api.PlanDropped {
		t.Errorf("superseded: %s; want dropped", st)
	}
	if got.Request.ID != x.ID {
		t.Errorf("answered %s; want %s", got.Request.ID, x.ID)
	}
	zz, err := req.Get(ctx, api.GetInput{ID: z.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(zz.Request.DependsOn, y.ID) || slices.Contains(zz.Request.DependsOn, x.ID) {
		t.Errorf("its dependent waits on %v; want %s, not %s", zz.Request.DependsOn, y.ID, x.ID)
	}
	tr, err := inboxOf(t, ctx).Triage(ctx, inboxapi.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(tr.Entries, func(e inboxapi.Entry) bool {
		return strings.Contains(e.Title, x.ID) && strings.Contains(e.Title, "superseded")
	}) {
		t.Errorf("the requester was not told: %+v", tr.Entries)
	}
	if !strings.Contains(requestPanel(t, ctx, x.ID), "Superseded by "+y.ID) || !strings.Contains(requestPanel(t, ctx, y.ID), "Supersedes "+x.ID) {
		t.Error("the request panels do not link the superseded request and its successor")
	}
	if _, err := req.Supersede(ctx, api.SupersedeInput{ID: x.ID, With: y.ID, Note: "again", By: session}); err == nil {
		t.Error("an ended request superseded again: accepted")
	}
}

func TestActualizedAMergedRequestCarriesItsContentIntoAnother(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	uc := want(t, ctx, cwm, "use-case", "Send invoices by mail")
	x := file(t, ctx, req, api.FileInput{Title: "Mail invoices", Module: "billing", Kind: api.KindBuild, Details: "Each month.", By: person,
		Deliverables: []api.Deliverable{{Title: "a mail per invoice"}, {Title: "the PDF attached"}}})
	if _, err := req.Assess(ctx, api.AssessInput{ID: x.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	y := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{{Title: "the PDF attached"}}})
	if _, err := req.Merge(ctx, api.MergeInput{ID: x.ID, Into: y.ID, Note: "one piece of work", By: session}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: y.ID})
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, d := range got.Request.Deliverables {
		titles = append(titles, d.Title)
	}
	if !slices.Equal(titles, []string{"the PDF attached", "a mail per invoice"}) {
		t.Errorf("merged deliverables %v; want its own then the new one, once", titles)
	}
	if !slices.Contains(got.Request.Serves, uc) || !strings.Contains(got.Request.Details, "Merged from "+x.ID) || !strings.Contains(got.Request.Details, "> Each month.") {
		t.Errorf("merged into: serves %v, details %q; want its want and its words carried over", got.Request.Serves, got.Request.Details)
	}
	if st := planState(t, ctx, x.ID); st != api.PlanDropped {
		t.Errorf("merged: %s; want dropped", st)
	}
	// Into work that runs: refused.
	w := file(t, ctx, req, api.FileInput{Title: "Remind late payers", Module: "billing", Kind: api.KindBuild, By: person})
	v := file(t, ctx, req, api.FileInput{Title: "Remind by mail", Module: "billing", Kind: api.KindBuild, By: person})
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, w.ID, api.StepIntake).ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Merge(ctx, api.MergeInput{ID: v.ID, Into: w.ID, Note: "the same", By: session}); err == nil {
		t.Error("a merge into a request whose work runs: accepted")
	}
}
