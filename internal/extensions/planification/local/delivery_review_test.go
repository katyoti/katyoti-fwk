// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN INCREMENT IS DELIVERED ONLY WHEN THE USER CAN USE WHAT IT DELIVERS
// : its delivery approval starts
// with an agent's delivery review, and the gate usable-delivered on its done
// accepts it only while that review passed and still holds - every screen
// feature's GUI test passed, every display switch retired or turned on by an
// increment of the same release, a demo path, each follow-up in an increment
// of the same release. A person forces it with a note.

// forcedNote is the note a test that is not about the delivery review closes
// an increment with: a person's, past usable-delivered.
const forcedNote = "closed for the test - forced past usable-delivered"

// demoPath is the demo path a passing review writes.
const demoPath = "Open Billing, pick an invoice, press Print: the invoice prints"

// reviewDelivery passes an increment's delivery review: the agent's step,
// approved with a demo path and what more gives.
func reviewDelivery(t *testing.T, ctx context.Context, inc string, more map[string]any) schedapi.TodoOutput {
	t.Helper()
	review := map[string]any{"demo": demoPath}
	for k, v := range more {
		review[k] = v
	}
	return pullAndReport(t, ctx, stepOf(t, ctx, inc, "delivery-review").ID, session,
		schedapi.Result{Summary: "the user can use it", Verdict: api.VerdictApproved, More: map[string]any{"review": review}})
}

// reportReview reports a delivery review as given, answering the
// scheduler's note (a refused report goes back to the queue).
func reportReview(t *testing.T, ctx context.Context, inc string, r schedapi.Result) schedapi.TodoOutput {
	t.Helper()
	s := schedOf(t, ctx)
	id := stepOf(t, ctx, inc, "delivery-review").ID
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the delivery review %s: %v %+v", id, err, got)
	}
	r.Status, r.By = schedapi.StatusSuccess, session
	out, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: r})
	if err != nil {
		t.Fatalf("report the delivery review %s: %v", id, err)
	}
	return out
}

// reviewWorld is a release with an increment shipping in it.
func reviewWorld(t *testing.T) (context.Context, cwmapi.CWMService, api.RequestService, api.Release, api.Increment) {
	t.Helper()
	ctx, cwm, req := world(t)
	r := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.4.0"})
	in := incrementIn(t, ctx, "Invoices reach their customers", r.ID)
	return ctx, cwm, req, r, in
}

// incrementIn creates an increment shipping in a release.
func incrementIn(t *testing.T, ctx context.Context, title, rel string) api.Increment {
	t.Helper()
	in := increment(t, ctx, title)
	if _, err := increments(t, ctx).ShipIn(ctx, api.ShipInInput{ID: in.ID, In: rel}); err != nil {
		t.Fatal(err)
	}
	return in
}

// switchFeature is a feature tagged display-switch, aimed at the increment.
func switchFeature(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, inc string) string {
	t.Helper()
	f := want(t, ctx, cwm, "feature", "The new invoice screens wait behind their switch")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: f, Capability: api.CapDisplaySwitch}); err != nil {
		t.Fatal(err)
	}
	aim(t, ctx, f, inc)
	return f
}

func aim(t *testing.T, ctx context.Context, f, inc string) {
	t.Helper()
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: inc}); err != nil {
		t.Fatal(err)
	}
}

// confirmDone has a person confirm features done: the last one asks the
// increment's delivery; it waits until the delivery review is ready.
func confirmDone(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, inc string, features ...string) {
	t.Helper()
	for _, f := range features {
		if err := move(ctx, cwm, f, "realized", session); err != nil {
			t.Fatal(err)
		}
		if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
			t.Fatal(err)
		}
	}
	eventually(t, "the delivery review planned", func() (string, bool) {
		got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: inc})
		if err != nil {
			return err.Error(), false
		}
		for _, td := range got.Todos {
			// Ready: the approval's own planning (no intake) has replaced
			// the plan its filing made.
			if td.Packet.Step == "delivery-review" && td.State == schedapi.StateReady {
				return "", true
			}
		}
		return "no ready delivery-review step", false
	})
}

// (1) The increment-accept recipe plans the agent's delivery review before
// the person's approve-delivery: the approval is not ready until the review
// has reported.
func TestTheIncrementAcceptRecipeReviewsTheDeliveryBeforeItsApproval(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	workIn(t, ctx, req, in.ID, "Totals are rounded wrong")
	review, approval := stepOf(t, ctx, in.ID, "delivery-review"), stepOf(t, ctx, in.ID, "approve-delivery")
	if review.For != schedapi.ForAgent || review.Packet.Template != "increment-accept" || review.Packet.Interaction != api.InteractionValidate {
		t.Fatalf("the delivery review: %+v; want an agent's validation (increment-accept)", review)
	}
	if approval.For != schedapi.ForPerson || approval.State == schedapi.StateReady || !slices.Contains(approval.After, review.ID) {
		t.Fatalf("the approval before the review reported: %s after %v; want the person's, waiting on %s", approval.State, approval.After, review.ID)
	}
	reviewDelivery(t, ctx, in.ID, nil)
	if a := stepOf(t, ctx, in.ID, "approve-delivery"); a.State != schedapi.StateReady {
		t.Fatalf("the approval once the review passed: %s; want ready", a.State)
	}
}

// (1) A failed delivery review names each thing missing - a screen feature
// whose GUI test has not passed, a display switch left with no follow-up, no
// demo path, a follow-up in an increment of another release, a finding in the
// reviewer's words - each filed as a fix inside the increment; the approval
// ends and is asked again once they have ended.
func TestAFailedDeliveryReviewNamesWhatIsMissing(t *testing.T) {
	t.Parallel()
	ctx, cwm, req, _, in := reviewWorld(t)
	later := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.5.0"})
	elsewhere := incrementIn(t, ctx, "Reminders", later.ID)
	other := file(t, ctx, req, api.FileInput{Title: "Mail the reminders", Module: "billing", Kind: api.KindFix, Increment: elsewhere.ID})
	sw := switchFeature(t, ctx, cwm, in.ID)
	screen := want(t, ctx, cwm, "feature", "The invoice pane shows its lines", cwmapi.ObjectEdge{Kind: "realizes", To: "app/ktw-gui/frontend/src/sections/InvoiceLines.tsx"})
	aim(t, ctx, screen, in.ID)
	confirmDone(t, ctx, cwm, in.ID, sw, screen)
	first := stepOf(t, ctx, in.ID, "delivery-review")

	reportReview(t, ctx, in.ID, schedapi.Result{Summary: "not usable yet", Verdict: api.VerdictRejected, More: map[string]any{"review": map[string]any{
		"findings": []map[string]any{{"what": "The old invoice printer is still in the code"}, {"what": "Reminders are not mailed", "follow_up": other.ID}},
	}}})
	list, err := req.List(ctx, api.ListInput{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var fixes []string
	var ids []string
	for _, r := range list.Requests {
		if r.Kind == api.KindFix && r.ID != other.ID {
			fixes = append(fixes, r.Title)
			ids = append(ids, r.ID)
		}
	}
	all := strings.Join(fixes, "\n")
	for _, missing := range []string{sw, screen + " changes a screen", "no demo path", other.ID + " is in " + elsewhere.ID, "The old invoice printer is still in the code"} {
		if !strings.Contains(all, missing) {
			t.Errorf("the fixes filed in %s do not name %q:\n%s", in.ID, missing, all)
		}
	}
	got, err := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rv := got.Increment.DeliveryReview; rv == nil || rv.Verdict != api.VerdictRejected {
		t.Fatalf("the failed review is not recorded on %s: %+v", in.ID, rv)
	}
	if got.Increment.State != api.IncrementOpen {
		t.Fatalf("its delivery review failed: %s; want it open", got.Increment.State)
	}
	if open, _ := schedOf(t, ctx).List(ctx, schedapi.ListInput{Request: in.ID}); len(open.Todos) > 0 {
		t.Fatalf("its delivery review failed, %d to-do(s) of the approval left: %+v", len(open.Todos), open.Todos[0])
	}
	for _, id := range ids {
		workFix(t, ctx, id)
	}
	again := stepOf(t, ctx, in.ID, "delivery-review")
	if again.ID == first.ID {
		t.Fatal("the delivery review was not asked again once its fixes ended")
	}
}

// (2) An increment holding a display switch with no follow-up planned in an
// increment of the same release is refused at done by usable-delivered,
// naming the switch - and so is a review approving it.
func TestAnIncrementWithASwitchAndNoFollowUpIsRefusedAtDone(t *testing.T) {
	t.Parallel()
	ctx, cwm, _, _, in := reviewWorld(t)
	sw := switchFeature(t, ctx, cwm, in.ID)
	confirmDone(t, ctx, cwm, in.ID, sw)
	s := schedOf(t, ctx)
	id := stepOf(t, ctx, in.ID, "delivery-review").ID
	if got, err := s.Pull(ctx, schedapi.PullInput{ID: id, By: session}); err != nil || got.Todo == nil {
		t.Fatalf("pull the delivery review %s: %v %+v", id, err, got)
	}
	_, err := s.Report(ctx, schedapi.ReportInput{ID: id, Result: schedapi.Result{Status: schedapi.StatusSuccess, Summary: "usable",
		Verdict: api.VerdictApproved, By: session, More: map[string]any{"review": map[string]any{"demo": demoPath}}}})
	if err == nil || !strings.Contains(err.Error(), sw) {
		t.Errorf("a review approving %s with its switch left: %v; want it refused naming %s", in.ID, err, sw)
	}
	// Not collected: the review is reported again, the approval still waits.
	if st := stepOf(t, ctx, in.ID, "approve-delivery").State; st == schedapi.StateReady {
		t.Errorf("the approval after a refused review: %s; want it waiting", st)
	}
	_, err = increments(t, ctx).Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Note: "looks fine"})
	if err == nil || !strings.Contains(err.Error(), "usable-delivered") || !strings.Contains(err.Error(), sw) {
		t.Fatalf("done with its switch left and no follow-up: %v; want usable-delivered refusing it, naming %s", err, sw)
	}
}

// (2)(3) A test delivery: an increment whose switch is turned on by an
// increment it names, open and planned in the same release, passes its
// review and is accepted at done; naming an increment of another release or
// a dropped one is refused.
func TestAnIncrementWithItsSwitchFollowUpPlannedIsAcceptedAtDone(t *testing.T) {
	t.Parallel()
	ctx, cwm, _, rel, in := reviewWorld(t)
	inc := increments(t, ctx)
	later := plan(t, ctx, releases(t, ctx), api.PlanInput{Module: "billing", Version: "1.5.0"})
	elsewhere := incrementIn(t, ctx, "Screens for everyone, later", later.ID)
	dropped := incrementIn(t, ctx, "Screens for everyone, dropped", rel.ID)
	if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: dropped.ID, By: person, Note: "not this way"}); err != nil {
		t.Fatal(err)
	}
	turnsOn := incrementIn(t, ctx, "Screens for everyone", rel.ID)
	for _, bad := range []string{elsewhere.ID, dropped.ID} {
		if _, err := inc.SetTestDelivery(ctx, api.SetTestDeliveryInput{ID: in.ID, TurnedOnBy: bad, By: person}); err == nil {
			t.Errorf("a test delivery turned on by %s accepted; want it refused", bad)
		}
	}
	if _, err := inc.SetTestDelivery(ctx, api.SetTestDeliveryInput{ID: in.ID, TurnedOnBy: turnsOn.ID, By: person}); err != nil {
		t.Fatalf("a test delivery turned on by %s of the same release: %v", turnsOn.ID, err)
	}
	sw := switchFeature(t, ctx, cwm, in.ID)
	confirmDone(t, ctx, cwm, in.ID, sw)
	if out := reviewDelivery(t, ctx, in.ID, nil); !strings.HasPrefix(out.Note, "collected") {
		t.Fatalf("the review of a test delivery: %q; want it collected", out.Note)
	}
	if got, err := inc.CheckUsableDelivered(ctx, api.UsableDeliveredInput{ID: in.ID}); err != nil || !got.Accept {
		t.Fatalf("usable-delivered on a test delivery reviewed: %+v %v; want it accepted", got, err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, in.ID, "approve-delivery").ID, person, schedapi.Result{Summary: "delivered", Verdict: api.VerdictApproved})
	if got, _ := inc.Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementDone {
		t.Fatalf("its delivery approved: %s; want done", got.Increment.State)
	}
}

// (2) A person forces an increment's done past usable-delivered with a note:
// accepted and recorded as forced with the note; an agent, or a person with
// no note, is refused.
func TestAPersonForcesUsableDeliveredWithANote(t *testing.T) {
	t.Parallel()
	ctx, cwm, _, _, in := reviewWorld(t)
	sw := switchFeature(t, ctx, cwm, in.ID)
	confirmDone(t, ctx, cwm, in.ID, sw)
	inc := increments(t, ctx)
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: session, Force: true, Note: "the agent insists"}); err == nil {
		t.Fatal("an agent forced usable-delivered")
	}
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true}); err == nil {
		t.Fatal("a person forced usable-delivered with no note")
	}
	const why = "delivered for the test team only: IN-2 turns the screens on"
	if _, err := inc.Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: why}); err != nil {
		t.Fatalf("a person forcing it with a note: %v", err)
	}
	j, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "the forced done in the journal", func() (string, bool) {
		got, err := j.Transitions(ctx, journalapi.TransitionsInput{ID: in.ID})
		if err != nil {
			return err.Error(), false
		}
		for _, e := range got.Entries {
			if e.To == api.IncrementDone && e.Forced && e.Note == why && e.By == person {
				return "", true
			}
		}
		return fmt.Sprintf("%+v", got.Entries), false
	})
}

// (4) Once the delivery review has passed, the person's approve-delivery
// to-do and the increment's pane show the review's result and its demo path.
func TestTheDeliveryApprovalShowsTheReviewAndTheDemoPath(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	workIn(t, ctx, req, in.ID, "Totals are rounded wrong")
	reviewDelivery(t, ctx, in.ID, nil)
	a := stepOf(t, ctx, in.ID, "approve-delivery")
	if a.Packet.Params["demo"] != demoPath || !strings.Contains(a.Packet.Params["review"].(string), "passed") {
		t.Fatalf("the approval's params: %+v; want the review passed and its demo path", a.Packet.Params)
	}
	for lang, words := range map[string][]string{"en": {"Its delivery review", "Passed - by " + session.ID, "Demo: " + demoPath},
		"fr": {"Sa revue de livraison", "Passée - par " + session.ID, "Démo : " + demoPath}} {
		def, err := increments(t, ctx).PanePage(ctx, api.IncrementPaneInput{ID: in.ID, Tab: "page", Language: lang})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(def)
		for _, w := range words {
			if !strings.Contains(string(raw), w) {
				t.Errorf("the increment's page (%s) does not show %q", lang, w)
			}
		}
	}
}
