// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A DEPENDS-ON IS MET ONCE ONLY THE PERSON'S CHECKS ARE LEFT
// : B depends on request A; A's agent steps are done and
// what is left of A is a person's - its by-hand deliverable, its review - so
// B's next step is ready while A stays open on the person's checks.
func TestADependencyWithOnlyThePersonsChecksLeftIsMet(t *testing.T) {
	t.Parallel()
	ctx, _, req := world(t)
	a := file(t, ctx, req, api.FileInput{Title: "Show the invoice", Module: "billing", Kind: api.KindBuild, By: person,
		Deliverables: []api.Deliverable{{Title: "The invoice reads well on screen", ByHand: "Open an invoice and read it: only a person judges how it reads."}}})
	b := file(t, ctx, req, api.FileInput{Title: "Print the invoice", Module: "billing", Kind: api.KindFix, By: person, DependsOn: []string{a.ID}})
	pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	agentStepsOf(t, ctx, a.ID, 1)
	if st := stepOf(t, ctx, b.ID, "implement"); st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, a.ID) {
		t.Fatalf("A has an agent step left: B's implement %s (%q); want waiting on %s", st.State, st.Waits, a.ID)
	}
	agentStepsOf(t, ctx, a.ID, 0)
	left := openOf(t, ctx, a.ID)
	if len(left) == 0 {
		t.Fatalf("A has no step left; the test wants A open on a person's check")
	}
	for _, td := range left {
		if td.For != schedapi.ForPerson {
			t.Fatalf("A's %s is an agent's (%s); the test wants only a person's left", td.Packet.Step, td.State)
		}
	}
	if st := stepOf(t, ctx, b.ID, "implement"); st.State != schedapi.StateReady {
		t.Fatalf("only the person's checks of A left (%v): B's implement %s (%q); want ready", steps(t, ctx, a.ID), st.State, st.Waits)
	}
}

// A DEPENDS-ON IS HELD WHILE A REBUILD IS OPEN (merged
// RQ-427): B depends on feature A, done; a fix of A is filed and its
// implement is open - B's next step waits while the rebuild is an agent's
// to do.
func TestADependencyOnADoneFeatureWaitsWhileItsRebuildIsOpen(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	a := want(t, ctx, cwm, "feature", "The invoice is totalled")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: a, To: "done", By: person, Force: true, Note: "verified by hand"}); err != nil {
		t.Fatal(err)
	}
	if _, err := req.File(ctx, api.FileInput{On: a, Kind: api.KindFix, By: person}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, a, api.StepIntake).ID, session, schedapi.Result{Summary: "a rebuild"})
	b := file(t, ctx, req, api.FileInput{Title: "Print the total", Module: "billing", Kind: api.KindFix, By: person, DependsOn: []string{a}})
	pullAndReport(t, ctx, stepOf(t, ctx, b.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	if st := stepOf(t, ctx, b.ID, "implement"); st.State != schedapi.StateWaiting || !strings.Contains(st.Waits, a) {
		t.Fatalf("A's rebuild open (%v): B's implement %s (%q); want waiting on %s", steps(t, ctx, a), st.State, st.Waits, a)
	}
}

// agentStepsOf works the request's ready agent steps, one after the other,
// until only `keep` of its agent steps are left.
func agentStepsOf(t *testing.T, ctx context.Context, request string, keep int) {
	t.Helper()
	for range 20 {
		var agent []api.WorkTodo
		for _, td := range openOf(t, ctx, request) {
			if td.For != schedapi.ForPerson {
				agent = append(agent, td)
			}
		}
		if len(agent) <= keep {
			return
		}
		ready := false
		for _, td := range agent {
			if td.State == schedapi.StateReady {
				pullAndReport(t, ctx, td.ID, session, schedapi.Result{Summary: td.Packet.Step + " done"})
				ready = true
				break
			}
		}
		if !ready {
			t.Fatalf("%s: agent steps %v left, none ready", request, steps(t, ctx, request))
		}
	}
}

// openOf is the request's scheduled to-dos.
func openOf(t *testing.T, ctx context.Context, request string) []api.WorkTodo {
	t.Helper()
	got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: request})
	if err != nil {
		t.Fatal(err)
	}
	return got.Todos
}
