// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gatesapi "github.com/katyoti/katyoti-fwk/internal/extensions/gates/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE WORK ON THE SCHEDULER — planification puts a request's to-dos in its
// module's queue (files out of CWM), releases them when they may run, and
// collects each result: what lasts is written on the request; the to-do is
// archived. Workers only pull and report.

var session = cwmapi.Actor{ID: "session-a", Kind: "agent"}

// another is a second session: an agent holds one to-do at a time
// , so a step held while others are worked is another's.
var another = cwmapi.Actor{ID: "session-b", Kind: "agent"}

func workOf(t *testing.T, ctx context.Context) api.WorkService {
	t.Helper()
	s, err := application.UseService[api.WorkService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func schedOf(t *testing.T, ctx context.Context) schedapi.TodoService {
	t.Helper()
	s, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func triggersOf(t *testing.T, ctx context.Context) api.TriggerService {
	t.Helper()
	s, err := application.UseService[api.TriggerService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// stepOf is the request's scheduled to-do of a step.
func stepOf(t *testing.T, ctx context.Context, request, step string) api.WorkTodo {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range got.Todos {
		if td.Packet.Step == step {
			return td
		}
	}
	t.Fatalf("no scheduled %s to-do in %+v", step, got.Todos)
	return api.WorkTodo{}
}

// steps is the request's scheduled steps, in order.
func steps(t *testing.T, ctx context.Context, request string) []string {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, td := range got.Todos {
		out = append(out, td.Packet.Step)
	}
	return out
}

// pullAndReport pulls a given to-do as a worker and reports its result.
func pullAndReport(t *testing.T, ctx context.Context, id string, by cwmapi.Actor, r schedapi.Result) schedapi.TodoOutput {
	t.Helper()
	s := schedOf(t, ctx)
	// The step a report releases is released by the module's reaction to it:
	// on a loaded runner it may still be on its way.
	var got schedapi.PullOutput
	eventually(t, "a pull of "+id, func() (string, bool) {
		var err error
		if got, err = s.Pull(ctx, schedapi.PullInput{ID: id, By: by}); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%s %v", got.Nothing, got.Waiting), got.Todo != nil
	})
	if r.Status == "" {
		r.Status = schedapi.StatusSuccess
	}
	if r.Summary == "" {
		r.Summary = "done"
	}
	r.By = by
	out, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: r})
	if err != nil {
		t.Fatalf("report %s: %v", id, err)
	}
	return out
}

// Filing a request plans it at once: its template's to-dos in its module's
// queue, as files out of CWM — the intake first and ready, each other step
// waiting on the one before, each with its work packet; planned twice is
// refused unless again, which archives the to-dos not started.
func TestFilingPlansTheRequestIntoItsModulesQueue(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild, Priority: "P1"})
	if got := strings.Join(steps(t, ctx, r.ID), ","); got != "intake,analyse,implement,auto-review,module-update,review" {
		t.Fatalf("steps %s; want intake, analyse, implement, module-update, review", got)
	}
	intake, analyse, review := stepOf(t, ctx, r.ID, "intake"), stepOf(t, ctx, r.ID, "analyse"), stepOf(t, ctx, r.ID, "review")
	if intake.State != schedapi.StateReady || analyse.State != schedapi.StateWaiting || analyse.Waits != "after "+intake.ID ||
		len(analyse.After) != 1 || analyse.After[0] != intake.ID {
		t.Fatalf("intake %s; analyse %s (%q) after %v; want intake ready, analyse waiting on it", intake.State, analyse.State, analyse.Waits, analyse.After)
	}
	if analyse.Skill != "ktw-agents:/skill/requirements-analysis" || !analyse.Packet.Refines || analyse.Packet.Instructions == "" ||
		analyse.Packet.Phase != api.PhasePreparation || analyse.Priority != "P1" || analyse.Target != r.ID || analyse.Packet.Template != "build" ||
		analyse.Owner != api.Owner || analyse.Start != schedapi.StartAuto || analyse.For != schedapi.ForAgent {
		t.Errorf("analyse's ticket: %+v", analyse)
	}
	// The review is an agent's - the reviewer's - by default.
	if review.For != schedapi.ForAgent || review.Packet.Task != "reviewer" || review.Packet.Interaction != api.InteractionValidate || review.Packet.Moves != "done" {
		t.Errorf("review: %+v; want the reviewer's, validating, moving the request done", review)
	}
	// The files, out of CWM: no object, no edge names a to-do.
	space := planningSpace(t, ctx, cwm, "billing")
	entries, err := os.ReadDir(filepath.Join(space, "todo"))
	if err != nil || len(entries) < 4 {
		t.Fatalf("billing's todo folder: %v, %v", entries, err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: intake.ID}); err == nil {
		t.Error("a to-do is a CWM object")
	}

	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, By: person}); err == nil {
		t.Error("a request planned twice")
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps(t, ctx, r.ID), ","); got != "analyse,implement,auto-review,module-update,review" {
		t.Errorf("planned again without intake: %s", got)
	}
	if old, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: intake.ID}); err != nil || old.Todo.Archived == "" || old.Todo.Note != "planned again" {
		t.Errorf("the first intake, planned again: %+v, %v; want archived", old.Todo, err)
	}
}

// The template follows the request: quick, a feature accepted.
func TestThePlanFollowsTheRequestsTemplate(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	quick := file(t, ctx, req, api.FileInput{Title: "Rename a label", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Assess(ctx, api.AssessInput{ID: quick.ID, Depth: api.DepthQuick, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: quick.ID, Again: true, By: person}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(steps(t, ctx, quick.ID), ","); got != "intake,implement,auto-review,review" || stepOf(t, ctx, quick.ID, "implement").Packet.Template != "build-quick" {
		t.Errorf("a quick build: %s", got)
	}
	f := want(t, ctx, cwm, "feature", "Send by mail")
	accepted(t, ctx, cwm, f)
	on := file(t, ctx, req, api.FileInput{Title: "Specify it", On: f})
	if got := strings.Join(steps(t, ctx, on.ID), ","); got != "intake,specify,challenge,size" {
		t.Fatalf("a feature accepted: %s", got)
	}
	challenge, size := stepOf(t, ctx, on.ID, "challenge"), stepOf(t, ctx, on.ID, "size")
	if challenge.Packet.Confirms != "ready-to-build/feature-challenged" || !size.Packet.Refines || size.Packet.Moves != "specified" || size.Target != f {
		t.Errorf("challenge %+v; size %+v", challenge.Packet, size.Packet)
	}
	d := file(t, ctx, req, api.FileInput{Title: "What does billing do?", Module: "billing", Kind: api.KindDiscover})
	if got := strings.Join(steps(t, ctx, d.ID), ","); got != "intake,survey,propose,review" || stepOf(t, ctx, d.ID, "propose").Skill != "ktw-methods:/skill/discover-wants" {
		t.Errorf("a discover request: %s", got)
	}
}

// A worker pulls and reports; planification decides what follows — a planning
// step adds a step, the person's rejected review has the work done again with
// their note, their approval ends the request — and writes each step's outcome
// on the request; every collected to-do is archived.
func TestPlanificationDecidesWhatFollowsEachResult(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson, Deliverables: []api.Deliverable{{Title: "It builds", Check: "go version"}}})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "intake").ID, session, schedapi.Result{Summary: "clear, one piece"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "analyse").ID, session, schedapi.Result{Summary: "needs a layout first",
		More: map[string]any{"add": []map[string]any{{"step": "design", "task": "architect", "instructions": "Sketch the invoice layout."}}}})
	design, implement := stepOf(t, ctx, r.ID, "design"), stepOf(t, ctx, r.ID, "implement")
	if design.State != schedapi.StateReady || !slices.Contains(implement.After, design.ID) || implement.State != schedapi.StateWaiting {
		t.Fatalf("after the analysis: design %s, implement after %v (%s)", design.State, implement.After, implement.State)
	}
	pullAndReport(t, ctx, design.ID, session, schedapi.Result{})
	pullAndReport(t, ctx, implement.ID, session, schedapi.Result{Summary: "built"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	if next, _ := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{By: session, Filter: schedapi.Filter{Request: r.ID}}); next.Todo != nil {
		t.Errorf("an agent pulled while the person reviews: %+v", next.Todo)
	}
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "seen", By: person}}); err == nil {
		t.Error("a review collected without a verdict")
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "the totals are wrong", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	redo, again := stepOf(t, ctx, r.ID, "implement (rework)"), stepOf(t, ctx, r.ID, "review")
	if redo.State != schedapi.StateReady || redo.Packet.Params["feedback"] != "the totals are wrong" || again.ID == review.ID || again.Packet.Moves != "done" || !slices.Contains(again.After, redo.ID) {
		t.Fatalf("rejected: rework %+v; review again %+v", redo, again)
	}
	if got, _ := req.Get(ctx, api.GetInput{ID: r.ID}); got.Request.State == api.StateDone {
		t.Fatal("the request ended on a rejected review")
	}
	ar, mu := stepOf(t, ctx, r.ID, "auto-review (rework)"), stepOf(t, ctx, r.ID, "module-update (rework)")
	if !slices.Contains(ar.After, redo.ID) || mu.State != schedapi.StateWaiting || !slices.Contains(mu.After, ar.ID) || !slices.Contains(again.After, mu.ID) {
		t.Fatalf("rejected: the auto-review %+v and the module update %+v redone; want them after the implementation redone, the review again after both", ar, mu)
	}
	pullAndReport(t, ctx, redo.ID, session, schedapi.Result{Summary: "totals fixed"})
	pullAndReport(t, ctx, ar.ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, mu.ID, session, schedapi.Result{Summary: "module up to date again"})
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: again.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "good", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Get(ctx, api.GetInput{ID: r.ID})
	if err != nil || got.Request.State != api.StateDone || !got.Request.Deliverables[0].Verified {
		t.Fatalf("approved: %+v, %v; want done, its deliverable verified", got.Request, err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Errorf("still scheduled: %v; want every to-do archived", left)
	}
	// What lasts is on the request: each step's outcome, by whom.
	plan := got.Request.Plan
	var outcomes []string
	for _, s := range plan.Steps {
		if s.Outcome != "" {
			outcomes = append(outcomes, s.Step+"="+s.Outcome)
		}
	}
	if len(outcomes) != 11 || !slices.Contains(outcomes, "build/review=success") || plan.Steps[len(plan.Steps)-1].By == nil ||
		!strings.Contains(plan.Steps[len(plan.Steps)-1].Result, "approved") {
		t.Errorf("the request's history: %v; want the eleven steps' outcomes (the auto-reviews among them), the last approved by the person", outcomes)
	}
	if ru, _ := workOf(t, ctx).Rollup(ctx, api.RollupInput{Request: r.ID}); ru.State != api.RollupDone || ru.Done != ru.Total {
		t.Errorf("rollup %+v", ru)
	}
}

// A result's other outcomes: a person's answer passes on, a failure waits for
// a retry, a follow-up is filed by the module, an intake that makes the request
// quick plans it again, a deliverable that fails blocks the request.
func TestAResultsOtherOutcomes(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix totals", Module: "billing", Kind: api.KindFix, Estimate: 1, Deliverables: []api.Deliverable{{Title: "It works", Check: "exit 3"}}})
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	implement := stepOf(t, ctx, r.ID, "implement")
	ask, err := workOf(t, ctx).Add(ctx, api.AddWorkInput{Request: r.ID, Step: "decide", Before: []string{implement.ID}, Doer: api.DoerPerson,
		Interaction: api.InteractionDecide, Instructions: "Round per line or per invoice?", Phase: api.PhasePreparation, By: person})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: ask.Todo.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "decided", By: person}}); err == nil {
		t.Error("a decision collected without the answer")
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: ask.Todo.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "decided", Answer: "per invoice", By: person}}); err != nil {
		t.Fatal(err)
	}
	if im := stepOf(t, ctx, r.ID, "implement"); im.Packet.Params["answer"] != "per invoice" || im.State != schedapi.StateReady {
		t.Errorf("implement after the decision: %+v", im)
	}
	pullAndReport(t, ctx, implement.ID, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "the build broke"})
	if im := stepOf(t, ctx, r.ID, "implement"); im.State != schedapi.StateWaiting || !strings.HasPrefix(im.Waits, "fail: the build broke") {
		t.Fatalf("after the failure: %s %q", im.State, im.Waits)
	}
	if ru, _ := workOf(t, ctx).Rollup(ctx, api.RollupInput{Request: r.ID}); ru.State != api.RollupBlocked {
		t.Errorf("rollup after a failure: %+v", ru)
	}
	if _, err := workOf(t, ctx).Retry(ctx, api.RetryWorkInput{ID: implement.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	out := pullAndReport(t, ctx, implement.ID, session, schedapi.Result{Summary: "fixed",
		More: map[string]any{"follow_ups": []map[string]any{{"own": true, "title": "Totals in the PDF too", "kind": "fix"}}}})
	if !strings.Contains(out.Note, "collected by planification") {
		t.Errorf("report note %q", out.Note)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "module-update").ID, session, schedapi.Result{Summary: "module up to date"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "verify").ID, session, schedapi.Result{Summary: "checked", Verdict: api.VerdictApproved})
	if got, _ := req.Get(ctx, api.GetInput{ID: r.ID}); got.Request.State != api.PlanBlocked {
		t.Errorf("the request whose check fails: %s, want blocked", got.Request.State)
	}
	if fu, _ := req.List(ctx, api.ListInput{Module: "billing", Requester: "billing"}); len(fu.Requests) != 1 || fu.Requests[0].Title != "Totals in the PDF too" {
		t.Errorf("billing's follow-ups: %+v", fu.Requests)
	}

	q := file(t, ctx, req, api.FileInput{Title: "Rename a label", Module: "billing", Kind: api.KindBuild})
	intake := stepOf(t, ctx, q.ID, "intake")
	if _, err := req.Assess(ctx, api.AssessInput{ID: q.ID, Depth: api.DepthQuick, By: person}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, intake.ID, session, schedapi.Result{Summary: "small"})
	if im := stepOf(t, ctx, q.ID, "implement"); im.Packet.Template != "build-quick" || im.State != schedapi.StateReady {
		t.Errorf("after a quick intake: implement from %s, %s", im.Packet.Template, im.State)
	}
}

// A plan is edited before it runs; a to-do a worker has is not changed.
func TestAPlanIsEditedBeforeItRuns(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	w := workOf(t, ctx)
	r := file(t, ctx, req, api.FileInput{Title: "Ship invoices", Module: "billing", Kind: api.KindBuild})
	intake, analyse, implement := stepOf(t, ctx, r.ID, "intake"), stepOf(t, ctx, r.ID, "analyse"), stepOf(t, ctx, r.ID, "implement")
	ed, err := w.Edit(ctx, api.EditWorkInput{ID: implement.ID, Priority: "P0", Params: map[string]any{"scope": "pdf"}, Start: schedapi.StartManual, By: person})
	if err != nil || ed.Todo.Priority != "P0" || ed.Todo.Packet.Params["scope"] != "pdf" || ed.Todo.Start != schedapi.StartManual || ed.Todo.Packet.Instructions != implement.Packet.Instructions {
		t.Fatalf("edited: %+v, %v", ed.Todo, err)
	}
	if _, err := w.Edit(ctx, api.EditWorkInput{ID: implement.ID, Skill: "not-a-skill", By: person}); err == nil {
		t.Error("a skill that is not a skill URI was accepted")
	}
	design, err := w.Add(ctx, api.AddWorkInput{Request: r.ID, Step: "design", After: []string{analyse.ID}, Before: []string{implement.ID},
		Task: "architect", Instructions: "Sketch the invoice layout.", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if design.Todo.State != schedapi.StateWaiting || !slices.Contains(stepOf(t, ctx, r.ID, "implement").After, design.Todo.ID) {
		t.Errorf("design %+v", design.Todo)
	}
	if _, err := w.Move(ctx, api.MoveWorkInput{ID: analyse.ID, After: []string{design.Todo.ID}, By: person}); err == nil {
		t.Error("a cycle was made")
	}
	if _, err := w.Remove(ctx, api.RemoveWorkInput{ID: analyse.ID, By: person}); err == nil {
		t.Error("removed without why")
	}
	if _, err := w.Remove(ctx, api.RemoveWorkInput{ID: analyse.ID, Note: "already analysed", By: person}); err != nil {
		t.Fatal(err)
	}
	if d := stepOf(t, ctx, r.ID, "design"); !slices.Contains(d.After, intake.ID) {
		t.Errorf("design after %v; want after the intake, what analyse came after", d.After)
	}
	if _, err := w.Remove(ctx, api.RemoveWorkInput{ID: stepOf(t, ctx, r.ID, "review").ID, Note: "x", By: person}); err == nil {
		t.Error("the step that moves the request was removed")
	}
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: intake.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Edit(ctx, api.EditWorkInput{ID: intake.ID, Priority: "P3", By: person}); err == nil {
		t.Error("a pulled to-do was edited")
	}
}

// A request's to-dos wait on what it depends on: its intake never; its
// preparation until the dependency's own preparation is done; the rest until
// the dependency ended. A review holds the work about what changed.
func TestToDosWaitOnDependenciesAndReviews(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	first := file(t, ctx, req, api.FileInput{Title: "Invoice model", Module: "billing", Kind: api.KindBuild, Estimate: 1})
	then := file(t, ctx, req, api.FileInput{Title: "Invoice PDF", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Depend(ctx, api.DependInput{ID: then.ID, On: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: first.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	intake := stepOf(t, ctx, then.ID, "intake")
	if a := stepOf(t, ctx, then.ID, "analyse"); a.State != schedapi.StateWaiting {
		t.Errorf("the dependent's analysis after depend: %s %q; want waiting", a.State, a.Waits)
	}
	if intake.State != schedapi.StateReady {
		t.Errorf("the dependent's intake: %s %q; an intake never waits", intake.State, intake.Waits)
	}
	pullAndReport(t, ctx, intake.ID, session, schedapi.Result{Summary: "clear"})
	if a := stepOf(t, ctx, then.ID, "analyse"); !strings.Contains(a.Waits, first.ID) {
		t.Errorf("the dependent's analysis: %q; want waiting on %s", a.Waits, first.ID)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, first.ID, "analyse").ID, session, schedapi.Result{Summary: "analysed"})
	analyse := stepOf(t, ctx, then.ID, "analyse")
	if analyse.State != schedapi.StateReady {
		t.Errorf("the dependent's analysis once %s is analysed: %s %q", first.ID, analyse.State, analyse.Waits)
	}
	pullAndReport(t, ctx, analyse.ID, session, schedapi.Result{Summary: "analysed"})
	if im := stepOf(t, ctx, then.ID, "implement"); !strings.Contains(im.Waits, first.ID) {
		t.Errorf("the dependent's build: %q; want waiting on %s", im.Waits, first.ID)
	}

	// A goal's change: its review holds the work about it, one priority above.
	g := want(t, ctx, cwm, "goal", "Invoices are sent")
	work := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild, About: g})
	raised, err := triggersOf(t, ctx).Raise(ctx, api.RaiseInput{Source: api.RequestSource{Kind: api.SourceTrigger, Name: "goal-changed", Object: g, Change: "its body changed"},
		Kind: api.KindReview, Title: "Review the work on the changed goal", Holds: true})
	if err != nil {
		t.Fatal(err)
	}
	review, _ := req.Get(ctx, api.GetInput{ID: raised.Request})
	if !raised.Filed || raised.Held < 3 || review.Request.Priority != "P1" {
		t.Fatalf("raised %+v, priority %s; want the work's to-dos held, the review at P1", raised, review.Request.Priority)
	}
	if i := stepOf(t, ctx, work.ID, "intake"); i.State != schedapi.StateWaiting || i.Waits != "held by the review of "+raised.Request {
		t.Errorf("the work's intake during the review: %s %q", i.State, i.Waits)
	}
	again, _ := triggersOf(t, ctx).Raise(ctx, api.RaiseInput{Source: api.RequestSource{Kind: api.SourceTrigger, Name: "goal-changed", Object: g, Change: "its body changed again"},
		Kind: api.KindReview, Title: "Review the work on the changed goal", Holds: true})
	if again.Filed || again.Request != raised.Request {
		t.Errorf("fired again: %+v; want appended", again)
	}
	for _, s := range []string{"intake", "review"} {
		td := stepOf(t, ctx, raised.Request, s)
		r := schedapi.Result{Summary: "the work still serves the goal"}
		if td.For == schedapi.ForPerson {
			if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: td.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
				t.Fatal(err)
			}
			continue
		}
		pullAndReport(t, ctx, td.ID, session, r)
	}
	if i := stepOf(t, ctx, work.ID, "intake"); i.State != schedapi.StateReady {
		t.Errorf("the work's intake after the review: %s %q; want ready", i.State, i.Waits)
	}
}

// A person's to-do is asked as a human task once it is ready
// — the question names the request and its step, never
// the to-do — and their report is its result.
func TestAPersonIsAskedInTheInbox(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Rename the total", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson, Estimate: 1})
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Depth: api.DepthQuick, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	if review := stepOf(t, ctx, r.ID, "review"); review.Packet.Ask != nil {
		t.Fatalf("asked before it is ready: %+v", review.Packet)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "renamed"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	review := stepOf(t, ctx, r.ID, "review")
	ask := review.Packet.Ask
	if ask == nil {
		t.Fatal("the person's review was not asked")
	}
	if strings.Contains(ask.Title+ask.Question+ask.AskedBy, review.ID) || ask.AskedBy != "planification:"+r.ID+"/review" {
		t.Errorf("the question %+v names the to-do, or not the request and its step", ask)
	}
	pullAndReport(t, ctx, review.ID, person, schedapi.Result{Verdict: api.VerdictApproved, Summary: "good"})
	slowly(t, "the request done from the person's approval", func() (string, bool) {
		got, err := req.Get(ctx, api.GetInput{ID: r.ID})
		return fmt.Sprint(got.Request.State, err), err == nil && got.Request.State == api.StateDone
	})
}

// The Work view shows a module's to-dos by state.
func TestTheWorkViewShowsAModulesToDos(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Fix totals", Module: "billing", Kind: api.KindFix, Priority: "P1"})
	intake := stepOf(t, ctx, r.ID, "intake")
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: intake.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	out, err := views(t, ctx).Work(ctx, api.WorkInput{Module: "billing", Lang: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	page := text(t, out, "work/billing")
	if !strings.Contains(page, "`"+intake.ID+"`") || !strings.Contains(page, "| `"+r.ID+"` | in-progress") || !strings.Contains(page, "after "+intake.ID) {
		t.Errorf("the Work view:\n%s", page)
	}
}

// A person's approval of a step that confirms a gate requirement records the
// confirmation on the node.
func TestAnApprovalConfirmsItsGateRequirement(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Send by mail")
	accepted(t, ctx, cwm, f)
	on := file(t, ctx, req, api.FileInput{Title: "Specify it", On: f, Estimate: 1})
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: on.ID, Again: true, NoIntake: true, By: person}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, on.ID, "specify").ID, session, schedapi.Result{Summary: "specified"})
	challenge := stepOf(t, ctx, on.ID, "challenge")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: challenge.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "clear", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	got, err := gatesOf(t, ctx).Check(ctx, gatesapi.CheckInput{ID: f, Gate: api.GateReadyToBuild})
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range got.Checklist.Items {
		if it.Name == "feature-challenged" && it.ConfirmedBy == nil {
			t.Errorf("feature-challenged after the approval: %+v", it)
		}
	}
}

// raised is the open requests a source raised on an object.
func raised(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, name, object string) []api.RequestFields {
	t.Helper()
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: api.CapRequest})
	if err != nil {
		t.Fatal(err)
	}
	var out []api.RequestFields
	for _, o := range got.Objects {
		f, _, _ := cwmapi.DecodeCapability[api.RequestFields](o, api.CapRequest)
		if f.Source != nil && f.Source.Name == name && f.Source.Object == object {
			out = append(out, f)
		}
	}
	return out
}

// Through CWM's own hooks: an accepted goal whose body changes has its review
// filed; planning's own writes on it fire nothing; a feature that comes to want
// more than seven use cases fails its rule once.
func TestTriggersFireOnRealChanges(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	g := want(t, ctx, cwm, "goal", "Invoices are sent")
	accepted(t, ctx, cwm, g)
	o, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: g})
	if err != nil {
		t.Fatal(err)
	}
	o.Object.Body = "Invoices are sent the day they are issued."
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o.Object}); err != nil {
		t.Fatal(err)
	}
	slowly(t, "the goal's review filed", func() (string, bool) {
		got := raised(t, ctx, cwm, "goal-changed", g)
		return fmt.Sprint(got), len(got) == 1 && strings.Contains(got[0].Source.Change, "its body changed")
	})
	if _, err := req.File(ctx, api.FileInput{On: g, Kind: api.KindAnalyse, By: person}); err != nil {
		t.Fatal(err)
	}
	planlocal.Settle(ctx, 3*time.Second)
	if got := raised(t, ctx, cwm, "goal-changed", g); len(got) != 1 || got[0].Source.Times != 1 {
		t.Errorf("after a request filed on the goal: %+v", got)
	}
	f := want(t, ctx, cwm, "feature", "Invoices by mail")
	accepted(t, ctx, cwm, f)
	for i := 1; i <= 9; i++ {
		uc := want(t, ctx, cwm, "use-case", fmt.Sprintf("Send case %d", i))
		if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: f, To: uc, Kind: "wants"}); err != nil {
			t.Fatal(err)
		}
		if i == 7 {
			planlocal.Settle(ctx, 3*time.Second)
			if got := raised(t, ctx, cwm, "feature-size", f); len(got) != 0 {
				t.Fatalf("the size rule fired at seven use cases: %+v", got)
			}
		}
	}
	slowly(t, "the size rule fired", func() (string, bool) {
		got := raised(t, ctx, cwm, "feature-size", f)
		return fmt.Sprint(got), len(got) == 1 && got[0].Kind == api.KindAnalyse
	})
}

// A module's channel: In, Out, the answers it received.
func TestAModulesChannel(t *testing.T) {
	t.Parallel()
	w := newPlannerWorld(t)
	ctx := w.ctx
	shipping := cwmapi.Actor{ID: "shipping", Kind: "module"}
	asked := file(t, ctx, w.req, api.FileInput{Title: "Weight on the invoice?", Module: "billing", Kind: api.KindQuestion, By: shipping})
	work := file(t, ctx, w.req, api.FileInput{Title: "Rename the total", Module: "billing", Kind: api.KindBuild})
	followUp := file(t, ctx, w.req, api.FileInput{Title: "Totals in the PDF", Module: "billing", Kind: api.KindFix, By: cwmapi.Actor{ID: "billing", Kind: "module"}})
	ch, err := w.req.Channel(ctx, api.ChannelInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	from := map[string]string{}
	for _, it := range ch.In {
		from[it.ID] = it.From
	}
	if from[asked.ID] != "shipping (module)" || !strings.Contains(from[work.ID], "(person)") || from[followUp.ID] == "" || len(ch.Out) != 0 {
		t.Fatalf("billing's channel: %+v", ch)
	}
	out, _ := w.req.Channel(ctx, api.ChannelInput{Module: "shipping"})
	if len(out.Out) != 1 || out.Out[0].ID != asked.ID || len(out.Answers) != 0 {
		t.Fatalf("shipping's channel: %+v", out)
	}
	if _, err := w.req.Answer(ctx, api.AnswerInput{ID: asked.ID, Text: "Yes, in kilograms.", By: session}); err != nil {
		t.Fatal(err)
	}
	if out, _ = w.req.Channel(ctx, api.ChannelInput{Module: "shipping"}); len(out.Answers) != 1 || out.Answers[0].Answer != "Yes, in kilograms." {
		t.Errorf("shipping's answers: %+v", out.Answers)
	}
}

// slowly is eventually with room for hooks dispatched behind a loaded suite.
func slowly(t *testing.T, what string, check func() (string, bool)) string {
	t.Helper()
	deadline := time.Now().Add(eventuallyWithin)
	for {
		v, ok := check()
		if ok {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s and still no %s (last seen: %s)", eventuallyWithin, what, v)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// planningSpace is a module's .ktw folder.
func planningSpace(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module string) string {
	t.Helper()
	got, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(got.Path)
	if filepath.Base(dir) == "objects" {
		dir = filepath.Dir(dir)
	}
	return filepath.Join(dir, ".ktw")
}

// A module asking another awaits its answer: a wait with a deadline, in the
// scheduler's files; the request ended, the wait is cleared. A person's
// request, or one a module files in itself, awaits nothing.
func TestAModuleAwaitsTheAnswerItAsked(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	cwmapp.Module(t, ctx, cwm, "shipping")
	waits, err := application.UseService[schedapi.WaitService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	shipping := cwmapi.Actor{ID: "shipping", Kind: "module"}
	asked := file(t, ctx, req, api.FileInput{Title: "Give the parcel weights", Module: "billing", Kind: api.KindQuestion, By: shipping})
	file(t, ctx, req, api.FileInput{Title: "Rename the total", Module: "billing", Kind: api.KindFix})
	file(t, ctx, req, api.FileInput{Title: "Its own fix", Module: "billing", Kind: api.KindFix, By: cwmapi.Actor{ID: "billing", Kind: "module"}})
	got, err := waits.List(ctx, schedapi.ListWaitsInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Waits) != 1 || got.Waits[0].Module != "shipping" || got.Waits[0].On != asked.ID || got.Waits[0].From != "billing" {
		t.Fatalf("waits %+v; want shipping awaiting its question in billing", got.Waits)
	}
	if _, err := req.Answer(ctx, api.AnswerInput{ID: asked.ID, Text: "2 kg each", By: person}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the wait cleared", func() (string, bool) {
		left, _ := waits.List(ctx, schedapi.ListWaitsInput{})
		return fmt.Sprint(left.Waits), len(left.Waits) == 0
	})
}

// A request whose last step is collected while a deliverable has no check is
// refused its end; once the check is given and passes (or a person verifies it
// by hand) it ends — nothing else would try its end again.
func TestARequestEndsOnceItsChecksPass(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "they are sent", ByHand: "a person checks it; no agent can"}}})
	toReview(t, ctx, r.ID)
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Fatalf("after its last step with an unchecked deliverable: %s; want blocked", st)
	}
	if _, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: person, Note: "a check for it",
		Deliverables: []api.Deliverable{{Title: "they are sent", Check: "go version"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("after its check passed: %s; want done", st)
	}
}

// A person verifying by hand the deliverable no command can check ends the
// request refused its end for it.
func TestAPersonVerifyingEndsARequest(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "a person checks it; no agent can"}}})
	toReview(t, ctx, r.ID)
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Fatalf("before the person verified it: %s; want blocked", st)
	}
	if _, err := req.Verify(ctx, api.VerifyInput{ID: r.ID, Deliverable: "seen on paper", Evidence: "printed and read", By: person}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("after the person verified it: %s; want done", st)
	}
}

// A request ending releases at once the steps of the requests that wait on it
// — whoever ends it (here a person, by hand).
func TestARequestEndingReleasesWhatWaitsOnIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	first := file(t, ctx, req, api.FileInput{Title: "Model the invoice", Module: "billing", Kind: api.KindBuild})
	then := file(t, ctx, req, api.FileInput{Title: "Print the invoice", Module: "billing", Kind: api.KindBuild, DependsOn: []string{first.ID}})
	// Its preparation waits on the first's preparation; its build on the first's end.
	for _, id := range []string{first.ID, then.ID} {
		for _, step := range []string{api.StepIntake, "analyse"} {
			pullAndReport(t, ctx, stepOf(t, ctx, id, step).ID, session, schedapi.Result{Summary: step + " done"})
		}
	}
	if impl := stepOf(t, ctx, then.ID, "implement"); impl.State != schedapi.StateWaiting || !strings.Contains(impl.Waits, first.ID) {
		t.Fatalf("its implementation: %s, %q; want waiting on %s", impl.State, impl.Waits, first.ID)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: first.ID, By: person, Note: "done by hand"}); err != nil {
		t.Fatal(err)
	}
	if impl := stepOf(t, ctx, then.ID, "implement"); impl.State != schedapi.StateReady {
		t.Fatalf("after %s ended: its implementation %s (%q); want ready", first.ID, impl.State, impl.Waits)
	}
}

func planState(t *testing.T, ctx context.Context, id string) string {
	t.Helper()
	got, err := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	return got.Plan.State
}

// demandOf is a request's open demand, as the module's Requests panel reads it.
func demandOf(t *testing.T, ctx context.Context, req api.RequestService, id string) api.Demand {
	t.Helper()
	got, err := req.Demands(ctx, api.DemandsInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got.Demands {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no open demand %s", id)
	return api.Demand{}
}

// A request whose build step is pulled shows on its card as running, on that
// step, held by the worker that pulled it — not the state it was filed in.
func TestADemandShowsTheStepInProgressAndWhoHoldsIt(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, r.ID, "implement").ID, By: session}); err != nil {
		t.Fatal(err)
	}
	d := demandOf(t, ctx, req, r.ID)
	if d.State != "running" || d.Step == nil || d.Step.Step != "implement" {
		t.Fatalf("its card: %s, step %+v; want running on implement", d.State, d.Step)
	}
	if d.Step.HeldBy == nil || d.Step.HeldBy.ID != session.ID || d.Step.Until == "" || d.Step.NeedsYou {
		t.Fatalf("its step: %+v; want held by %s until its lease, not needing a person", d.Step, session.ID)
	}
}

// Only a request whose person's step is up says it needs you, with that step
// and where it was asked.
func TestOnlyADemandWithAPersonsStepUpNeedsYou(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	up := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild, Review: api.ReviewPerson})
	other := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, up.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	d := demandOf(t, ctx, req, up.ID)
	if d.Step == nil || !d.Step.NeedsYou || d.Step.Step != "review" || d.Step.Who != api.DoerPerson || d.Step.Interaction == "" {
		t.Fatalf("its card: %s, step %+v; want needing a person on review", d.State, d.Step)
	}
	if o := demandOf(t, ctx, req, other.ID); o.Step == nil || o.Step.NeedsYou {
		t.Fatalf("the other card: step %+v; want an agent's step, not needing a person", o.Step)
	}
}

// A dependency that ended is not shown as waited on; one still open is.
func TestAnEndedDependencyIsNotWaitedOn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	first := file(t, ctx, req, api.FileInput{Title: "Model the invoice", Module: "billing", Kind: api.KindBuild})
	then := file(t, ctx, req, api.FileInput{Title: "Print the invoice", Module: "billing", Kind: api.KindBuild, DependsOn: []string{first.ID}})
	if d := demandOf(t, ctx, req, then.ID); len(d.WaitsOn) != 1 || d.WaitsOn[0] != first.ID {
		t.Fatalf("before %s ended: waits on %v; want it", first.ID, d.WaitsOn)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: first.ID, By: person, Note: "done by hand"}); err != nil {
		t.Fatal(err)
	}
	if d := demandOf(t, ctx, req, then.ID); len(d.WaitsOn) != 0 {
		t.Fatalf("after %s ended: waits on %v; want nothing", first.ID, d.WaitsOn)
	}
}

// A demand with no to-do (a feature planned outside a work plan) reads as its
// plan says, not "no step scheduled".
func TestADemandWithNoToDoReadsAsItsPlan(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Send by mail")
	if _, err := plansOf(t, ctx).Plan(ctx, api.PlanNextInput{ID: f, By: person}); err != nil {
		t.Fatal(err)
	}
	if d := demandOf(t, ctx, req, f); d.State != api.PlanReady || d.Step != nil {
		t.Fatalf("a planned feature: %s, step %+v, why %q; want ready as its plan", d.State, d.Step, d.Why)
	}
}

// A person's step in a module whose scheduling is off is asked of no one: it
// does not say it needs you.
func TestAPersonsStepNotAskedDoesNotNeedYou(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	s := schedOf(t, ctx)
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "implement").ID, session, schedapi.Result{Summary: "built"})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "auto-review").ID, session, schedapi.Result{Summary: "no automatic review applies"})
	impl := stepOf(t, ctx, r.ID, "module-update")
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: impl.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mode(ctx, schedapi.ModeInput{Module: "billing", Mode: schedapi.ModeOff, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: impl.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "built", By: session}}); err != nil {
		t.Fatal(err)
	}
	if d := demandOf(t, ctx, req, r.ID); d.Step == nil || d.Step.Step != "review" || d.Step.NeedsYou {
		t.Fatalf("its card: step %+v; want on review, not needing a person (nobody was asked)", d.Step)
	}
}

// Dropping a request archives its to-dos not started — the one that moves its
// node included; a pulled one stays with its worker and its report is
// collected with nothing following from it.
func TestDroppingARequestArchivesItsWork(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	analyse := stepOf(t, ctx, r.ID, "analyse")
	impl := stepOf(t, ctx, r.ID, "implement").ID
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: analyse.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: person, Note: "a duplicate"}); err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 1 || left[0] != "analyse" {
		t.Fatalf("after its drop: %v scheduled; want only the pulled analysis", left)
	}
	if got, err := s.Get(ctx, schedapi.GetInput{ID: impl}); err != nil || got.Todo.Archived == "" || !strings.Contains(got.Todo.Note, "a duplicate") {
		t.Fatalf("its implementation after the drop: %+v, %v; want archived with the drop's note", got.Todo, err)
	}
	// Even a failure is collected as nothing: no step waits to be retried.
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: analyse.ID, Result: schedapi.Result{Status: schedapi.StatusFail, Summary: "gave up", By: session}}); err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Fatalf("after the pulled step reported: %v scheduled; want nothing", left)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDropped {
		t.Fatalf("its plan: %s; want still dropped", st)
	}
}

// A request dropped by hand, past the plans service, leaves no work either.
func TestARequestDroppedByHandLeavesNoWork(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	intakeID := stepOf(t, ctx, r.ID, api.StepIntake).ID
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: r.ID, To: api.PlanDropped, Transition: "drop", By: person, Note: "not needed any more", Capability: api.CapPlan}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if got, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: intakeID}); err != nil || !strings.Contains(got.Todo.Note, "not needed any more") {
			t.Errorf("its intake after the drop by hand: %+v, %v; want archived with the drop's note", got.Todo, err)
		}
	}()
	// The observer archives them meanwhile: a listing may miss a file moved.
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: r.ID})
		if err == nil && len(got.Todos) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("5s after its drop by hand: %+v, %v; want nothing scheduled", got.Todos, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A step reported with work added after its request was dropped adds nothing.
func TestAStepAddingWorkToADroppedRequestAddsNothing(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	analyse := stepOf(t, ctx, r.ID, "analyse")
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: analyse.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: person, Note: "a duplicate"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: analyse.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "analysed", By: session,
		More: map[string]any{"add": []map[string]any{{"step": "design", "task": "architect", "instructions": "Sketch the invoice layout."}}}}}); err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Fatalf("after the step reported with work added: %v scheduled; want nothing", left)
	}
}

// A step whose report would be refused (a verdict from an agent) after its
// request was dropped is collected as nothing, not put back in the queue.
func TestARefusedReportOnADroppedRequestIsNotOfferedAgain(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	analyse := stepOf(t, ctx, r.ID, "analyse")
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: analyse.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, Outcome: api.PlanDropped, By: person, Note: "a duplicate"}); err != nil {
		t.Fatal(err)
	}
	_, _ = s.Report(ctx, schedapi.ReportInput{ID: analyse.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: session}})
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Fatalf("after the refused report: %v scheduled; want nothing", left)
	}
}

// A revise with a note and no deliverables list keeps the deliverables (it
// records the note); an explicit empty list removes them.
func TestARevisionWithoutAListKeepsTheDeliverables(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "they are sent", Check: "go version"}, {Title: "they are paid"}}})
	got, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: person, Note: "the person's direction: mail them on Mondays"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Request.Deliverables); n != 2 || !strings.Contains(got.Request.Details, "mail them on Mondays") {
		t.Fatalf("after a revise with a note only: %d deliverables, details %q; want both kept and the note recorded", n, got.Request.Details)
	}
	got, err = req.Revise(ctx, api.ReviseInput{ID: r.ID, By: person, Note: "none is wanted any more", Deliverables: []api.Deliverable{}})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(got.Request.Deliverables); n != 0 {
		t.Fatalf("after an explicit empty list: %d deliverables; want none", n)
	}
}

// A revise with a note only, on a request one of whose deliverables is already
// verified, records the note and keeps that verification — the kept
// deliverables are not checked again as if filed anew.
func TestANoteOnlyRevisionKeepsAVerifiedDeliverable(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "they are sent", Check: "go version"}, {Title: "they are paid"}}})
	if _, err := req.Check(ctx, api.CheckInput{ID: r.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Revise(ctx, api.ReviseInput{ID: r.ID, By: person, Note: "the person's direction: mail them on Mondays"})
	if err != nil {
		t.Fatalf("a note-only revise with a verified deliverable: %v; want it recorded", err)
	}
	d := got.Request.Deliverables
	if len(d) != 2 || !d[0].Verified || d[0].VerifiedAt == "" || d[1].Verified || !strings.Contains(got.Request.Details, "mail them on Mondays") {
		t.Fatalf("after a note-only revise: %+v, details %q; want the verified one kept verified, the other unverified, the note recorded", d, got.Request.Details)
	}
}

// Finishing a request by hand (filed, then done at once) archives its to-dos
// not started; a pulled one's report is collected as nothing.
func TestFinishingARequestArchivesItsWork(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	intakeStep := stepOf(t, ctx, r.ID, api.StepIntake)
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: intakeStep.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: r.ID, By: person, Note: "done already, by hand"}); err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 1 || left[0] != api.StepIntake {
		t.Fatalf("after its finish: %v scheduled; want only the pulled intake", left)
	}
	// Even a failure is collected as nothing: no step waits to be retried.
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: intakeStep.ID, Result: schedapi.Result{Status: schedapi.StatusFail, Summary: "gave up", By: session}}); err != nil {
		t.Fatal(err)
	}
	if left := steps(t, ctx, r.ID); len(left) != 0 {
		t.Fatalf("after the pulled step reported: %v scheduled; want nothing", left)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("its plan: %s; want still done", st)
	}
}

// use-case-changed fires on what judges the use case — its probe — never on a
// note alone, nor on a use case with nothing to run.
func TestUseCaseChangedFiresOnlyOnWhatIsJudged(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	update := func(id string, change func(o *cwmapi.Object)) {
		t.Helper()
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		o := got.Object
		change(&o)
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	checkable := func(kind, probe, note string) func(o *cwmapi.Object) {
		return func(o *cwmapi.Object) {
			g := map[string]any{"kind": kind, "note": note}
			if probe != "" {
				g["probe"] = probe
			}
			_ = o.SetCapability("checkable", g)
		}
	}
	unmeasured := want(t, ctx, cwm, "use-case", "Send by mail")
	update(unmeasured, checkable("none", "", "no probe yet"))
	update(unmeasured, checkable("none", "", "still no probe, said better"))
	update(unmeasured, func(o *cwmapi.Object) { o.Name = "Send invoices by mail" })
	probed := want(t, ctx, cwm, "use-case", "Print them")
	times := func() int {
		n := 0
		for _, r := range raised(t, ctx, cwm, "use-case-changed", probed) {
			n += r.Source.Times
		}
		return n
	}
	// Firings travel through CWM's events before planification sees them, so
	// each wait is on a firing seen, never on a pause: the probe given fires
	// once, and the probe changed after the note-only change is the barrier
	// that one firing of the note-only change would have come before.
	update(probed, checkable("command", "go test ./a", "first"))
	slowly(t, "the probe given fired", func() (string, bool) {
		return fmt.Sprint(times()), times() >= 1
	})
	planlocal.Settle(ctx, 3*time.Second)
	before := times()
	update(probed, checkable("command", "go test ./a", "the note only"))
	update(probed, checkable("command", "go test ./b", "the probe changed"))
	slowly(t, "the probe's change fired", func() (string, bool) {
		return fmt.Sprint(times()), times() > before
	})
	planlocal.Settle(ctx, 3*time.Second)
	if after := times(); after != before+1 {
		t.Errorf("a note-only change fired (%d firings after the note and the probe changed, %d before)", after, before)
	}
	if got := raised(t, ctx, cwm, "use-case-changed", unmeasured); len(got) != 0 {
		t.Errorf("a use case with nothing to run fired: %+v", got)
	}
}

// A request worked with no plan - its filing lost it - records nothing and its
// end is refused; planning it gives it its plan, records the steps it ran and
// ends it, without planning its work again.
func TestPlanningARequestWithNoPlanRecordsItsStepsAndEndsIt(t *testing.T) {
	// Not in parallel: its observer's moves race the other worlds' on a loaded machine.
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Rename a label", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "It builds", Check: "go version"}}})
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Depth: api.DepthQuick, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, Again: true, By: person}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectRemoveCapability(ctx, cwmapi.ObjectRemoveCapabilityInput{ID: r.ID, Capability: api.CapPlan}); err != nil {
		t.Fatal(err)
	}
	ran := steps(t, ctx, r.ID)
	for _, step := range ran {
		td := stepOf(t, ctx, r.ID, step)
		res := schedapi.Result{Status: schedapi.StatusSuccess, Summary: step + " done", By: session}
		if td.Packet.Interaction == api.InteractionValidate {
			res.Verdict = api.VerdictApproved
		}
		if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: td.ID, By: session}); err != nil {
			t.Fatal(err)
		}
		_, _ = schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: td.ID, Result: res}) // its end is refused: no plan
	}
	if left := steps(t, ctx, r.ID); len(left) > 0 {
		t.Fatalf("its work ran, yet %v is left", left)
	}
	if _, err := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: r.ID}); err == nil {
		t.Fatal("the request still carries a plan: nothing was lost")
	}

	if _, err := workOf(t, ctx).Plan(ctx, api.PlanWorkInput{Request: r.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	got, err := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.State != api.PlanDone {
		t.Errorf("planned: %s (%s); want done - its work ran and its check passes", got.Plan.State, got.Plan.Note)
	}
	if len(got.Plan.Steps) != len(ran) {
		t.Errorf("recorded %d steps %+v; want the %d it ran (%v)", len(got.Plan.Steps), got.Plan.Steps, len(ran), ran)
	}
	if left := steps(t, ctx, r.ID); len(left) > 0 {
		t.Errorf("its work was planned again: %v", left)
	}
}
