// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REJECTION WHOSE VALIDATION LOST ITS AFTER EDGES STILL SENDS WORK BACK
// : a review left with no after (a plan undone, a step
// put back by hand) and rejected redoes the request's last round of agent
// build work - the implementation, then its auto-review and module update -
// and the review is asked again after them, never alone on unchanged work.
// A rejection with no agent build work done at all is refused, saying why.
func TestARejectionWithNoAfterRedoesTheLastBuildSteps(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round invoices", Module: "billing", Kind: api.KindBuild})
	for _, step := range []string{api.StepIntake, "analyse", "implement", "auto-review", "module-update"} {
		pullAndReport(t, ctx, stepOf(t, ctx, r.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	review := cutAfter(t, ctx, stepOf(t, ctx, r.ID, "review").ID)
	pullAndReport(t, ctx, review, person, schedapi.Result{Summary: "the rounding is wrong", Verdict: api.VerdictRejected})

	got := steps(t, ctx, r.ID)
	for _, gone := range []string{"intake (rework)", "analyse (rework)"} {
		if slices.Contains(got, gone) {
			t.Errorf("%s is redone: steps %v; want only the last round of build work", gone, got)
		}
	}
	impl := stepOf(t, ctx, r.ID, "implement (rework)")
	for _, check := range []string{"auto-review (rework)", "module-update (rework)"} {
		if c := stepOf(t, ctx, r.ID, check); !slices.Contains(c.After, impl.ID) {
			t.Errorf("%s comes after %v; want after the implementation done again", check, c.After)
		}
	}
	again := stepOf(t, ctx, r.ID, "review")
	for _, s := range []string{"implement (rework)", "auto-review (rework)", "module-update (rework)"} {
		if !slices.Contains(again.After, stepOf(t, ctx, r.ID, s).ID) {
			t.Errorf("the review asked again comes after %v; want after %s", again.After, s)
		}
	}
	if again.State == schedapi.StateReady {
		t.Errorf("the review asked again is ready at once: it must wait on the work done again")
	}

	// Nothing built yet: the rejection is refused, and the review stays to answer.
	bare := file(t, ctx, req, api.FileInput{Title: "Round credit notes", Module: "billing", Kind: api.KindBuild})
	early := cutAfter(t, ctx, stepOf(t, ctx, bare.ID, "review").ID)
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: early, By: person}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Report(ctx, schedapi.ReportInput{ID: early, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "not like this", Verdict: api.VerdictRejected, By: person}})
	if err == nil || !strings.Contains(err.Error(), "sends back no work") {
		t.Fatalf("a rejection with no build work done: %v; want refused, saying it sends back no work", err)
	}
	if got := steps(t, ctx, bare.ID); slices.ContainsFunc(got, func(s string) bool { return strings.HasSuffix(s, "(rework)") }) {
		t.Errorf("a refused rejection planned work again: %v", got)
	}
}

// cutAfter drops a to-do's after edges in the scheduler, as a plan undone
// leaves them, and answers its id.
func cutAfter(t *testing.T, ctx context.Context, id string) string {
	t.Helper()
	s := schedOf(t, ctx)
	got, err := s.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	td := got.Todo
	td.After, td.Waits, td.State = nil, "", schedapi.StateReady
	if _, err := s.Put(ctx, schedapi.PutInput{Todo: td}); err != nil {
		t.Fatal(err)
	}
	return id
}

// WHAT A BLOCKED OR FAILED WORKER FOUND ON THE WAY IS FILED
// : the follow-ups of a report that did not succeed are
// filed as a success's are, within the cap of what agents add on their own
// - counted, so a later report past the cap waits on a
// person's validate step that stands aside: the step itself waits on nothing
// more.
func TestABlockedReportFilesItsFollowUps(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Round orders", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, ctx, stepOf(t, ctx, r.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "intake done"})
	analyse := stepOf(t, ctx, r.ID, "analyse")
	before := slices.Clone(analyse.After)
	pullAndReport(t, ctx, analyse.ID, session, schedapi.Result{Status: schedapi.StatusBlocked, Summary: "the rounding rule is unknown",
		More: map[string]any{"follow_ups": []map[string]any{{"title": "Ask the bank for its rounding rule", "module": "billing"}}}})
	if !hasRequest(t, ctx, req, "Ask the bank for its rounding rule") {
		t.Fatalf("a blocked report's follow-up was not filed")
	}

	// Failed after a retry, with the cap's rest and one more: the rest is
	// filed, the one past the cap waits on a person's validate step.
	if _, err := workOf(t, ctx).Retry(ctx, api.RetryWorkInput{ID: analyse.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	var fus []map[string]any
	var titles []string
	for i := 1; i <= followUpsPastCap-1; i++ {
		title := fmt.Sprintf("Round the order line %d", i)
		titles = append(titles, title)
		fus = append(fus, map[string]any{"title": title, "module": "billing"})
	}
	pullAndReport(t, ctx, analyse.ID, session, schedapi.Result{Status: schedapi.StatusFail, Summary: "the build broke",
		More: map[string]any{"follow_ups": fus}})
	for _, title := range titles[:len(titles)-1] {
		if !hasRequest(t, ctx, req, title) {
			t.Errorf("a failed report's follow-up %q under the cap was not filed", title)
		}
	}
	if hasRequest(t, ctx, req, titles[len(titles)-1]) {
		t.Errorf("a follow-up past the cap was filed: the blocked report's follow-up was not counted")
	}
	ask := stepOf(t, ctx, r.ID, "validate-adds")
	if !strings.Contains(ask.Packet.Instructions, titles[len(titles)-1]) {
		t.Errorf("the validate step does not list the follow-up past the cap: %q", ask.Packet.Instructions)
	}
	if got := stepOf(t, ctx, r.ID, "analyse"); !slices.Equal(got.After, before) {
		t.Errorf("the failed step comes after %v; want %v - a validate step of follow-ups stands aside", got.After, before)
	}
}
