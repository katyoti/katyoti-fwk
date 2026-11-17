// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A PERSON'S TO-DO IS THE PERSON'S — reported from an agent's session (the
// command's cause is an agent: person.CommandCause under Claude Code), it is
// refused whatever by says; the person reporting it themselves is collected.
func TestAnAgentSessionNeverReportsAPersonsToDo(t *testing.T) {
	ctx, _, svc, got := world(t)
	ask := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", For: api.ForPerson, Start: api.StartManual})
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	report := api.ReportInput{ID: ask.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "approved", By: person}}
	if _, err := svc.Report(agent, report); err == nil || !strings.Contains(err.Error(), "agent's session") {
		t.Fatalf("a person's to-do reported from an agent's session: %v; want refused", err)
	}
	if len(got.seen()) != 0 {
		t.Fatalf("collected after a refused report: %v", got.seen())
	}
	own := cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	if _, err := svc.Report(own, report); err != nil {
		t.Fatalf("the person reporting it: %v", err)
	}
	if len(got.seen()) != 1 {
		t.Fatalf("collected: %v; want the person's report", got.seen())
	}
}

// TWO SESSIONS ACTING AS ONE ACTOR ARE TOLD APART — the
// claim records the session an agent pulled from (its cause); the same actor
// from another session is told so on pull, and refused renewing, releasing
// or reporting it; the same session, or a call naming no session, goes on.
func TestAnotherSessionOfTheSameActorIsToldApart(t *testing.T) {
	ctx, _, svc, _ := world(t)
	td := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement"})
	from := func(session string) context.Context {
		return cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code", Session: session})
	}
	a, b := from("session-one"), from("session-two")
	got, err := svc.Pull(a, api.PullInput{By: alice})
	if err != nil || got.Todo == nil || got.Todo.ID != td.ID {
		t.Fatalf("pull from session-one: %+v %v", got, err)
	}
	if got.Todo.Claim.Session != "session-one" {
		t.Fatalf("claim session: %q; want session-one", got.Todo.Claim.Session)
	}
	again, err := svc.Pull(b, api.PullInput{By: alice})
	if err != nil || again.Todo != nil || !strings.Contains(again.Nothing, "another session (session-one)") {
		t.Fatalf("pull from session-two: %+v %v; want told the to-do is held from session-one", again, err)
	}
	if _, err := svc.Renew(b, api.RenewInput{ID: td.ID, By: alice}); err == nil || !strings.Contains(err.Error(), "session-one") {
		t.Fatalf("renew from session-two: %v; want refused naming session-one", err)
	}
	if _, err := svc.Release(b, api.ReleaseInput{ID: td.ID, By: alice}); err == nil || !strings.Contains(err.Error(), "session-one") {
		t.Fatalf("release from session-two: %v; want refused naming session-one", err)
	}
	report := api.ReportInput{ID: td.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "done", By: alice}}
	if _, err := svc.Report(b, report); err == nil || !strings.Contains(err.Error(), "session-one") {
		t.Fatalf("report from session-two: %v; want refused naming session-one", err)
	}
	if _, err := svc.Renew(ctx, api.RenewInput{ID: td.ID, By: alice}); err != nil {
		t.Fatalf("renew naming no session: %v; want it goes on", err)
	}
	if _, err := svc.Report(a, report); err != nil {
		t.Fatalf("report from session-one: %v", err)
	}
}
