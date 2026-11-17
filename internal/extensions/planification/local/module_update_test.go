// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REQUEST THAT CHANGES A MODULE ENDS WITH ITS MODULE UP TO DATE: it names
// the want it serves (the intake is proposed one, never given one), its
// module-update step records the acceptance check it changed and its change
// log, and its end — its last step's, or a finish by hand through the
// module-updated gate — is refused while one is missing. Exempt: the gate off
// (no date configured), another template, a request filed before the date,
// one a gate filed, a split demand, a person's waiver.

// criterionPack declares an acceptance check: an object carrying checkable.
var criterionPack = fstest.MapFS{
	"object-types/criterion.yaml": {Data: []byte("name: criterion\nfolder: criteria\nid_prefix: AC\ncapabilities: [container, checkable]\n")},
}

// gatedWorld is the world with the module-updated gate on since since.
func gatedWorld(t *testing.T, since string) (context.Context, cwmapi.CWMService, api.RequestService) {
	t.Helper()
	return worldConfigured(t, criterionPack, map[string]string{"Planification.yaml": "module_update:\n  since: \"" + since + "\"\n"})
}

// criterion creates an acceptance check under parent.
func criterion(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, parent, title string) string {
	t.Helper()
	o := cwmapi.Object{Type: "criterion"}
	o.Parent, o.Name = parent, title
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o})
	if err != nil {
		t.Fatalf("create the criterion %q: %v", title, err)
	}
	return out.Object.ID
}

// The intake names the want a request serves: assessing a build with none is
// answered the wants it could serve, and nothing is written; a serves on an
// object that wants nothing is refused; one on a use case is an edge.
func TestAssessProposesTheWantARequestServesAndWritesNothing(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	uc := want(t, ctx, cwm, "use-case", "Send invoices by mail")

	out, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Priority: "P1", By: intake})
	if err != nil {
		t.Fatal(err)
	}
	if out.Proposal == nil || !slices.ContainsFunc(out.Proposal.Candidates, func(c api.ServeCandidate) bool { return c.ID == uc }) {
		t.Fatalf("a build serving no want: proposal %+v; want %s among its candidates", out.Proposal, uc)
	}
	got, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: r.ID})
	if got.Object.EdgeTo(api.EdgeServes) != "" || len(out.Request.Serves) != 0 {
		t.Fatalf("the proposal was written: %+v", got.Object.Edges)
	}

	other := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindAnalyse})
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Serves: other.ID, By: intake}); err == nil || !strings.Contains(err.Error(), "wants nothing") {
		t.Fatalf("serving a request (no want): %v; want refused", err)
	}
	if out, err = req.Assess(ctx, api.AssessInput{ID: r.ID, Serves: uc, By: intake}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Request.Serves, []string{uc}) || out.Proposal != nil {
		t.Fatalf("serves %s: %v, proposal %+v; want the edge, no proposal", uc, out.Request.Serves, out.Proposal)
	}
	if a, err := req.Assess(ctx, api.AssessInput{ID: other.ID, By: intake}); err != nil || a.Proposal != nil {
		t.Fatalf("an analysis changes no module: proposal %+v, %v", a.Proposal, err)
	}
	// A request filed on a want serves it: nothing proposed.
	f := want(t, ctx, cwm, "feature", "Mail them")
	on := file(t, ctx, req, api.FileInput{On: f, Kind: api.KindFix})
	if a, err := req.Assess(ctx, api.AssessInput{ID: on.ID, By: intake}); err != nil || a.Proposal != nil {
		t.Fatalf("a fix filed on a feature: proposal %+v, %v; want none (it serves the feature)", a.Proposal, err)
	}
}

// The end of a build's last step asks for its module up to date before its
// node moves: refused, its module-update step is planned again with what is
// missing; recorded — the want, a changed check, the change log — it ends.
func TestARequestEndsOnlyWithItsModuleUpToDate(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := gatedWorld(t, "2020-01-01")
	uc := want(t, ctx, cwm, "use-case", "Send invoices by mail")
	ac := criterion(t, ctx, cwm, uc, "A mail leaves for each invoice")
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, r.ID, "review").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "good", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := state(t, ctx, req, r.ID); st == api.StateDone {
		t.Fatal("ended with its module not up to date")
	}
	again := stepOf(t, ctx, r.ID, "module-update (again)")
	feedback, _ := again.Packet.Params["feedback"].(string)
	for _, missing := range []string{"serves no want", "no acceptance check", "no change-log entry"} {
		if !strings.Contains(feedback, missing) || !strings.Contains(again.Packet.Instructions, missing) {
			t.Errorf("the module update planned again: feedback %q; want it to name %q", feedback, missing)
		}
	}
	if again.Skill != "ktw-agents:/skill/module-update" || again.Packet.Gate != "module-updated" || again.Packet.Moves != "" {
		t.Errorf("the step planned again: skill %s, gate %s, moves %q", again.Skill, again.Packet.Gate, again.Packet.Moves)
	}

	// What its step records: a change no user sees says why; a check names
	// an acceptance check.
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Internal: true, By: session}); err == nil {
		t.Error("a change no user sees, without a reason: accepted")
	}
	if got, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Internal: true, Reason: "a test only", By: session}); err != nil ||
		got.Request.ModuleUpdate == nil || got.Request.ModuleUpdate.UserVisible == nil || *got.Request.ModuleUpdate.UserVisible || !got.Request.ModuleUpdate.Recorded() {
		t.Fatalf("a change no user sees: %+v, %v; want user_visible false with its reason", got.Request.ModuleUpdate, err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Checks: []string{uc}, By: session}); err == nil {
		t.Error("a use case recorded as an acceptance check (it carries no checkable): accepted")
	}
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Serves: uc, By: session}); err != nil {
		t.Fatal(err)
	}
	rel, _ := changeLogRelease(t, ctx)
	entry, err := changesOf(t, ctx).Add(ctx, api.AddChangeInput{Release: rel, Title: "Les factures partent par mail", Domain: "Interface", Value: api.ValueMajor, Text: "x", Requests: []string{r.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := req.ModuleUpdate(ctx, api.ModuleUpdateInput{ID: r.ID, Changelog: entry.Change.ID, Checks: []string{ac}, By: session}); err != nil {
		t.Fatal(err)
	}
	// The check existed before the request: unchanged since, it proves nothing.
	if out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: r.ID}); err != nil || out.Accept || !strings.Contains(out.Reason, "has not changed") {
		t.Fatalf("a check unchanged since the filing: %+v, %v; want refused, saying so", out, err)
	}
	o, _ := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: ac})
	o.Object.Description = "One mail per invoice, with its PDF."
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "billing", Object: o.Object}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the check's change journaled", func() (string, bool) {
		out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: r.ID})
		return out.Reason, err == nil && out.Accept
	})
	pullAndReport(t, ctx, again.ID, session, schedapi.Result{Summary: "the use case, its check and the change log updated"})
	if st := state(t, ctx, req, r.ID); st != api.StateDone {
		t.Fatalf("its module up to date: %s; want done", st)
	}
}

// Ended by hand, a request is held by the module-updated gate on its plan's
// finish, naming what is missing; a person's waiver — refused from an agent
// or an agent's session — lets it end, and shows on its panel and timeline.
func TestAFinishByHandIsHeldUntilTheModuleIsUpToDateOrWaived(t *testing.T) {
	t.Parallel()
	ctx, _, req := gatedWorld(t, "2020-01-01")
	r := file(t, ctx, req, api.FileInput{Title: "Send invoices by mail", Module: "billing", Kind: api.KindBuild})
	plans := plansOf(t, ctx)
	if _, err := plans.Finish(ctx, api.FinishInput{ID: r.ID, By: person, Note: "done by hand"}); err == nil || !strings.Contains(err.Error(), "serves no want") {
		t.Fatalf("finished by hand, its module not up to date: %v; want refused by the module-updated gate, naming what is missing", err)
	}
	if _, err := req.Waive(ctx, api.WaiveInput{ID: r.ID, Note: "a rename", By: session}); err == nil {
		t.Error("an agent waived it")
	}
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	if _, err := req.Waive(agent, api.WaiveInput{ID: r.ID, Note: "a rename", By: person}); err == nil {
		t.Error("an agent's session waived it as the person")
	}
	if _, err := req.Waive(ctx, api.WaiveInput{ID: r.ID, By: person}); err == nil {
		t.Error("waived without saying why")
	}
	w, err := req.Waive(ctx, api.WaiveInput{ID: r.ID, Note: "a label renamed, nothing to prove", By: person})
	if err != nil || w.Request.Waiver == nil || w.Request.Waiver.By.ID != person.ID {
		t.Fatalf("waived: %+v, %v", w.Request.Waiver, err)
	}
	page, err := views(t, ctx).Request(ctx, api.RequestViewInput{ID: r.ID, Language: "fr"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page)
	if !strings.Contains(string(raw), "Dispensée") || !strings.Contains(string(raw), "a label renamed") {
		t.Errorf("the request panel does not show the waiver: %s", raw)
	}
	if _, err := plans.Finish(ctx, api.FinishInput{ID: r.ID, By: person, Note: "done by hand"}); err != nil {
		t.Fatalf("waived, finished by hand: %v", err)
	}
	hist, err := req.Demands(ctx, api.DemandsInput{Module: "billing", View: api.DemandsHistory})
	if err != nil || len(hist.Demands) != 1 || hist.Demands[0].ID != r.ID {
		t.Fatalf("its history: %+v, %v", hist.Demands, err)
	}
	if d := hist.Demands[0]; !slices.ContainsFunc(d.Timeline, func(e api.TimelineEntry) bool { return e.Title == "Module update waived" && e.By == person.ID }) {
		t.Errorf("its timeline: %+v; want the waiver", hist.Demands[0].Timeline)
	}
}

// What is not held: another template, a request filed before the date, one a
// gate filed, a split demand — and nothing at all while no date is set.
func TestTheModuleUpdatedGateExemptions(t *testing.T) {
	t.Parallel()
	ctx, _, req := gatedWorld(t, "2020-01-01")
	plans := plansOf(t, ctx)
	a := file(t, ctx, req, api.FileInput{Title: "What does billing need?", Module: "billing", Kind: api.KindAnalyse})
	if _, err := plans.Finish(ctx, api.FinishInput{ID: a.ID, By: person}); err != nil {
		t.Errorf("an analysis (its template changes no module): %v", err)
	}
	g := file(t, ctx, req, api.FileInput{Title: "Make it pass", Module: "billing", Kind: api.KindFix,
		Source: &api.RequestSource{Kind: api.SourceGate, Name: "ready-to-test/verification-green", Object: "billing"}})
	if _, err := plans.Finish(ctx, api.FinishInput{ID: g.ID, By: person}); err != nil {
		t.Errorf("a fix a gate filed: %v", err)
	}
	s := file(t, ctx, req, api.FileInput{Title: "Send and print", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Split(ctx, api.SplitInput{ID: s.ID, Parts: []api.SplitPart{{Module: "billing", Title: "Send"}}, By: person}); err != nil {
		t.Fatal(err)
	}
	if out, err := req.CheckModuleUpdated(ctx, api.ModuleUpdatedInput{ID: s.ID}); err != nil || !out.Accept || !strings.Contains(out.Reason, "split") {
		t.Errorf("a split demand: %+v, %v; want exempt", out, err)
	}

	later, _, lreq := gatedWorld(t, "2999-01-01")
	b := file(t, later, lreq, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	if _, err := plansOf(t, later).Finish(later, api.FinishInput{ID: b.ID, By: person}); err != nil {
		t.Errorf("a build filed before the date: %v", err)
	}
	off, _, oreq := world(t)
	c := file(t, off, oreq, api.FileInput{Title: "Send invoices", Module: "billing", Kind: api.KindBuild})
	if out, err := oreq.CheckModuleUpdated(off, api.ModuleUpdatedInput{ID: c.ID}); err != nil || !out.Accept || !strings.Contains(out.Reason, "off") {
		t.Errorf("no date configured: %+v, %v; want the gate off", out, err)
	}
}
