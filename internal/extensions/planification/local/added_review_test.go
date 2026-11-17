// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REVIEW ADDED AFTER THE ANALYSIS IS PREPARATION — a challenge (the
// reviewer's) and an approval (a person's validation) added after a request's
// analysis, with no phase given, run in its preparation: they read the module,
// write nothing — pulled while another holds the module for reading — and run
// while the request they depend on is still open, once its preparation is done.
func TestChallengeAndApproveAddedAfterTheAnalysisRunWhileTheDependencyIsOpen(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	first := file(t, ctx, req, api.FileInput{Title: "Invoice model", Module: "billing", Kind: api.KindBuild})
	then := file(t, ctx, req, api.FileInput{Title: "Invoice PDF", Module: "billing", Kind: api.KindBuild, DependsOn: []string{first.ID}})
	pullAndReport(t, ctx, stepOf(t, ctx, then.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	w := workOf(t, ctx)
	analyse, implement := stepOf(t, ctx, then.ID, "analyse"), stepOf(t, ctx, then.ID, "implement")
	challenge, err := w.Add(ctx, api.AddWorkInput{Request: then.ID, Step: "challenge", After: []string{analyse.ID}, Before: []string{implement.ID},
		Doer: api.DoerPersona, Task: "reviewer", Instructions: "Challenge the analysis.", By: person})
	if err != nil {
		t.Fatal(err)
	}
	c := challenge.Todo
	if c.Packet.Phase != api.PhasePreparation || len(c.Writes) != 0 || !slices.Equal(c.Reads, []string{"billing"}) {
		t.Fatalf("the challenge: phase %s, reads %v, writes %v; want preparation, reading billing only", c.Packet.Phase, c.Reads, c.Writes)
	}
	approve, err := w.Add(ctx, api.AddWorkInput{Request: then.ID, Step: "approve", After: []string{c.ID}, Before: []string{implement.ID},
		Doer: api.DoerPerson, Interaction: api.InteractionValidate, Instructions: "Approve the analysis.", By: person})
	if err != nil {
		t.Fatal(err)
	}
	if approve.Todo.Packet.Phase != api.PhasePreparation || len(approve.Todo.Writes) != 0 {
		t.Fatalf("the approval: phase %s, writes %v; want preparation, no write", approve.Todo.Packet.Phase, approve.Todo.Writes)
	}

	// The dependency's preparation done, then the analysis: the challenge is
	// ready while the dependency is still open.
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, first.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, analyse.ID, session, schedapi.Result{Summary: "analysed"})
	if c := stepOf(t, ctx, then.ID, "challenge"); c.State != schedapi.StateReady {
		t.Fatalf("the challenge while %s is open: %s %q; want ready", first.ID, c.State, c.Waits)
	}

	// Another request's analysis reads billing meanwhile: the challenge, a
	// reader too, is still pulled.
	other := file(t, ctx, req, api.FileInput{Title: "Invoice numbering", Module: "billing", Kind: api.KindBuild})
	pullAndReport(t, ctx, stepOf(t, ctx, other.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	if _, err := schedOf(t, ctx).Pull(ctx, schedapi.PullInput{ID: stepOf(t, ctx, other.ID, "analyse").ID, By: another}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, c.ID, session, schedapi.Result{Summary: "the analysis holds"})
	if a := stepOf(t, ctx, then.ID, "approve"); a.State != schedapi.StateReady {
		t.Fatalf("the approval once challenged, %s still open: %s %q; want ready", first.ID, a.State, a.Waits)
	}
	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: approve.Todo.ID, Result: schedapi.Result{Status: schedapi.StatusSuccess,
		Summary: "approved", Verdict: api.VerdictApproved, By: person}}); err != nil {
		t.Fatal(err)
	}
	if im := stepOf(t, ctx, then.ID, "implement"); im.State != schedapi.StateWaiting || !strings.Contains(im.Waits, first.ID) {
		t.Fatalf("the build: %s %q; want waiting on %s", im.State, im.Waits, first.ID)
	}
}

// A step added with the build phase given still writes its module and waits
// for the request it depends on to end.
func TestAnAddedBuildStepStillWaits(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	first := file(t, ctx, req, api.FileInput{Title: "Invoice model", Module: "billing", Kind: api.KindBuild})
	then := file(t, ctx, req, api.FileInput{Title: "Invoice PDF", Module: "billing", Kind: api.KindBuild, DependsOn: []string{first.ID}})
	pullAndReport(t, ctx, stepOf(t, ctx, then.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "clear"})
	analyse, implement := stepOf(t, ctx, then.ID, "analyse"), stepOf(t, ctx, then.ID, "implement")
	got, err := workOf(t, ctx).Add(ctx, api.AddWorkInput{Request: then.ID, Step: "challenge", After: []string{analyse.ID}, Before: []string{implement.ID},
		Doer: api.DoerPersona, Task: "reviewer", Instructions: "Challenge the analysis.", Phase: api.PhaseBuild, By: person})
	if err != nil {
		t.Fatal(err)
	}
	c := got.Todo
	if c.Packet.Phase != api.PhaseBuild || !slices.Equal(c.Writes, []string{"billing"}) {
		t.Fatalf("a build step added: phase %s, writes %v; want build, writing billing", c.Packet.Phase, c.Writes)
	}
	for _, step := range []string{api.StepIntake, "analyse"} {
		pullAndReport(t, ctx, stepOf(t, ctx, first.ID, step).ID, session, schedapi.Result{Summary: step + " done"})
	}
	pullAndReport(t, ctx, analyse.ID, session, schedapi.Result{Summary: "analysed"})
	if c := stepOf(t, ctx, then.ID, "challenge"); c.State != schedapi.StateWaiting || !strings.Contains(c.Waits, first.ID) {
		t.Fatalf("a build step added while %s is open: %s %q; want waiting on it", first.ID, c.State, c.Waits)
	}
}
