// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT THAT CHANGES SCREENS PLANS ITS DISPLAY SWITCH FIRST:
// a reconcile adding a feature to an increment in a module whose code draws a
// screen is sent back while the increment holds no display switch; with a
// feature-add of role display-switch it is kept, and once applied the switch
// is tagged and the screen feature depends on it - built first, wherever the
// delta lists it.
func TestAnIncrementChangingScreensPlansItsSwitchFirst(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldWith(t, fstest.MapFS{"capabilities/code.yaml": {Data: []byte("name: code\n")}})
	screens := cwmapi.Object{ID: "screens", Type: "module"}
	screens.Name = "screens"
	_ = screens.SetCapability("code", map[string]any{"code_packages": []any{"app/ktw-gui"}})
	_ = screens.SetCapability("planned", map[string]any{})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "screens", Object: screens}); err != nil {
		t.Fatal(err)
	}
	inc := increment(t, ctx, "Invoices are seen on screen")
	d := file(t, ctx, req, api.FileInput{Title: "The invoice pane", Module: "billing", Kind: api.KindDemand, About: inc.ID, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	pane := map[string]any{"op": api.OpFeatureAdd, "module": "screens", "title": "The invoice pane shows its lines", "why": "the demand"}
	mail := map[string]any{"op": api.OpFeatureAdd, "module": "billing", "title": "Invoices are mailed", "why": "the demand"}
	err := report(rec, map[string]any{api.DeltaKey: []map[string]any{pane, mail}})
	if err == nil || !strings.Contains(err.Error(), "holds no display switch") || !strings.Contains(err.Error(), "delta[0]") || strings.Contains(err.Error(), "delta[1]") {
		t.Fatalf("a screen feature in an increment with no display switch: %v; want delta[0] sent back, naming the switch", err)
	}
	sw := map[string]any{"op": api.OpFeatureAdd, "module": "screens", "title": "The invoice pane is behind its switch", "role": api.RoleDisplaySwitch, "why": "R-006"}
	if err := report(rec, map[string]any{api.DeltaKey: []map[string]any{pane, sw}}); err != nil {
		t.Fatalf("with its display switch: %v", err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	mk := func(title string) string {
		t.Helper()
		o := cwmapi.Object{Type: "feature", Parent: "screens"}
		o.Name = title
		_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
		out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "screens", Object: o})
		if err != nil {
			t.Fatal(err)
		}
		return out.Object.ID
	}
	paneID := mk("The invoice pane shows its lines")
	swID := mk("The invoice pane is behind its switch")
	apply := stepOf(t, ctx, d.ID, api.StepApplyDelta).ID
	if err := report(apply, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": paneID}, {"op": 1, "id": swID}}}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "screens", ID: swID})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Object.Capabilities[api.CapDisplaySwitch]; !ok {
		t.Errorf("the switch %s is not tagged display-switch: %v", swID, got.Object.Capabilities)
	}
	p, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "screens", ID: paneID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(p.Object.Edges, func(e cwmapi.ObjectEdge) bool { return e.Kind == api.EdgeDependsOn && e.To == swID }) {
		t.Errorf("the screen feature %s does not depend on its switch %s: %+v", paneID, swID, p.Object.Edges)
	}
}

// A FEATURE THAT DRAWS NO SCREEN NEEDS NO SWITCH: in a
// module whose code ships a screen, a feature-add with the role no-screen (a
// guard, a migration, a parser) passes the reconcile with no display switch,
// and once applied the feature carries the no-screen tag and depends on no
// switch; the same feature-add without the role is still sent back.
func TestANoScreenFeatureNeedsNoDisplaySwitch(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldWith(t, fstest.MapFS{"capabilities/code.yaml": {Data: []byte("name: code\n")}})
	screens := cwmapi.Object{ID: "screens", Type: "module"}
	screens.Name = "screens"
	_ = screens.SetCapability("code", map[string]any{"code_packages": []any{"app/ktw-gui"}})
	_ = screens.SetCapability("planned", map[string]any{})
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "screens", Object: screens}); err != nil {
		t.Fatal(err)
	}
	inc := increment(t, ctx, "Invoices migrate")
	d := file(t, ctx, req, api.FileInput{Title: "The invoice migration", Module: "billing", Kind: api.KindDemand, About: inc.ID, By: person})
	pullAndReport(t, ctx, stepOf(t, ctx, d.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a demand"})
	s := schedOf(t, ctx)
	rec := stepOf(t, ctx, d.ID, api.StepReconcile).ID
	report := func(id string, more map[string]any) error {
		t.Helper()
		if _, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil {
			t.Fatal(err)
		}
		_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "done", By: session, More: more}})
		return err
	}
	guard := map[string]any{"op": api.OpFeatureAdd, "module": "screens", "title": "Old invoices migrate on load", "why": "the demand"}
	if err := report(rec, map[string]any{api.DeltaKey: []map[string]any{guard}}); err == nil || !strings.Contains(err.Error(), "holds no display switch") {
		t.Fatalf("a feature of a screen module without a role: %v; want it sent back for its display switch", err)
	}
	guard["role"] = api.RoleNoScreen
	if err := report(rec, map[string]any{api.DeltaKey: []map[string]any{guard}}); err != nil {
		t.Fatalf("a no-screen feature of a screen module with no display switch: %v; want it kept", err)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: stepOf(t, ctx, d.ID, "approve").ID,
		Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "yes", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	o := cwmapi.Object{Type: "feature", Parent: "screens"}
	o.Name = "Old invoices migrate on load"
	_ = o.SetCapability(cwmapi.CapWant, map[string]any{"reachability": "closable"})
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "screens", Object: o})
	if err != nil {
		t.Fatal(err)
	}
	if err := report(stepOf(t, ctx, d.ID, api.StepApplyDelta).ID, map[string]any{api.AppliedKey: []map[string]any{{"op": 0, "id": out.Object.ID}}}); err != nil {
		t.Fatal(err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "screens", ID: out.Object.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Object.Capabilities[api.CapNoScreen]; !ok {
		t.Errorf("the no-screen feature %s is not tagged no-screen: %v", out.Object.ID, got.Object.Capabilities)
	}
	if slices.ContainsFunc(got.Object.Edges, func(e cwmapi.ObjectEdge) bool { return e.Kind == api.EdgeDependsOn }) {
		t.Errorf("the no-screen feature %s depends on a switch: %+v", out.Object.ID, got.Object.Edges)
	}
}
