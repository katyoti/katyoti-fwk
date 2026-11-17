// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// intakeQuestions are the open human tasks asking the requester of request.
func intakeQuestions(t *testing.T, ctx context.Context, request string) []schedapi.Todo {
	t.Helper()
	open, err := schedOf(t, ctx).List(ctx, schedapi.ListInput{})
	if err != nil {
		t.Fatal(err)
	}
	var out []schedapi.Todo
	for _, td := range open.Todos {
		if ask, ok := inboxapi.AskOf(td); ok && ask.About == request && td.State == schedapi.StateReady {
			out = append(out, td)
		}
	}
	return out
}

// TestAnIntakeQuestionIsAProvideHumanTaskOnceIN039IsOn (
// decision): with IN-039's switch on, a request asking its
// requester one question starts one provide human task about the request -
// asked of the person who filed it, settling that question - and waits idle
// on it; no inbox entry, no asked-in edge; a second question waits. The
// person's answer, kept under the request, is added to its details and plans
// its intake again. A request that cannot wait asks nothing. With the switch
// off, the question goes to the inbox as before.
func TestAnIntakeQuestionIsAProvideHumanTaskOnceIN039IsOn(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	r := file(t, ctx, req, api.FileInput{Title: "Faster invoices", Module: "billing", Details: "they are slow"})
	got, err := req.Ask(ctx, api.AskInput{ID: r.ID, Question: "Faster to produce, or faster to be paid?", By: intake})
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.State != api.PlanIdle || len(got.Request.AskedIn) != 0 || !strings.Contains(got.Request.Plan.Note, "Faster to produce") {
		t.Fatalf("asked = %s, asked in %v, plan %+v; want idle waiting on the answer, no inbox entry", got.Request.State, got.Request.AskedIn, got.Request.Plan)
	}
	qs := intakeQuestions(t, ctx, r.ID)
	if len(qs) != 1 {
		t.Fatalf("human tasks about %s = %d, want one", r.ID, len(qs))
	}
	ask, _ := inboxapi.AskOf(qs[0])
	if ask.Kind != inboxapi.KindProvide || ask.Title != "Faster to produce, or faster to be paid?" || !strings.Contains(ask.Question, r.ID) ||
		len(ask.Satisfies) != 1 || !strings.HasPrefix(ask.Satisfies[0], r.ID+"/question@") ||
		ask.Assignee == nil || ask.Assignee.ID != person.ID || ask.AnswerableBy != inboxapi.AnswerablePerson {
		t.Fatalf("asked %+v; want a provide question about the request, of its requester only, settling that question", ask)
	}
	tri, err := inboxOf(t, ctx).Triage(ctx, inboxapi.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range tri.Entries {
		if e.About == r.ID || strings.Contains(e.RequestedBy, r.ID) {
			t.Fatalf("an inbox entry was filed: %+v", e)
		}
	}
	if _, err := req.Ask(ctx, api.AskInput{ID: r.ID, Question: "And another?", By: intake}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Errorf("a second question while one waits: %v, want refused (one question)", err)
	}

	if _, err := schedOf(t, ctx).Report(ctx, schedapi.ReportInput{ID: qs[0].ID, Result: schedapi.Result{
		Status: schedapi.StatusSuccess, Summary: "answered", Answer: "paid: the reminders", By: person}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		now, err := req.Get(ctx, api.GetInput{ID: r.ID})
		if err == nil && now.Request.State == api.PlanReady {
			d := now.Request.Details
			last := now.Request.Plan.Trail[len(now.Request.Plan.Trail)-1]
			if !strings.HasPrefix(last.Note, "answered: paid: the reminders") || !strings.HasPrefix(d, "they are slow") ||
				!strings.Contains(d, "Q: Faster to produce, or faster to be paid?\nA: paid: the reminders") {
				t.Fatalf("woken = last move %+v, details %q; want its intake planned again and the Q/A added", last, d)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the request's intake is not planned again 5s after the answer: %+v, %v", now.Request.State, err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	// A split request cannot wait on an answer: nothing is asked.
	split := file(t, ctx, req, api.FileInput{Title: "Mail and print invoices", Module: "billing"})
	if _, err := req.Split(ctx, api.SplitInput{ID: split.ID, By: intake, Note: "two", Parts: []api.SplitPart{{Module: "billing", Title: "Mail them"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.Ask(ctx, api.AskInput{ID: split.ID, Question: "Mail or print first?", By: intake}); err == nil {
		t.Error("asking a split request was accepted; want it refused")
	}
	if qs := intakeQuestions(t, ctx, split.ID); len(qs) != 0 {
		t.Errorf("a split request left %d question(s) asked", len(qs))
	}
}
