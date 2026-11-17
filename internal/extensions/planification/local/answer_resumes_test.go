// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// AN ANSWER RESUMES THE STEP THAT ASKED: an intake that
// asks its requester and reports its to-do blocked leaves the request's plan
// ready, not idle - the answer still reaches it: the question and the answer
// join the request's details and the blocked intake comes back to the queue.
// The question is a provide human task.
func TestAnAnswerResumesTheStepThatAsked(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing", Details: "they are slow"})
	td := stepOf(t, ctx, r.ID, api.StepIntake)
	s := schedOf(t, ctx)
	if _, err := s.Pull(ctx, schedapi.PullInput{ID: td.ID, By: session}); err != nil {
		t.Fatal(err)
	}
	got, err := req.Ask(ctx, api.AskInput{ID: r.ID, Question: "Faster to produce, or faster to be paid?", By: session})
	if err != nil {
		t.Fatal(err)
	}
	qs := intakeQuestions(t, ctx, r.ID)
	if len(got.Request.AskedIn) != 0 || len(qs) != 1 {
		t.Fatalf("asked in %v, %d human task(s); want one human task, no inbox entry", got.Request.AskedIn, len(qs))
	}
	ask := qs[0].ID
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: td.ID, Result: schedapi.Result{Status: schedapi.StatusBlocked, Summary: "asked the requester", By: session}}); err != nil {
		t.Fatal(err)
	}
	// While it waits, « À faire » asks the person the question (one card),
	// never only a blocked to-do.
	views, err := application.UseService[api.ViewService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := views.PersonQueue(ctx, api.PersonQueueInput{})
	if err != nil {
		t.Fatal(err)
	}
	var answers, unblocks int
	for _, it := range q.Items {
		if it.Action == api.ActionAnswer && it.Todo == ask {
			answers++
		}
		if it.Request == r.ID && it.Action == api.ActionUnblock {
			unblocks++
		}
	}
	if answers != 1 || unblocks != 0 {
		t.Errorf("« À faire » while the question waits: %d answer cards, %d unblock cards for %s; want one answer card, no unblock: %+v", answers, unblocks, r.ID, q.Items)
	}
	if _, err := s.Report(ctx, schedapi.ReportInput{ID: ask, Result: schedapi.Result{
		Status: schedapi.StatusSuccess, Summary: "answered", Answer: "paid: the reminders", By: person}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		now, err := req.Get(ctx, api.GetInput{ID: r.ID})
		step := stepOf(t, ctx, r.ID, api.StepIntake)
		if err == nil && strings.Contains(now.Request.Details, "A: paid: the reminders") && step.Waits == "" && step.State == schedapi.StateReady {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("5s after the answer: details %q, the intake %+v; want the Q/A added and the intake back in the queue", now.Request.Details, step)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
