// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// EVERY WAIT ENDS - « À faire » holds only what waits
// on the person: a wait that can never end by itself is resolved by the
// reconciler, and a block that names no cause stays with who reported it
// until its deadline, is retried once, and only then is the person's.

// unblockOf says whether « À faire » carries an unblock item for a request.
func unblockOf(q api.PersonQueueOutput, request string) bool {
	for _, it := range q.Items {
		if it.Request == request && it.Action == api.ActionUnblock {
			return true
		}
	}
	return false
}

// decisionsAbout are the decisions opened about a request.
func decisionsAbout(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, request string) []cwmapi.Object {
	t.Helper()
	got, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "billing", Type: decisionsapi.DecisionType})
	if err != nil {
		t.Fatal(err)
	}
	var out []cwmapi.Object
	for _, o := range got.Objects {
		if o.EdgeTo("about") == request {
			out = append(out, o)
		}
	}
	return out
}

// A reconcile resolves every wait that can never end: a blocked report
// waiting on a superseded request is re-pointed to its open successor, and
// cleared once that one has ended; a step waiting on its increment's
// dependency is cleared once its own increment has ended; a wait no rule
// resolves (an object that cannot be read) opens ONE prepared decision about
// the request - never a second on the next sweep. None of them is an unblock
// item of « À faire »; the decision is a decide item there.
func TestAWaitThatCanNeverEndIsResolvedByTheReconciler(t *testing.T) {
	t.Parallel()
	t.Run("a wait on superseded work follows its successor", func(t *testing.T) {
		ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairRepointWait, api.RepairClearWait))
		a := file(t, ctx, req, api.FileInput{Title: "Print the numbers", Module: "billing"})
		b := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
		c := file(t, ctx, req, api.FileInput{Title: "Number the invoices yearly", Module: "billing"})
		intakeA := stepOf(t, ctx, a.ID, api.StepIntake).ID
		reportBlocked(t, ctx, intakeA, "it needs the numbering", &schedapi.HeldOn{Kind: schedapi.HeldObject, Target: b.ID, Reason: "the numbering first"})
		if _, err := req.Supersede(ctx, api.SupersedeInput{ID: b.ID, With: c.ID, Note: "yearly covers it", By: person}); err != nil {
			t.Fatal(err)
		}
		if unblockOf(personQueue(t, ctx, ""), a.ID) {
			t.Error("a step waiting on superseded work is an unblock item of « À faire »; its successor holds it")
		}
		if rep := repairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairRepointWait, a.ID); !strings.Contains(rep.Reason, c.ID) {
			t.Errorf("the repair says %q; want it to name the successor %s", rep.Reason, c.ID)
		}
		noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairClearWait, a.ID)
		got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairRepointWait, a.ID)
		if !got.Applied {
			t.Fatalf("not applied: %s", got.Skipped)
		}
		held, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: intakeA})
		if err != nil {
			t.Fatal(err)
		}
		if held.Todo.State != schedapi.StateWaiting || held.Todo.Held == nil || held.Todo.Held.Target != c.ID {
			t.Fatalf("after the sweep: %s on %+v; want it still waiting, on %s", held.Todo.State, held.Todo.Held, c.ID)
		}
		noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairRepointWait, a.ID)
		// Its successor ends: the wait is cleared as any wait on ended work.
		if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: c.ID, Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
			t.Fatal(err)
		}
		if rep := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairClearWait, a.ID); !rep.Applied {
			t.Fatalf("not applied: %s", rep.Skipped)
		}
		if after, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: intakeA}); err != nil || after.Todo.State != schedapi.StateReady {
			t.Errorf("its step is %s (%q); want ready", after.Todo.State, after.Todo.Waits)
		}
	})

	t.Run("a step of an ended increment waits no more", func(t *testing.T) {
		ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.RepairClearWait))
		inc := increments(t, ctx)
		create := func(title string) string {
			t.Helper()
			out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: []string{title}, By: person})
			if err != nil {
				t.Fatal(err)
			}
			return out.Increment.ID
		}
		b, c := create("B first"), create("C after B")
		if _, err := req.Depend(ctx, api.DependInput{ID: c, On: b}); err != nil {
			t.Fatal(err)
		}
		fix := heldFix(t, ctx, cwm, req, c, "A fix C holds")
		if fix.State != schedapi.StateWaiting || fix.Waits != "its increment "+c+" waits on "+b {
			t.Fatalf("the fix C holds: %s (%q); want it waiting on %s", fix.State, fix.Waits, b)
		}
		if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: c, By: person}); err != nil {
			t.Fatal(err)
		}
		// The broken state: its step still waits on B, its increment ended
		// (C still depends on B, and B is open) - the settle that follows the
		// drop is lost; once any observer is through, the wait is put back.
		for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
			if stepOf(t, ctx, fix.Request, fix.Packet.Step).Waits != fix.Waits {
				break
			}
		}
		time.Sleep(200 * time.Millisecond)
		s := schedOf(t, ctx)
		cur, err := s.Get(ctx, schedapi.GetInput{ID: fix.ID})
		if err != nil {
			t.Fatal(err)
		}
		stale := cur.Todo
		stale.State, stale.Waits = schedapi.StateWaiting, fix.Waits
		if _, err := s.Put(ctx, schedapi.PutInput{Todo: stale}); err != nil {
			t.Fatal(err)
		}
		got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairClearWait, fix.Request)
		if !got.Applied || !strings.Contains(got.Reason, "its increment "+c+" has ended") {
			t.Fatalf("the repair: %q, applied %v (%s); want its increment's end named and the wait cleared", got.Reason, got.Applied, got.Skipped)
		}
		if td := stepOf(t, ctx, fix.Request, fix.Packet.Step); td.Waits == fix.Waits {
			t.Errorf("after the sweep it still waits %q", td.Waits)
		}
	})

	t.Run("a step after its own request's earlier step is no cycle", func(t *testing.T) {
		ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.RepairClearWait, api.RepairDecideWait))
		r := file(t, ctx, req, api.FileInput{Title: "Check the invoices", Module: "billing"})
		w := workOf(t, ctx)
		add := func(step string, after ...string) string {
			t.Helper()
			out, err := w.Add(ctx, api.AddWorkInput{Request: r.ID, Step: step, Task: "coder", Instructions: step + " it.", After: after, By: session})
			if err != nil {
				t.Fatal(err)
			}
			return out.Todo.ID
		}
		// The verify waits after an ended step and the auto-review still
		// open, both its own request's: ordinary sequencing.
		build, review := add("build-it"), add("review-it")
		verify := add("verify-it", build, review)
		pullAndReport(t, ctx, build, session, schedapi.Result{Summary: "built"})
		if v := stepOf(t, ctx, r.ID, "verify-it"); v.State != schedapi.StateWaiting || v.Waits != "after "+review {
			t.Fatalf("the verify: %s (%q); want it waiting after %s", v.State, v.Waits, review)
		}
		for _, b := range check(t, ctx, api.PlanningCheckInput{Census: true}).Blockers {
			if b.Work == r.ID && b.Kind == api.BlockerIncident {
				t.Errorf("the check reads %s (%s) as an incident: %s", b.Todo, b.Step, b.Reason)
			}
		}
		out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
		noRepairOn(t, out, api.RepairDecideWait, r.ID)
		for _, i := range out.Incidents {
			if i.Work == r.ID {
				t.Errorf("the sweep raises an incident on %s: %s", i.Todo, i.Reason)
			}
		}
		if n := len(decisionsAbout(t, ctx, cwm, r.ID)); n != 0 {
			t.Errorf("%d decisions opened about %s; want none", n, r.ID)
		}
		if unblockOf(personQueue(t, ctx, ""), r.ID) {
			t.Errorf("the verify %s waiting on its own auto-review is an unblock item of « À faire »", verify)
		}
	})

	t.Run("a wait no rule resolves is one prepared decision", func(t *testing.T) {
		ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.RepairClearWait, api.RepairDecideWait))
		r := file(t, ctx, req, api.FileInput{Title: "Archive the invoices", Module: "billing"})
		gone := "billing.RQ-999"
		reportBlocked(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, "it needs the archive", &schedapi.HeldOn{Kind: schedapi.HeldObject, Target: gone, Reason: "the archive first"})
		if !unblockOf(personQueue(t, ctx, ""), r.ID) {
			t.Fatal("before the sweep: no unblock item for a wait on an object that cannot be read; the test reads nothing")
		}
		dry := repairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairDecideWait, r.ID)
		if dry.Applied || len(decisionsAbout(t, ctx, cwm, r.ID)) != 0 {
			t.Fatalf("the dry run opened a decision: %+v", dry)
		}
		if got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairDecideWait, r.ID); !got.Applied {
			t.Fatalf("not applied: %s", got.Skipped)
		}
		ods := decisionsAbout(t, ctx, cwm, r.ID)
		if len(ods) != 1 {
			t.Fatalf("%d decisions about %s; want one", len(ods), r.ID)
		}
		od := ods[0]
		out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
		noRepairOn(t, out, api.RepairDecideWait, r.ID)
		if n := len(decisionsAbout(t, ctx, cwm, r.ID)); n != 1 {
			t.Errorf("after a second sweep: %d decisions about %s; want still one", n, r.ID)
		}
		q := personQueue(t, ctx, "")
		if unblockOf(q, r.ID) {
			t.Error("the wait is still an unblock item of « À faire »; its decision is the person's card")
		}
		decide := false
		for _, it := range q.Items {
			if it.Object == od.ID && it.Action == api.ActionDecide {
				decide = it.Proposal != "" && len(it.Options) >= 2
			}
		}
		if !decide {
			t.Errorf("« À faire » holds no prepared decide item for %s: %+v", od.ID, itemsOf(q, od.ID))
		}
		// Settled, the decision gives the step's own card back: the person
		// acts on the choice; no second decision is opened.
		resolve(t, ctx, od.ID, "Remove the wait")
		noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairDecideWait, r.ID)
		if !unblockOf(personQueue(t, ctx, ""), r.ID) {
			t.Error("the decision settled: no unblock item for the step to act on the choice")
		}
	})
}

// ageReport moves a stopped to-do's report back by age, as if it had been
// blocked that long. The planner collects a report on its own loop and
// writes the to-do back with the report's own time: it settles first, so
// that write never lands after the aged one and undoes it (:
// under load the incident read as not yet due).
func ageReport(t *testing.T, ctx context.Context, id string, age time.Duration) {
	t.Helper()
	planlocal.Settle(ctx, 5*time.Second)
	s := schedOf(t, ctx)
	cur, err := s.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil || cur.Todo.Held == nil && cur.Todo.Result == nil {
		t.Fatalf("get %s: %v %+v", id, err, cur.Todo)
	}
	old := cur.Todo
	at := time.Now().Add(-age).UTC().Format(time.RFC3339Nano)
	if old.Result != nil {
		res := *old.Result
		res.At = at
		old.Result = &res
	}
	if old.Held != nil {
		h := *old.Held
		h.Since = at
		old.Held = &h
	}
	if _, err := s.Put(ctx, schedapi.PutInput{Todo: old}); err != nil {
		t.Fatal(err)
	}
	planlocal.Settle(ctx, 5*time.Second)
	if got, err := s.Get(ctx, schedapi.GetInput{ID: id}); err != nil ||
		got.Todo.Result != nil && got.Todo.Result.At != at || got.Todo.Held != nil && got.Todo.Held.Since != at {
		t.Fatalf("aging %s did not hold: %v %+v %+v", id, err, got.Todo.Result, got.Todo.Held)
	}
}

// A blocked report that names no cause stays off « À faire » until its
// deadline (reconciler.unclassified_hours, 24 by default): it is the
// reporter's, its blocker says when it is due. Past it, the reconciler sends
// the step back to the queue once; a second unnamed block of the same step is
// the person's at once, and is never retried again.
func TestABlockWithNoCauseIsRetriedOncePastItsDeadline(t *testing.T) {
	t.Parallel()
	ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairRetryUnnamed))
	r := file(t, ctx, req, api.FileInput{Title: "Archive the invoices", Module: "billing"})
	intake := stepOf(t, ctx, r.ID, api.StepIntake).ID
	reportBlocked(t, ctx, intake, "something is off", nil)

	b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), r.ID)
	if b.Kind != api.BlockerIncident || b.Due == "" {
		t.Fatalf("a fresh unnamed block: %s due %q; want an unclassified incident with its deadline", b.Kind, b.Due)
	}
	if due, err := time.Parse(time.RFC3339, b.Due); err != nil || time.Until(due) < 23*time.Hour || time.Until(due) > 25*time.Hour {
		t.Errorf("due %q; want 24 hours from its report", b.Due)
	}
	if unblockOf(personQueue(t, ctx, ""), r.ID) {
		t.Error("a fresh unnamed block is an unblock item of « À faire »; it is the reporter's until its deadline")
	}
	noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairRetryUnnamed, r.ID)

	// Past its deadline: the person's, and the reconciler retries it once.
	ageReport(t, ctx, intake, 25*time.Hour)
	if b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), r.ID); b.Due != "" {
		t.Errorf("past its deadline it is still due %q", b.Due)
	}
	if !unblockOf(personQueue(t, ctx, ""), r.ID) {
		t.Error("past its deadline, not retried: no unblock item for the person")
	}
	got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairRetryUnnamed, r.ID)
	if !got.Applied {
		t.Fatalf("not applied: %s", got.Skipped)
	}
	td := stepOf(t, ctx, r.ID, api.StepIntake)
	if td.State != schedapi.StateReady || td.Packet.UnnamedRetries != 1 {
		t.Fatalf("after the retry: %s (%q), retries %d; want ready, its retry recorded", td.State, td.Waits, td.Packet.UnnamedRetries)
	}

	// Blocked again with no cause: the person's at once, never retried again.
	reportBlocked(t, ctx, intake, "something is still off", nil)
	if b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), r.ID); b.Due != "" {
		t.Errorf("a second unnamed block is due %q; want it the person's at once", b.Due)
	}
	if !unblockOf(personQueue(t, ctx, ""), r.ID) {
		t.Error("a second unnamed block of the same step: no unblock item for the person")
	}
	ageReport(t, ctx, intake, 25*time.Hour)
	noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairRetryUnnamed, r.ID)
}
