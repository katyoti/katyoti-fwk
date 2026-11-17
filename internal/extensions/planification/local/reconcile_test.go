// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE RECONCILER - per repair, a broken state built on a
// temp universe is reported in dry-run and repaired once switched on; a state
// with no safe repair raises one incident; an element repaired too often
// raises « flapping ».

// reconcilerOn is a Planification.yaml switching repairs on.
func reconcilerOn(repairs ...string) map[string]string {
	return map[string]string{"Planification.yaml": "reconciler:\n  repairs: [" + strings.Join(repairs, ", ") + "]\n"}
}

func reconcile(t *testing.T, ctx context.Context, in api.ReconcileInput) api.ReconcileOutput {
	t.Helper()
	pl, err := application.UseService[api.PlanningService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := pl.Reconcile(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// repairOn is the one repair of a kind the sweep found on a piece of work.
func repairOn(t *testing.T, out api.ReconcileOutput, repair, work string) api.Repair {
	t.Helper()
	var got []api.Repair
	for _, r := range out.Repairs {
		if r.Repair == repair && r.Work == work {
			got = append(got, r)
		}
	}
	if len(got) != 1 {
		t.Fatalf("the sweep found %d %s repair(s) on %s: %+v; want one (incidents: %+v)", len(got), repair, work, out.Repairs, out.Incidents)
	}
	return got[0]
}

func noRepairOn(t *testing.T, out api.ReconcileOutput, repair, work string) {
	t.Helper()
	for _, r := range out.Repairs {
		if r.Repair == repair && r.Work == work {
			t.Errorf("the sweep still finds %s on %s: %+v", repair, work, r)
		}
	}
}

// removeAll archives a request's open to-dos past planification (a crash, a
// hand edit), its plan left as it is.
func removeAll(t *testing.T, ctx context.Context, request string) {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range got.Todos {
		if _, err := schedOf(t, ctx).Archive(ctx, schedapi.ArchiveInput{ID: td.ID, Note: "lost"}); err != nil {
			t.Fatal(err)
		}
	}
}

// journaledDone waits for the journal to record an object's move to done: it
// is written behind the move, and the end-plan repair reads when it happened.
func journaledDone(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	deadline := time.Now().Add(eventuallyWithin)
	for !journalHasDone(ctx, id) {
		if time.Now().After(deadline) {
			t.Fatalf("the journal never recorded %s's move to done", id)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func journalHasDone(ctx context.Context, id string) bool {
	j, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		return false
	}
	got, err := j.Transitions(ctx, journalapi.TransitionsInput{ID: id})
	if err != nil {
		return false
	}
	for _, e := range got.Entries {
		if e.To == api.StateDone {
			return true
		}
	}
	return false
}

func openTodos(t *testing.T, ctx context.Context, request string) []api.WorkTodo {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	return got.Todos
}

// A done lifecycle with an open plan: reported in dry-run, its plan ended once
// end-plan is on; not applied while the repair is off or the kill switch on.
func TestTheReconcilerEndsThePlanOfADoneObject(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		config  map[string]string
		applied bool
		why     string
	}{
		{"switched on", reconcilerOn(api.RepairEndPlan), true, ""},
		{"switched off", reconcilerOn(api.RepairLabels), false, "not switched on"},
		{"kill switch", map[string]string{"Planification.yaml": "reconciler:\n  off: true\n  repairs: [end-plan]\n"}, false, "dry-run"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cwm, req := worldConfigured(t, nil, tc.config)
			uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
			if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindBuild, By: person}); err != nil {
				t.Fatal(err)
			}
			removeAll(t, ctx, uc)
			if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
				t.Fatal(err)
			}
			journaledDone(t, ctx, uc)
			if got := planState(t, ctx, uc); got != api.PlanReady {
				t.Fatalf("the broken state: plan %s; want ready", got)
			}
			dry := reconcile(t, ctx, api.ReconcileInput{})
			if r := repairOn(t, dry, api.RepairEndPlan, uc); r.Applied || !dry.DryRun {
				t.Errorf("the dry-run applied %+v", r)
			}
			if got := planState(t, ctx, uc); got != api.PlanReady {
				t.Fatalf("the dry-run moved the plan to %s", got)
			}
			out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
			r := repairOn(t, out, api.RepairEndPlan, uc)
			if r.Applied != tc.applied || !strings.Contains(r.Skipped, tc.why) {
				t.Fatalf("applied %v (skipped %q); want applied %v (%q)", r.Applied, r.Skipped, tc.applied, tc.why)
			}
			want := api.PlanReady
			if tc.applied {
				want = api.PlanDone
			}
			if got := planState(t, ctx, uc); got != want {
				t.Errorf("its plan is %s; want %s", got, want)
			}
			if tc.applied {
				noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairEndPlan, uc)
			}
		})
	}
}

// A done lifecycle whose request still has open steps is no safe end: one
// incident, never a repair.
func TestADoneObjectWithOpenStepsIsAnIncident(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.Repairs...))
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	noRepairOn(t, out, api.RepairEndPlan, uc)
	n := 0
	for _, i := range out.Incidents {
		if i.Work == uc && i.Check == api.RepairEndPlan {
			n++
			if i.Key != api.RepairEndPlan+":"+uc || !strings.Contains(i.Reason, "still open") {
				t.Errorf("the incident: %+v", i)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d end-plan incident(s) on %s: %+v; want one", n, uc, out.Incidents)
	}
	if got := planState(t, ctx, uc); planEndedState(got) {
		t.Errorf("its plan was ended (%s) with steps open", got)
	}
}

// A done lifecycle whose only open steps prove it (phase verify) - a person
// accepted it before its proof ran - is no incident, neither for the
// reconciler nor for the census: the plan ends by
// itself when the proof reports.
func TestADoneObjectWaitingOnlyOnItsProofIsNoIncident(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.Repairs...))
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindReview, By: person}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, uc, api.StepIntake).ID, session, schedapi.Result{Summary: "a review"})
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	noRepairOn(t, out, api.RepairEndPlan, uc)
	for _, i := range out.Incidents {
		if i.Work == uc {
			t.Errorf("an incident on %s while only its proof is open: %+v", uc, i)
		}
	}
	for _, b := range check(t, ctx, api.PlanningCheckInput{Census: true}).Blockers {
		if b.Work == uc && strings.Contains(b.Reason, "lifecycle is done") {
			t.Errorf("the census reads %s as an incident: %+v", uc, b)
		}
	}
	pullAndReport(t, ctx, stepOf(t, ctx, uc, "review").ID, session, schedapi.Result{Summary: "no finding"})
	if got := planState(t, ctx, uc); got != api.PlanDone {
		t.Errorf("its proof reported, its plan is %s; want done", got)
	}
}

// A done object whose plan could not finish only because its live GUI walk
// is not verified yet - the walk listed too by the batch request that runs
// the release's GUI tests together - waits on that batch: no incident, for
// the census or the reconciler. With no batch
// listing the walk it stays an incident.
func TestADoneObjectWaitingOnItsGUIBatchIsNoIncident(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.Repairs...))
	walk := "bash app/ktw-gui/frontend/tests/live/invoice-mail.sh"
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindReview, By: person,
		Deliverables: []api.Deliverable{
			{Title: "The invoice is mailed, by element ids", Check: walk},
			{Title: "The mailed invoice reads well", ByHand: "in the release's GUI batch, run `" + walk + "` and read its PASS line"},
		}}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, uc, api.StepIntake).ID, session, schedapi.Result{Summary: "a review"})
	pullAndReport(t, ctx, stepOf(t, ctx, uc, "review").ID, session, schedapi.Result{Summary: "no finding"})
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	if got := planState(t, ctx, uc); got != api.PlanBlocked {
		t.Fatalf("the case needs its plan blocked by the unverified walk; it is %s", got)
	}
	incident := func() bool {
		for _, b := range check(t, ctx, api.PlanningCheckInput{Census: true}).Blockers {
			if b.Work == uc && b.Kind == api.BlockerIncident {
				return true
			}
		}
		return false
	}
	if !incident() {
		t.Fatalf("no batch lists its walk: the census should read %s as an incident", uc)
	}

	batch := file(t, ctx, req, api.FileInput{Title: "Run the release's GUI tests together", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "The GUI walks of the release", Check: "bash app/ktw-gui/frontend/tests/live/invoice-list.sh && " + walk}}})
	if incident() {
		t.Errorf("its walk waits in %s: the census still reads %s as an incident", batch.ID, uc)
	}
	waits := false
	for _, b := range check(t, ctx, api.PlanningCheckInput{Census: true}).Blockers {
		if b.Work == uc {
			waits = waits || b.On == batch.ID && strings.Contains(b.Reason, batch.ID)
		}
	}
	if !waits {
		t.Errorf("the census does not name %s's wait on %s", uc, batch.ID)
	}
	out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	noRepairOn(t, out, api.RepairEndPlan, uc)
	for _, i := range out.Incidents {
		if i.Work == uc {
			t.Errorf("an incident on %s while its walk waits in %s: %+v", uc, batch.ID, i)
		}
	}
}

// A verify reported blocked with no named kind because it could not run its
// live GUI walk - the walk listed not yet verified by the batch request that
// runs the release's GUI tests together - waits on that batch: the census
// names the wait, what waits after the request waits on the batch too, and
// the reconciler never retries it. A walk no batch
// lists stays an unclassified wait.
func TestAVerifyBlockedOnItsGUIWalkWaitsOnTheBatch(t *testing.T) {
	t.Parallel()
	ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairRetryUnnamed))
	r := file(t, ctx, req, api.FileInput{Title: "Fix the invoice mail", Module: "billing", Kind: api.KindFix, By: person})
	for _, step := range []string{api.StepIntake, "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	verify := stepOf(t, ctx, r.ID, "verify")
	if verify.For != schedapi.ForAgent || verify.Packet.Phase != api.PhaseVerify {
		t.Fatalf("the case needs an agent's verify step; it is %s, phase %s", verify.For, verify.Packet.Phase)
	}
	reportBlocked(t, ctx, verify.ID, "AC-1 passes; the GUI walk tests/live/invoice-mail.sh is not run - it counts by the release's batch verdict", nil)
	c := file(t, ctx, req, api.FileInput{Title: "Mail the credit notes too", Module: "billing", Kind: api.KindFix, By: person})
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: r.ID}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, c.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	other := file(t, ctx, req, api.FileInput{Title: "Fix the invoice print", Module: "billing", Kind: api.KindFix, By: person})
	for _, step := range []string{api.StepIntake, "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, other.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	reportBlocked(t, ctx, stepOf(t, ctx, other.ID, "verify").ID, "the walk invoice-print.sh is not run", nil)

	unclassified := func(b api.Blocker) bool {
		return b.Kind == api.BlockerIncident && strings.Contains(b.Reason, "unclassified")
	}
	if b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), r.ID); !unclassified(b) {
		t.Fatalf("no batch lists its walk: %s %q; want an unclassified wait", b.Kind, b.Reason)
	}

	walk := "bash app/ktw-gui/frontend/tests/live/invoice-mail.sh"
	batch := file(t, ctx, req, api.FileInput{Title: "Run the release's GUI tests together", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "The GUI walks of the release", Check: "bash app/ktw-gui/frontend/tests/live/invoice-list.sh && " + walk}}})
	out := check(t, ctx, api.PlanningCheckInput{Census: true})
	if b := blockerOf(t, out, r.ID); b.On != batch.ID || b.Kind == api.BlockerIncident || !strings.Contains(b.Reason, batch.ID) {
		t.Errorf("its walk waits in %s: the census reads %s as %s on %q (%q)", batch.ID, r.ID, b.Kind, b.On, b.Reason)
	}
	if b := blockerOf(t, out, c.ID); b.Kind == api.BlockerIncident || !slices.Contains(b.Chain, batch.ID) && b.On != batch.ID {
		t.Errorf("%s waits after %s, which waits on %s: %s on %q, chain %v (%q)", c.ID, r.ID, batch.ID, b.Kind, b.On, b.Chain, b.Reason)
	}
	if b := blockerOf(t, out, other.ID); !unclassified(b) {
		t.Errorf("no batch lists invoice-print.sh: %s %q; want an unclassified wait", b.Kind, b.Reason)
	}

	ageReport(t, ctx, verify.ID, 25*time.Hour)
	noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairRetryUnnamed, r.ID)
	if td := stepOf(t, ctx, r.ID, "verify"); td.Packet.UnnamedRetries != 0 {
		t.Errorf("the verify waiting on %s was retried: %s, retries %d", batch.ID, td.State, td.Packet.UnnamedRetries)
	}
}

// A done object whose plan is ready with nothing to do is one inconsistency,
// said once - its lifecycle done while its plan is open, which the
// reconciler's end-plan reads - never also « ready with no step to do », whose
// put-step the reconciler skips on a done object.
func TestADoneObjectWithAReadyPlanAndNoStepIsOneIncident(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn())
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	removeAll(t, ctx, uc)
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	var on []api.Blocker
	for _, b := range check(t, ctx, api.PlanningCheckInput{Census: true}).Blockers {
		if b.Work == uc {
			on = append(on, b)
		}
	}
	if len(on) != 1 || on[0].Kind != api.BlockerIncident || !strings.Contains(on[0].Reason, "lifecycle is done") {
		t.Errorf("the census reads %s as %+v; want one incident, its lifecycle done", uc, on)
	}
	noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairPutStep, uc)
}

// A wait on a split request - it has no step of its own, its parts carry its
// work - follows its first open part: no incident « split with no step to do »
func TestAWaitOnASplitRequestFollowsItsParts(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	big := file(t, ctx, req, api.FileInput{Title: "Mail the invoices and the reminders", Module: "billing", Kind: api.KindBuild})
	c := file(t, ctx, req, api.FileInput{Title: "Report the mailings", Module: "billing", Kind: api.KindBuild})
	if _, err := req.Depend(ctx, api.DependInput{ID: c.ID, On: big.ID}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, c.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a build"})
	split, err := req.Split(ctx, api.SplitInput{ID: big.ID, By: intake, Note: "two", Parts: []api.SplitPart{
		{Module: "billing", Title: "Mail the invoices"}, {Module: "billing", Title: "Mail the reminders"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(split.Parts) != 2 {
		t.Fatalf("%d parts; want 2", len(split.Parts))
	}
	first := split.Parts[0].ID
	if split.Parts[1].ID < first {
		first = split.Parts[1].ID
	}
	b := blockerOf(t, check(t, ctx, api.PlanningCheckInput{Census: true}), c.ID)
	if b.Kind == api.BlockerIncident || b.On != big.ID || !slices.Contains(b.Chain, first) {
		t.Errorf("%s waits on the split %s: %s on %q, chain %v (%q); want a wait reaching its part %s", c.ID, big.ID, b.Kind, b.On, b.Chain, b.Reason, first)
	}
}

// A ready plan with no to-do and no step run gets its template's steps; an
// element repaired more than three times is « flapping » and left alone.
func TestTheReconcilerPutsTheStepsOfAReadyPlanAndStopsWhenItFlaps(t *testing.T) {
	t.Parallel()
	ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairPutStep))
	r := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	for round := 1; round <= 4; round++ {
		removeAll(t, ctx, r.ID)
		if got := openTodos(t, ctx, r.ID); len(got) != 0 {
			t.Fatalf("round %d: %d to-dos left", round, len(got))
		}
		dry := reconcile(t, ctx, api.ReconcileInput{})
		repairOn(t, dry, api.RepairPutStep, r.ID)
		out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
		got := repairOn(t, out, api.RepairPutStep, r.ID)
		if round <= 3 {
			if !got.Applied {
				t.Fatalf("round %d: not applied: %s", round, got.Skipped)
			}
			if n := len(openTodos(t, ctx, r.ID)); n == 0 {
				t.Fatalf("round %d: no to-do put", round)
			}
			continue
		}
		if got.Applied || !strings.HasPrefix(got.Skipped, "flapping") {
			t.Errorf("round %d: %+v; want it left alone, flapping", round, got)
		}
		flapping := false
		for _, i := range out.Incidents {
			flapping = flapping || i.Check == api.IncidentFlapping && i.Work == r.ID
		}
		if !flapping {
			t.Errorf("no flapping incident: %+v", out.Incidents)
		}
		if n := len(openTodos(t, ctx, r.ID)); n != 0 {
			t.Errorf("a flapping element was repaired again: %d to-dos", n)
		}
	}
}

// An answer given in the Inbox that its idle request did not take up is
// applied: the request resumes with it.
func TestTheReconcilerAppliesAnAnswerNotTakenUp(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.RepairApplyAnswer))
	r := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	if _, err := req.Ask(ctx, api.AskInput{ID: r.ID, Question: "Which numbering?", By: intake}); err != nil {
		t.Fatal(err)
	}
	if got := planState(t, ctx, r.ID); got != api.PlanIdle {
		t.Fatalf("asked, the plan is %s; want idle", got)
	}
	// The answer was given while nothing listened: an entry answered before
	// the request was linked to it.
	inbox := inboxOf(t, ctx)
	e, err := inbox.File(ctx, inboxapi.FileInput{Title: "Which numbering?", Expects: inboxapi.ExpectsText})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: e.Request.ID, Note: "yearly"}); err != nil {
		t.Fatal(err)
	}
	answerFollowed(t, ctx, cwm, e.Request.ID)
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: r.ID, To: e.Request.ID, Kind: api.EdgeAskedIn}); err != nil {
		t.Fatal(err)
	}
	dry := reconcile(t, ctx, api.ReconcileInput{})
	repairOn(t, dry, api.RepairApplyAnswer, r.ID)
	if got := planState(t, ctx, r.ID); got != api.PlanIdle {
		t.Fatalf("the dry-run resumed it (%s)", got)
	}
	got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairApplyAnswer, r.ID)
	if !got.Applied {
		t.Fatalf("not applied: %s", got.Skipped)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanReady {
		t.Errorf("answered, the plan is %s; want ready", st)
	}
	noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairApplyAnswer, r.ID)
}

// A step blocked on an object that has ended is put back in the queue.
func TestTheReconcilerClearsAWaitOnSomethingEnded(t *testing.T) {
	t.Parallel()
	ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairClearWait))
	a := file(t, ctx, req, api.FileInput{Title: "Print the numbers", Module: "billing"})
	b := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	intakeA := stepOf(t, ctx, a.ID, api.StepIntake).ID
	reportBlocked(t, ctx, intakeA, "it needs the numbering", &schedapi.HeldOn{Kind: schedapi.HeldObject, Target: b.ID, Reason: "the numbering first"})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: b.ID, Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
		t.Fatal(err)
	}
	held, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: intakeA})
	if err != nil || held.Todo.State != schedapi.StateWaiting {
		t.Fatalf("the broken state: %+v %v; want its step still waiting", held.Todo, err)
	}
	repairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairClearWait, a.ID)
	got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairClearWait, a.ID)
	if !got.Applied {
		t.Fatalf("not applied: %s", got.Skipped)
	}
	after, err := schedOf(t, ctx).Get(ctx, schedapi.GetInput{ID: intakeA})
	if err != nil || after.Todo.State != schedapi.StateReady {
		t.Errorf("its step is %s (%q); want ready", after.Todo.State, after.Todo.Waits)
	}
}

// A to-do whose labels are out of date gets them recomputed.
func TestTheReconcilerRecomputesStaleLabels(t *testing.T) {
	t.Parallel()
	ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairLabels))
	r := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	id := stepOf(t, ctx, r.ID, api.StepIntake).ID
	s := schedOf(t, ctx)
	got, err := s.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	stale := got.Todo
	stale.Labels = []string{"increment:billing.IN-099"}
	if _, err := s.Put(ctx, schedapi.PutInput{Todo: stale}); err != nil {
		t.Fatal(err)
	}
	repairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairLabels, r.ID)
	if rep := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairLabels, r.ID); !rep.Applied {
		t.Fatalf("not applied: %s", rep.Skipped)
	}
	after, err := s.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(after.Todo.Labels, "increment:billing.IN-099") {
		t.Errorf("its labels are still %v", after.Todo.Labels)
	}
}

// A repair applies only to what is as the sweep read it, and an unknown
// repair is refused.
func TestTheReconcilerRefusesAnUnknownRepair(t *testing.T) {
	t.Parallel()
	ctx, _, _ := world(t)
	pl, _ := application.UseService[api.PlanningService](ctx)
	if _, err := pl.Reconcile(ctx, api.ReconcileInput{Repairs: []string{"undo-everything"}}); err == nil {
		t.Error("an unknown repair: accepted")
	}
}

// Above twenty to-dos put in a day the reconciler switches itself to dry-run.
func TestTheReconcilerGoesBackToDryRunPastTheDaysPuts(t *testing.T) {
	t.Parallel()
	ctx, _, req := worldConfigured(t, nil, reconcilerOn(api.RepairPutStep))
	var ids []string
	for i := 0; i < 8; i++ {
		r := file(t, ctx, req, api.FileInput{Title: "Number the invoices " + string(rune('A'+i)), Module: "billing"})
		removeAll(t, ctx, r.ID)
		ids = append(ids, r.ID)
	}
	out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	applied, skipped := 0, 0
	for _, id := range ids {
		r := repairOn(t, out, api.RepairPutStep, id)
		switch {
		case r.Applied:
			applied++
		case strings.Contains(r.Skipped, "day's limit"):
			skipped++
		default:
			t.Errorf("%s: %+v", id, r)
		}
	}
	if applied == 0 || skipped == 0 {
		t.Fatalf("%d applied, %d skipped for the day's limit; want both", applied, skipped)
	}
	next := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	if !next.DryRun || !strings.Contains(next.Why, "day's limit") {
		t.Errorf("the next sweep: dry-run %v (%q); want dry-run for the day's limit", next.DryRun, next.Why)
	}
}
