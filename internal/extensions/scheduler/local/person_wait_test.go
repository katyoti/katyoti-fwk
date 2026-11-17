// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"slices"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// WHILE A PERSON'S STEP WAITS, THE NEXT TAKEABLE TO-DO IS PULLED — request A's
// agent step is reported, its person's step (reading the module, as a step
// added by hand does) waits on the person: a session holding nothing pulls,
// with --module, request B's writer of the same module; the person's waiting
// step holds no lock.
func TestAPersonsWaitLetsTheNextRequestBeTaken(t *testing.T) {
	ctx, _, svc, got := world(t)
	analyse := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "analyse A", Priority: "P1", Reads: []string{"billing"}})
	approve := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "approve A", Priority: "P1", For: api.ForPerson,
		Start: api.StartManual, Reads: []string{"billing"}, State: api.StateWaiting})
	build := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-B", Title: "implement B", Priority: "P3", Writes: []string{"billing"}})

	pulled, err := svc.Pull(ctx, api.PullInput{ID: analyse.ID, By: alice})
	if err != nil || pulled.Todo == nil {
		t.Fatalf("pull A's analysis: %+v, %v", pulled, err)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: analyse.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "analysed", By: alice}}); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.seen(), analyse.ID) {
		t.Fatalf("A's analysis not collected: %v", got.seen())
	}
	// A's person's step is now up: it waits on the person.
	approve.State = api.StateReady
	if _, err := svc.Put(ctx, api.PutInput{Todo: approve}); err != nil {
		t.Fatal(err)
	}

	next, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Module: "billing"}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Todo == nil || next.Todo.ID != build.ID {
		t.Fatalf("the pull while A's person's step waits: %+v; want B's implement", next)
	}
}
