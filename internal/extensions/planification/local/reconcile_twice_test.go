// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// THE RECONCILER IS LEVEL-TRIGGERED - it reads the
// stored state, never an event: a second sweep over the state the first one
// repaired repairs nothing and puts no to-do; a restart changes nothing it
// does.

// allTodos is every open to-do of the store, by id.
func allTodos(t *testing.T, ctx context.Context) []string {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(got.Todos))
	for _, td := range got.Todos {
		ids = append(ids, td.ID+"@"+string(td.State))
	}
	slices.Sort(ids)
	return ids
}

// One broken state per repair, all switched on: the first applied sweep
// repairs each; the second, over what the first left, repairs nothing and
// puts no to-do.
func TestASecondSweepOverTheSameStateRepairsNothing(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.Repairs...))

	// end-plan: a done use case whose plan is still ready, no step open.
	uc := want(t, ctx, cwm, "use-case", "Mail the invoices")
	if _, err := req.File(ctx, api.FileInput{On: uc, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	removeAll(t, ctx, uc)
	if err := move(ctx, cwm, uc, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	journaledDone(t, ctx, uc)

	// put-step: a ready plan whose to-dos were lost.
	lost := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	removeAll(t, ctx, lost.ID)

	// apply-answer: an idle request whose question was answered unseen.
	asked := file(t, ctx, req, api.FileInput{Title: "Choose the numbering", Module: "billing"})
	if _, err := req.Ask(ctx, api.AskInput{ID: asked.ID, Question: "Which numbering?", By: intake}); err != nil {
		t.Fatal(err)
	}
	inbox := inboxOf(t, ctx)
	e, err := inbox.File(ctx, inboxapi.FileInput{Title: "Which numbering?", Expects: inboxapi.ExpectsText})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inbox.Answer(ctx, inboxapi.AnswerInput{ID: e.Request.ID, Note: "yearly"}); err != nil {
		t.Fatal(err)
	}
	answerFollowed(t, ctx, cwm, e.Request.ID)
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: asked.ID, To: e.Request.ID, Kind: api.EdgeAskedIn}); err != nil {
		t.Fatal(err)
	}

	// clear-wait: a step waiting on a request that was dropped.
	waiting := file(t, ctx, req, api.FileInput{Title: "Print the numbers", Module: "billing"})
	dropped := file(t, ctx, req, api.FileInput{Title: "Number the credit notes", Module: "billing"})
	waitingIntake := stepOf(t, ctx, waiting.ID, api.StepIntake).ID
	reportBlocked(t, ctx, waitingIntake, "it needs the numbering", &schedapi.HeldOn{Kind: schedapi.HeldObject, Target: dropped.ID, Reason: "the numbering first"})
	if _, err := plansOf(t, ctx).Finish(ctx, api.FinishInput{ID: dropped.ID, Outcome: api.PlanDropped, By: person, Note: "not needed"}); err != nil {
		t.Fatal(err)
	}

	// labels: a to-do carrying a label its work no longer has.
	stale := file(t, ctx, req, api.FileInput{Title: "Sign the invoices", Module: "billing"})

	// The module's reactions to these writes run on its observer: a sweep
	// that reads a request before its reaction landed is refused (it moved
	// since the sweep read it) - wait until each last write is followed, and
	// what those reactions wrote in turn, before the stale label is planted:
	// any of them settles every to-do (the drop's above all), and a settle
	// that lands after the plant recomputes the label the first sweep must
	// find out of date.
	for _, id := range []string{lost.ID, asked.ID, waiting.ID, dropped.ID, stale.ID} {
		answerFollowed(t, ctx, cwm, id)
	}
	planlocal.Settle(ctx, eventuallyWithin)

	s := schedOf(t, ctx)
	intakeOf := stepOf(t, ctx, stale.ID, api.StepIntake).ID
	got, err := s.Get(ctx, schedapi.GetInput{ID: intakeOf})
	if err != nil {
		t.Fatal(err)
	}
	td := got.Todo
	td.Labels = []string{"increment:billing.IN-099"}
	if _, err := s.Put(ctx, schedapi.PutInput{Todo: td}); err != nil {
		t.Fatal(err)
	}
	// Nothing moves the planted label but a settle: still stored once the
	// observer is idle, or a reaction ran late (the race is then in the code).
	planlocal.Settle(ctx, eventuallyWithin)
	if got, err := s.Get(ctx, schedapi.GetInput{ID: intakeOf}); err != nil {
		t.Fatal(err)
	} else if !slices.Equal(got.Todo.Labels, td.Labels) {
		t.Fatalf("the planted label of %s was settled away before the first sweep: %v", intakeOf, got.Todo.Labels)
	}

	broken := map[string]string{
		api.RepairEndPlan: uc, api.RepairPutStep: lost.ID, api.RepairApplyAnswer: asked.ID,
		api.RepairClearWait: waiting.ID, api.RepairLabels: stale.ID,
	}
	first := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	for repair, work := range broken {
		if r := repairOn(t, first, repair, work); !r.Applied {
			t.Fatalf("the first sweep did not apply %s on %s: %s", repair, work, r.Skipped)
		}
	}
	// What the repairs started (a resumed plan, a released step) settles
	// before the state is read again.
	planlocal.Settle(ctx, 5*time.Second)

	before := allTodos(t, ctx)
	second := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	planlocal.Settle(ctx, 5*time.Second)
	for _, r := range second.Repairs {
		if r.Applied {
			t.Errorf("the second sweep repaired %s on %s again: %s", r.Repair, orWork(r), r.Reason)
		}
	}
	for repair, work := range broken {
		noRepairOn(t, second, repair, work)
	}
	if after := allTodos(t, ctx); !slices.Equal(before, after) {
		t.Errorf("the second sweep changed the to-dos:\nbefore %v\nafter  %v", before, after)
	}
}

func orWork(r api.Repair) string {
	if r.Todo != "" {
		return r.Todo
	}
	return r.Work
}

// A restart the same day keeps the day's limit: the reconciler a previous
// process switched to dry-run - its count and why kept in the incidents'
// list - is still in dry-run in a fresh process.
func TestARestartKeepsTheDaysLimit(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.RepairPutStep))
	r := file(t, ctx, req, api.FileInput{Title: "Number the invoices", Module: "billing"})
	removeAll(t, ctx, r.ID)

	// What the previous process left: past the day's limit today.
	why := "the day's limit of put to-dos is passed (21 > 20): the reconciler is back to dry-run until tomorrow"
	list := filepath.Join(application.EnvOf(projectRootOf(t, ctx, cwm, r.ID)).HomeDir(), "planification", "incidents.json")
	if err := os.MkdirAll(filepath.Dir(list), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := `{"incidents": [], "day": "` + time.Now().UTC().Format("2006-01-02") + `", "puts": 21, "stopped": "` + why + `"}`
	if err := os.WriteFile(list, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	out := reconcile(t, ctx, api.ReconcileInput{Apply: true})
	if !out.DryRun || !strings.Contains(out.Why, "day's limit") {
		t.Fatalf("after the restart: dry-run %v (%q); want dry-run for the day's limit", out.DryRun, out.Why)
	}
	if rep := repairOn(t, out, api.RepairPutStep, r.ID); rep.Applied {
		t.Errorf("after the restart a step was put: %+v", rep)
	}
	if n := len(openTodos(t, ctx, r.ID)); n != 0 {
		t.Errorf("after the restart %d to-do(s) put; want none", n)
	}
}
