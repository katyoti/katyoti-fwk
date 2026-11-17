// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// WANTS PUT FORWARD WAIT IN « À FAIRE » — the goals and features an open
// increment holds, proposed or in review, are one validate item of the
// person's queue: listed, each accepted (reviewed then accepted) or dropped
// there, or every one at once. A want recorded realized is an agent's to
// verify: never in the person's queue. An agent accepts nothing.
func TestWantsPutForwardWaitOnThePersonByIncrement(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	inc := increments(t, ctx)
	in := increment(t, ctx, "Invoices reach their customers by mail")
	proposed := func(title string) string {
		id := want(t, ctx, cwm, "feature", title)
		if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: id, To: "proposed", By: person, Note: "put forward"}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a, b := proposed("Mail the invoices"), proposed("Print them")
	built := want(t, ctx, cwm, "feature", "Archive them")
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: built, To: "realized", By: session, Note: "already built"}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{a, b, built} {
		if _, err := inc.Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
			t.Fatal(err)
		}
	}
	items := itemsOf(personQueue(t, ctx, ""), in.ID)
	if len(items) != 1 || items[0].Action != api.ActionValidate || len(items[0].Wants) != 2 {
		t.Fatalf("the increment's items: %+v; want one validate item listing %s and %s", items, a, b)
	}
	for _, w := range items[0].Wants {
		if w.ID == built {
			t.Errorf("%s, recorded realized, waits on the person; an agent verifies it", built)
		}
	}
	if q := personQueue(t, ctx, a); len(itemsOf(q, in.ID)) != 1 {
		t.Errorf("the queue scoped to %s: %+v; want the increment's item", a, q.Items)
	}

	if _, err := inc.Accept(ctx, api.AcceptWantsInput{ID: in.ID, Want: a, By: session}); err == nil {
		t.Error("an agent accepted a feature; only a person does")
	}
	got, err := inc.Accept(ctx, api.AcceptWantsInput{ID: in.ID, Want: a, By: person})
	if err != nil || len(got.Accepted) != 1 {
		t.Fatalf("accept %s: %+v, %v", a, got, err)
	}
	if st := stateOf(t, ctx, cwm, a); st != "accepted" {
		t.Errorf("%s accepted from « À faire »: %q; want accepted", a, st)
	}
	if items := itemsOf(personQueue(t, ctx, ""), in.ID); len(items) != 1 || len(items[0].Wants) != 1 || items[0].Wants[0].ID != b {
		t.Fatalf("after accepting %s: %+v; want only %s left", a, items, b)
	}
	if got, err := inc.Accept(ctx, api.AcceptWantsInput{ID: in.ID, By: person}); err != nil || len(got.Accepted) != 1 || got.Accepted[0] != b {
		t.Fatalf("accept them all: %+v, %v; want %s", got, err, b)
	}
	if items := itemsOf(personQueue(t, ctx, ""), in.ID); len(items) != 0 {
		t.Errorf("every want accepted, the increment still waits: %+v", items)
	}
}

// stateOf is an object's lifecycle state.
func stateOf(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) string {
	t.Helper()
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	l, _ := got.Object.Lifecycle()
	return l.State
}
