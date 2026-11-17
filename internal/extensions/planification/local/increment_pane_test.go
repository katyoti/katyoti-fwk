// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN INCREMENT'S GOALS TAB DRAWS EACH REQUEST IN FULL (
// F-095): each delivery goal in detail, each request serving it with its open
// step and every step named, the constraints marked; an unknown tab is
// refused.
func TestAnIncrementsGoalsTabDrawsEachRequestInFull(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	cwmapp.Module(t, ctx, cwm, "shop")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default", gradedVerification{status: map[string]string{}})
	g := want(t, ctx, cwm, "goal", "Customers are paid on time")
	accepted(t, ctx, cwm, g)
	paid, mailed, releasable := "My customers pay me on time", "My invoices reach them by mail", "Releasable when it ends"
	inc := increments(t, ctx)
	out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: "Invoices get paid", Description: "Customers pay on time because their invoices reach them.",
		Goals: []string{paid, mailed, releasable}, By: person})
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
		{"op": api.OpIncrementTarget, "object": g, "increment": in.ID, "for": paid, "why": "the module already wants it"},
		{"op": api.OpFeatureAdd, "module": "shop", "title": "Mail the invoices", "under": g, "for": mailed, "why": "no module mails them"},
	}})
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, in.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: g, Increment: in.ID, Demand: in.ID}); err != nil {
		t.Fatal(err)
	}
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
	report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": g}, {"op": 1, "id": f}}})
	// Its plan filed the feature's build: the feature carries its request.
	rq := f

	page := func(tab string) (render.Def, string) {
		t.Helper()
		def, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: tab})
		if err != nil {
			t.Fatalf("the %s tab: %v", tab, err)
		}
		if err := render.ValidatePage(def.DSLTree); err != nil {
			t.Errorf("the %s tab is not a valid page: %v", tab, err)
		}
		raw, _ := json.Marshal(def.DSLTree)
		return def, string(raw)
	}
	has := func(where, s string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(s, w) {
				t.Errorf("%s lacks %s", where, w)
			}
		}
	}

	// The Goals tab - each request the shared request row, dense, its open
	// step named under its dots, every step named on hover.
	_, gt := page("goals")
	has("the Goals tab", gt, `"data-view":"increment-goals-detail"`, "Mail the invoices", `"data-request":"`+rq+`"`, `"data-dense":true`, `"variant":"dots"`, "delivery constraint")

	if _, err := inc.PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "nowhere"}); err == nil {
		t.Error("an unknown tab was answered")
	}
}
