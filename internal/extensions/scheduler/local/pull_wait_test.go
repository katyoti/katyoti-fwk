// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A PULL THAT WAITS — a worker asks with a wait instead of
// stopping: it is answered the to-do once one of its scope can be pulled,
// finished once the scope holds nothing an agent can move, or timeout while
// work of the scope is still held.

// The next step of a request waits behind the step another session holds: a
// wait answers it once that step is reported and its owner releases the next.
func TestAPullThatWaitsTakesTheNextStepOnceTheHeldOneIsReported(t *testing.T) {
	ctx, _, svc, _ := world(t)
	label := []string{"increment:IN-1"}
	analyse := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "analyse", Writes: []string{"billing"}, Labels: label})
	build := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "implement", Writes: []string{"billing"}, Labels: label, State: api.StateWaiting})
	if got, err := svc.Pull(ctx, api.PullInput{ID: analyse.ID, By: bob}); err != nil || got.Todo == nil {
		t.Fatalf("bob pulls the analysis: %+v, %v", got, err)
	}

	type answer struct {
		out api.PullOutput
		err error
	}
	done := make(chan answer, 1)
	go func() {
		out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Labels: label}, Wait: "20s"})
		done <- answer{out, err}
	}()
	select {
	case a := <-done:
		t.Fatalf("the wait answered while bob still holds the analysis: %+v, %v", a.out, a.err)
	case <-time.After(500 * time.Millisecond):
	}

	if _, err := svc.Report(ctx, api.ReportInput{ID: analyse.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "analysed", By: bob}}); err != nil {
		t.Fatal(err)
	} // its owner collects it (the test collector archives it)
	build.State = api.StateReady // and releases the next step
	if _, err := svc.Put(ctx, api.PutInput{Todo: build}); err != nil {
		t.Fatal(err)
	}

	select {
	case a := <-done:
		if a.err != nil || a.out.Todo == nil || a.out.Todo.ID != build.ID || a.out.Ended != "" {
			t.Fatalf("the wait's answer: %+v, %v; want the implement step pulled", a.out, a.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the wait did not take the next step once it was ready")
	}
}

// Only a person's step is left in the scope: a wait answers finished at once.
func TestAPullThatWaitsIsFinishedWhenOnlyAPersonsStepRemains(t *testing.T) {
	ctx, _, svc, _ := world(t)
	label := []string{"increment:IN-2"}
	put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "approve", For: api.ForPerson, Start: api.StartManual, Labels: label})
	put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "after the approval", Labels: label, State: api.StateWaiting})
	put(t, ctx, svc, api.Todo{Module: "billing", Title: "another scope", Labels: []string{"increment:other"}})

	start := time.Now()
	out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Labels: label}, Wait: "20s"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo != nil || out.Ended != api.EndedFinished {
		t.Fatalf("the wait's answer: %+v; want finished", out)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Errorf("finished after %s; want at once", took)
	}
	// A label no to-do carries is finished too.
	if out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Labels: []string{"nowhere:x"}}, Wait: "1m"}); err != nil || out.Ended != api.EndedFinished {
		t.Fatalf("an empty label's wait: %+v, %v; want finished", out, err)
	}
}

// The scope's work is held by another session and nothing is released before
// the wait passes: timeout, saying why the ready one waits.
func TestAPullThatWaitsTimesOutWhileTheScopesWorkIsHeld(t *testing.T) {
	ctx, _, svc, _ := world(t)
	label := []string{"increment:IN-3"}
	held := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "implement A", Writes: []string{"billing"}, Labels: label})
	put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-B", Title: "implement B", Writes: []string{"billing"}, Labels: label})
	if got, err := svc.Pull(ctx, api.PullInput{ID: held.ID, By: bob}); err != nil || got.Todo == nil {
		t.Fatalf("bob pulls A: %+v, %v", got, err)
	}

	start := time.Now()
	out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Labels: label}, Wait: "1s"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Todo != nil || out.Ended != api.EndedTimeout || len(out.Waiting) == 0 {
		t.Fatalf("the wait's answer: %+v; want timeout with why B waits", out)
	}
	if took := time.Since(start); took < time.Second {
		t.Errorf("timed out after %s; want the whole wait", took)
	}
	if _, err := svc.Pull(ctx, api.PullInput{By: alice, Wait: "2h"}); err == nil {
		t.Error("a wait above the most a pull waits was accepted")
	}
}
