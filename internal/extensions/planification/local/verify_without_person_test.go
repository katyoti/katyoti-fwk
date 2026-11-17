// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	journalapi "github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	verificationapi "github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// ONLY WHAT IS THE PERSON'S WAITS ON THEM - a realized
// feature whose acceptance criteria all pass and which no by-hand deliverable
// built is verified without the person, the passing criteria named in its
// move; one built with a by-hand deliverable still waits on the person's
// confirmation in « À faire ».
func TestARealizedFeatureWhoseCriteriaPassIsVerifiedWithoutThePerson(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := criterionWorld(t)
	built := want(t, ctx, cwm, "feature", "Print the invoices")
	byHand := want(t, ctx, cwm, "feature", "Mail the invoices")
	legacy := want(t, ctx, cwm, "feature", "Archive the invoices")
	stuck := want(t, ctx, cwm, "feature", "Sign the invoices")
	application.PutService[verificationapi.VerificationService](application.AppFromContext(ctx), "default",
		functionVerification{status: map[string]string{built: "pass", byHand: "pass", legacy: "pass"}})
	crit := verdictCriterion(t, ctx, cwm, built, "The invoices print", "pass", "printed")

	// A build on a feature not yet active ends realized: its criteria pass,
	// nothing is tested by hand - verified at once.
	if _, err := req.File(ctx, api.FileInput{On: built, Kind: api.KindBuild, By: person}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{api.StepIntake, "implement", "module-update", "verify"} {
		td := stepOf(t, ctx, built, step)
		by, r := session, schedapi.Result{Summary: step + " done"}
		if td.For == schedapi.ForPerson {
			by, r.Verdict = person, api.VerdictApproved
		}
		pullAndReport(t, ctx, td.ID, by, r)
	}
	if st := stateOf(t, ctx, cwm, built); st != "done" {
		t.Fatalf("the built feature whose criteria pass: %q; want done without the person", st)
	}
	eventually(t, "its move's note naming it verified without the person and "+crit, func() (string, bool) {
		note := doneNote(t, ctx, built)
		return note, strings.Contains(note, "verified without the person") && strings.Contains(note, crit)
	})

	// Built by a request with a deliverable a person tests by hand: it
	// stays realized, and « À faire » asks the person to confirm it.
	record := func(id string) {
		t.Helper()
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "realized", By: session, Note: "already in the code"}); err != nil {
			t.Fatal(err)
		}
	}
	record(byHand)
	r := file(t, ctx, req, api.FileInput{Title: "Mail them", Module: "billing", Kind: api.KindBuild,
		Deliverables: []api.Deliverable{{Title: "the mail reads well", ByHand: "open the mail; only a person judges it"}}})
	if _, err := req.Assess(ctx, api.AssessInput{ID: r.ID, Serves: byHand, By: intake}); err != nil {
		t.Fatal(err)
	}
	verdictCriterion(t, ctx, cwm, byHand, "The mail is sent", "pass", "sent")
	// A realized feature recorded by hand, its criterion's verdict recorded
	// later: verified once the verdict is in.
	record(legacy)
	verdictCriterion(t, ctx, cwm, legacy, "The invoices are archived", "pass", "archived")
	// Its criterion passes but its verification is not declared: the gate
	// refuses the agent's verify, and the refusal plans what meets it - agent
	// work, not the person's.
	record(stuck)
	verdictCriterion(t, ctx, cwm, stuck, "The invoices are signed", "pass", "signed")

	eventually(t, "the legacy feature verified on its verdict", func() (string, bool) {
		st := stateOf(t, ctx, cwm, legacy)
		return st, st == "done"
	})
	if st := stateOf(t, ctx, cwm, byHand); st != "realized" {
		t.Errorf("the feature built with a by-hand deliverable: %q; want still realized, for the person", st)
	}
	var asked []string
	for _, it := range personQueue(t, ctx, "").Items {
		if it.Action == api.ActionValidate {
			asked = append(asked, it.Object)
		}
	}
	if !hasItem(asked, byHand) {
		t.Errorf("« À faire » does not ask to confirm %s (by hand): %v", byHand, asked)
	}
	eventually(t, "the refused verify of "+stuck+" planning what meets its gate", func() (string, bool) {
		got, err := req.List(ctx, api.ListInput{Module: "billing"})
		if err != nil {
			return err.Error(), false
		}
		var titles []string
		for _, r := range got.Requests {
			titles = append(titles, r.Title)
			if strings.Contains(r.Title, "verification of Sign the invoices") {
				return r.Title, true
			}
		}
		return strings.Join(titles, " | "), false
	})
	if st := stateOf(t, ctx, cwm, stuck); st != "realized" {
		t.Errorf("the feature its gate refuses: %q; want still realized", st)
	}
	for _, f := range []string{built, legacy, stuck} {
		if hasItem(asked, f) {
			t.Errorf("« À faire » still asks to confirm %s, which an agent verifies", f)
		}
	}
}

// doneNote is the note of the move that brought an object done, from its
// journal.
func doneNote(t *testing.T, ctx context.Context, id string) string {
	t.Helper()
	j, err := application.UseService[journalapi.JournalService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := j.Transitions(ctx, journalapi.TransitionsInput{ID: id})
	if err != nil {
		return err.Error()
	}
	for i := len(got.Entries) - 1; i >= 0; i-- {
		if got.Entries[i].To == "done" {
			return got.Entries[i].Note
		}
	}
	return ""
}

func hasItem(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}
