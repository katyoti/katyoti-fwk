// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A GOAL'S REQUESTS ON THE PANE ARE AMONG THOSE THE SUMMARY COUNTS
// : a delivery goal's requests are read from what the
// increment's applied proposal made of it, and the summary counts the
// requests of what the increment holds. One list decides both: a feature the
// proposal made and a person then took out of the increment (untarget) is
// still what the goal became, but its request is counted nowhere - neither on
// the goal nor in the header - and the header's total is the summary's.
func TestAnIncrementsGoalRequestsAreAmongThoseItsSummaryCounts(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	cwmapp.Module(t, ctx, cwm, "shop")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default", gradedVerification{status: map[string]string{}})
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	accepted(t, ctx, cwm, g)
	mailed := "My invoices reach them by mail"
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices reach them", Description: "Invoices reach the customers.",
		Goals: []string{mailed}, By: person})
	if err != nil {
		t.Fatal(err)
	}
	in := out.Increment
	runIncrement(t, ctx, in.ID)
	if st := stepOf(t, ctx, in.ID, api.StepIntake); st.ID != "" {
		pullAndReport(t, ctx, st.ID, session, schedapi.Result{Summary: "an increment's proposal"})
	}
	s := schedOf(t, ctx)
	report := func(id string, more map[string]any) {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}}); err != nil {
			t.Fatal(err)
		}
	}
	report(stepOf(t, ctx, in.ID, "analyse").ID, nil)
	report(stepOf(t, ctx, in.ID, api.StepReconcile).ID, map[string]any{api.DeltaKey: []map[string]any{
		{"op": api.OpFeatureAdd, "module": "shop", "title": "Mail the invoices", "under": g, "for": mailed, "why": "no module mails them"},
	}})
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, in.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	// The feature is made under the goal, which the increment does not target.
	fo := cwmapi.Object{Type: "feature"}
	fo.Parent, fo.Name = "shop", "Mail the invoices"
	fo.Edges = []cwmapi.ObjectEdge{{Kind: api.EdgeServes, To: g}}
	_ = fo.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "shop", Object: fo})
	if err != nil {
		t.Fatal(err)
	}
	f := made.Object.ID
	apply := stepOf(t, ctx, in.ID, api.StepApplyDelta).ID
	report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": f}}})

	if _, err := inc.Untarget(ctx, api.UntargetInput{ID: f}); err != nil {
		t.Fatal(err)
	}
	scope, err := inc.Scope(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range scope.Targets {
		if tg.ID == f || tg.ID == g {
			t.Fatalf("the case needs a feature in no target's reach; %s is targeted", tg.ID)
		}
	}
	d, err := inc.Detail(ctx, api.IncrementDetailInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Goals) == 0 || len(d.Goals[0].Became) == 0 || d.Goals[0].Became[0].ID != f {
		t.Fatalf("the case needs the goal G1 to have become the feature: %+v", d.Goals)
	}
	def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "goals"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(def.DSLTree)
	if strings.Contains(string(raw), `"data-request":"`+f+`"`) {
		t.Errorf("the goal G1 lists the request of a feature the increment no longer holds: %s", raw)
	}

	sum, err := inc.Summary(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if n := sum.Awaiting + sum.Running + sum.ToDo + sum.WaitingOn + sum.Idle + sum.Blocked + sum.RequestsDone; n != sum.Requests {
		t.Errorf("the summary's request total %d is not the sum of its buckets %d: %+v", sum.Requests, n, sum)
	}
	// None: the feature is out of it, and the increment itself (its
	// proposal) is never one of its requests.
	if sum.Requests != 0 {
		t.Errorf("the summary counts %d requests; want 0: %+v", sum.Requests, sum)
	}
	for _, tab := range []string{"open-requests", "goals"} {
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: tab})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		if head := fmt.Sprintf(`"data-done":%d`, sum.RequestsDone); !strings.Contains(string(raw), head) || !strings.Contains(string(raw), `"data-view":"increment-page-counts"`) {
			t.Errorf("the %s tab's head does not count the summary's %d done: %s", tab, sum.RequestsDone, raw)
		}
	}
}
