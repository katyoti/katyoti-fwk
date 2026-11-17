// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN AGENT REVIEWER SENDS WORK BACK AT MOST 2 ROUNDS PER REQUEST
// : two agent rejections are reworked as before; the
// third plans no rework - the plan is blocked with the reason, shown to the
// person on « À faire » - and resumed by the person, the round is planned as
// theirs: one more.
func TestAThirdAgentRejectionBlocksThePlan(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	for round := 1; round <= 2; round++ {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "review").ID, another, schedapi.Result{Summary: "the rounding is still wrong", Verdict: api.VerdictRejected})
		redoRound(t, ctx, r.ID, round)
	}
	if st := planState(t, ctx, r.ID); st == api.PlanBlocked {
		t.Fatalf("blocked after two agent rounds; want them reworked")
	}

	review := stepOf(t, ctx, r.ID, "review")
	pullAndReport(t, ctx, review.ID, another, schedapi.Result{Summary: "the rounding is wrong a third time", Verdict: api.VerdictRejected})
	if s := steps(t, ctx, r.ID); strings.Contains(strings.Join(s, ","), "(rework)") {
		t.Errorf("a third agent rejection planned its rework: %v", s)
	}
	got, err := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.State != api.PlanBlocked || !strings.Contains(got.Plan.Note, "past the cap of 2 rounds") || !strings.Contains(got.Plan.Note, "the rounding is wrong a third time") {
		t.Fatalf("the plan after a third agent rejection: %s %q; want blocked with the reason", got.Plan.State, got.Plan.Note)
	}
	if p := stepOf(t, ctx, r.ID, "review"); p.ID != review.ID || p.State == schedapi.StateReady {
		t.Errorf("the rejecting review: %s %s %q; want it parked, not ready", p.ID, p.State, p.Waits)
	}
	if !queueAsksToResume(t, ctx, r.ID, "past the cap of 2 rounds") {
		t.Errorf("« À faire » does not show the blocked plan with its reason")
	}

	// Resumed by the person: one more round, theirs.
	if _, err := plansOf(t, ctx).Plan(ctx, api.PlanNextInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if im := stepOf(t, ctx, r.ID, "implement"+strings.Repeat(" (rework)", 3)); im.Packet.AddedBy == nil || *im.Packet.AddedBy != person {
		t.Errorf("the rework resumed: added by %v; want the person", im.Packet.AddedBy)
	}
	if st := planState(t, ctx, r.ID); st != api.PlanReady {
		t.Errorf("resumed, the plan is %s; want ready", st)
	}
}

// A PARKED SEND-BACK KEEPS THE FOLLOW-UPS ITS REPORT FILES PAST THE ADD CAP
// : the third agent rejection files follow-ups past
// the cap of 8; they are not filed while the round is parked - the parked step
// holds them - and resumed by the person, every one of them is filed.
func TestAParkedSendBackKeepsItsFollowUpsPastTheAddCap(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round orders", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	for round := 1; round <= 2; round++ {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "review").ID, another, schedapi.Result{Summary: "the rounding is still wrong", Verdict: api.VerdictRejected})
		redoRound(t, ctx, r.ID, round)
	}
	// Nine follow-ups: more than the whole cap, so some are past it whatever
	// the rounds already added.
	var fus []map[string]any
	var titles []string
	for i := 1; i <= followUpsPastCap; i++ {
		title := fmt.Sprintf("Round the order line %d", i)
		titles = append(titles, title)
		fus = append(fus, map[string]any{"title": title, "module": "billing"})
	}
	review := stepOf(t, ctx, r.ID, "review")
	pullAndReport(t, ctx, review.ID, another, schedapi.Result{Summary: "the rounding is wrong a third time", Verdict: api.VerdictRejected,
		More: map[string]any{"follow_ups": fus}})
	if st := planState(t, ctx, r.ID); st != api.PlanBlocked {
		t.Fatalf("the third agent rejection left the plan %s; want blocked", st)
	}
	parkedStep := stepOf(t, ctx, r.ID, "review")
	if held, _ := parkedStep.Packet.Params["follow_ups"].([]any); len(held) == 0 {
		t.Fatalf("the parked step holds no follow-up past the add cap: params %v", parkedStep.Packet.Params)
	}
	if hasRequest(t, ctx, req, titles[len(titles)-1]) {
		t.Errorf("a follow-up past the add cap was filed while the round is parked")
	}

	// Resumed by the person: every follow-up is filed.
	if _, err := plansOf(t, ctx).Plan(ctx, api.PlanNextInput{ID: r.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, title := range titles {
		if !hasRequest(t, ctx, req, title) {
			t.Errorf("resumed, the follow-up %q was not filed", title)
		}
	}
}

// followUpsPastCap is one more follow-up than the whole add cap.
const followUpsPastCap = 9

// A PERSON'S REJECTED VALIDATION IS NEVER CAPPED: after two agent rounds, the
// person rejecting still gets the work done again.
func TestAPersonsRejectionIsReworkedAfterTwoAgentRounds(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round credit notes", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	for round := 1; round <= 2; round++ {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, "review").ID, another, schedapi.Result{Summary: "still wrong", Verdict: api.VerdictRejected})
		redoRound(t, ctx, r.ID, round)
	}
	review := stepOf(t, ctx, r.ID, "review")
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: review.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "the totals are off by a cent", Verdict: api.VerdictRejected, By: person}}); err != nil {
		t.Fatal(err)
	}
	if st := planState(t, ctx, r.ID); st == api.PlanBlocked {
		t.Fatalf("a person's rejection after two agent rounds blocked the plan")
	}
	if im := stepOf(t, ctx, r.ID, "implement"+strings.Repeat(" (rework)", 3)); im.Packet.AddedBy == nil || *im.Packet.AddedBy != person {
		t.Errorf("the person's rework: added by %v; want the person", im.Packet.AddedBy)
	}
}

// A THIRD AGENT REOPEN OF A REALIZED FEATURE'S VERIFY BLOCKS: two verify
// rounds reopening it end as before; the third keeps its round open and
// blocked, so no fourth round is filed while the person has not decided.
func TestAThirdAgentReopenOfAFeaturesVerifyBlocks(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	realize := func() {
		t.Helper()
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built again"}); err != nil {
			t.Fatal(err)
		}
	}
	round := func(n int) {
		t.Helper()
		realize()
		file(t, ctx, req, api.FileInput{On: f, Kind: api.KindReview, Title: "Verify the realized feature"})
		pullAndReport(t, ctx, stepOf(t, ctx, f, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
		v := stepOf(t, ctx, f, "verify")
		if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: v.ID, By: another}); err != nil {
			t.Fatal(err)
		}
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "proposed", Transition: "reopen", By: another, Note: "the PDF has no logo"}); err != nil {
			t.Fatal(err)
		}
		if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: v.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
			Summary: "reopened: the PDF has no logo, round " + string(rune('0'+n)), By: another}}); err != nil {
			t.Fatal(err)
		}
	}
	round(1)
	round(2)
	if st := planState(t, ctx, f); st == api.PlanBlocked {
		t.Fatalf("blocked after two agent reopens; want their rounds ended")
	}
	round(3)
	got, err := plansOf(t, ctx).Get(ctx, api.GetPlanInput{ID: f})
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan.State != api.PlanBlocked || !strings.Contains(got.Plan.Note, "reopened the feature in its verify round past the cap of 2 rounds") {
		t.Fatalf("the verify round after a third agent reopen: %s %q; want blocked with the reason", got.Plan.State, got.Plan.Note)
	}
	if !queueAsksToResume(t, ctx, f, "past the cap of 2 rounds") {
		t.Errorf("« À faire » does not show the blocked verify round with its reason")
	}
}

// redoRound does the work a rejected review sent back in its round: each
// step done again carries one more " (rework)" per round.
func redoRound(t *testing.T, ctx context.Context, request string, round int) {
	t.Helper()
	for _, step := range []string{"implement", "auto-review", "module-update"} {
		name := step + strings.Repeat(" (rework)", round)
		pullAndReport(t, ctx, stepOf(t, ctx, request, name).ID, session, schedapi.Result{Summary: name + " done"})
	}
}

// queueAsksToResume says « À faire » shows a request's blocked plan with a
// reason containing `why`, to resume.
func queueAsksToResume(t *testing.T, ctx context.Context, id, why string) bool {
	t.Helper()
	for _, it := range itemsOf(personQueue(t, ctx, ""), id) {
		if it.Action == api.ActionUnblock && it.Resume && strings.Contains(it.Asks, why) {
			return true
		}
	}
	return false
}
