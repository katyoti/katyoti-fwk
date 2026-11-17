// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A DELIVERABLE A PERSON'S APPROVAL VERIFIES — an analyse request filed with
// a deliverable no check verifies has its accept step verify it: the step is
// planned to verify every such deliverable (WorkPacket.Verifies), the Request panel says a person's
// approval verifies it, and approving the step verifies it — by the person,
// the approval as evidence — so the request ends done.
func TestAnApprovalVerifiesTheDeliverableItNames(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Find what invoices miss", Module: "billing", Kind: api.KindAnalyse,
		Deliverables: []api.Deliverable{{Title: "the findings"}}})
	accept := stepOf(t, ctx, r.ID, "accept")
	if accept.Packet.Verifies != api.VerifiesUnchecked {
		t.Fatalf("the accept step verifies %q; want every deliverable no check verifies", accept.Packet.Verifies)
	}
	if page := requestPanel(t, ctx, r.ID); !strings.Contains(page, "verified when a person approves accept") {
		t.Error("the request panel does not say the accept step verifies it")
	}
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: accept.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "the findings are right", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("after the person approved accept: %s; want done", st)
	}
	d := deliverableOf(t, ctx, req, r.ID, "the findings")
	if !d.Verified || d.VerifiedBy == nil || d.VerifiedBy.ID != person.ID || d.VerifiedAt == "" ||
		!strings.HasPrefix(d.Evidence, "approved step accept: the findings are right") {
		t.Fatalf("the findings: %+v; want verified by %s, the approval as evidence", d, person.ID)
	}
	if page := requestPanel(t, ctx, r.ID); !strings.Contains(page, "approved at step accept by jdoe") {
		t.Error("the request panel does not say the approval verified it")
	}
}

// A rejection verifies nothing: the deliverable stays unverified, the work is
// done again and the person asked again — the step asked again still verifies it.
func TestARejectionLeavesTheDeliverableUnverified(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Find what invoices miss", Module: "billing", Kind: api.KindAnalyse,
		Deliverables: []api.Deliverable{{Title: "the findings"}}})
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	accept := stepOf(t, ctx, r.ID, "accept")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: accept.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "the costs are missing", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	if d := deliverableOf(t, ctx, req, r.ID, "the findings"); d.Verified {
		t.Fatalf("the findings after a rejection: %+v; want unverified", d)
	}
	if st := planState(t, ctx, r.ID); st == api.PlanDone {
		t.Fatal("the request ended after a rejection")
	}
	again := stepOf(t, ctx, r.ID, "accept")
	if again.ID == accept.ID || again.Packet.Verifies != api.VerifiesUnchecked {
		t.Fatalf("the accept asked again: %s verifies %q; want a new one verifying the findings", again.ID, again.Packet.Verifies)
	}
}

// An agent's session never answers for a person: reporting a person's step or
// verifying a deliverable by hand from it is refused, whatever by says.
func TestAnAgentSessionNeverVerifiesForAPerson(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Print invoices", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "seen on paper", ByHand: "a person checks it; no agent can"}}})
	toReview(t, ctx, r.ID)
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	review := stepOf(t, ctx, r.ID, "review")
	approve := schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "fine", Verdict: api.VerdictApproved, By: person}}
	if _, err := schedOf(t, ctx).Report(agent, approve); err == nil {
		t.Fatal("an agent's session reported the person's review")
	}
	if _, err := schedOf(t, ctx).Report(ctx, approve); err != nil {
		t.Fatal(err)
	}
	verify := api.VerifyInput{ID: r.ID, Deliverable: "seen on paper", Evidence: "printed and read", By: person}
	if _, err := req.Verify(agent, verify); err == nil || !strings.Contains(err.Error(), "agent's session") {
		t.Fatalf("an agent's session verifying for the person: %v; want refused", err)
	}
	if d := deliverableOf(t, ctx, req, r.ID, "seen on paper"); d.Verified {
		t.Fatalf("verified from an agent's session: %+v", d)
	}
	if _, err := req.Verify(ctx, verify); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Fatalf("after the person verified it: %s; want done", st)
	}
}

func deliverableOf(t *testing.T, ctx context.Context, req api.RequestService, id, title string) api.Deliverable {
	t.Helper()
	got, err := req.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got.Request.Deliverables {
		if d.Title == title {
			return d
		}
	}
	t.Fatalf("%s has no deliverable %q", id, title)
	return api.Deliverable{}
}

// requestPanel is the Request panel of a request, as JSON.
func requestPanel(t *testing.T, ctx context.Context, id string) string {
	t.Helper()
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	def, err := views.Request(ctx, api.RequestViewInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	return string(raw)
}

// With several deliverables only a person can check, one approval verifies
// every one of them, each with the approval as evidence; one a check verifies
// is left to its check, and the panel says which the approval verifies.
func TestApprovalVerifiesEveryDeliverableOnlyAPersonCanCheck(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Find what invoices miss", Module: "billing", Kind: api.KindAnalyse,
		Deliverables: []api.Deliverable{{Title: "the findings"}, {Title: "the costs"}, {Title: "it builds", Check: "go version"}}})
	accept := stepOf(t, ctx, r.ID, "accept")
	if page := requestPanel(t, ctx, r.ID); strings.Count(page, "verified when a person approves accept") != 2 {
		t.Errorf("the request panel does not say the approval verifies both the findings and the costs")
	}
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: accept.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "all right", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"the findings", "the costs"} {
		if d := deliverableOf(t, ctx, req, r.ID, title); !d.Verified || !strings.HasPrefix(d.Evidence, "approved step accept") {
			t.Errorf("%s after the approval: %+v; want verified, the approval as evidence", title, d)
		}
	}
	if d := deliverableOf(t, ctx, req, r.ID, "it builds"); strings.HasPrefix(d.Evidence, "approved step") {
		t.Errorf("a deliverable with a check was verified by the approval: %+v", d)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanDone {
		t.Errorf("after the approval: %s; want done", st)
	}
}
