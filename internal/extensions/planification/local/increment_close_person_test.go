// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// CLOSING AN INCREMENT TAKES A PERSON: everything the
// increment holds is done. An agent's `increments done` is refused, saying how
// the person closes it, and so is its move of the increment through cwm; the
// person's is accepted.
func TestClosingAnIncrementTakesAPerson(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, f, "realized", session); err != nil {
		t.Fatal(err)
	}
	if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
		t.Fatal(err)
	}

	_, err := increments(t, ctx).Done(ctx, api.IncrementMoveInput{ID: in.ID, By: session})
	if err == nil || !strings.Contains(err.Error(), "takes a person") {
		t.Fatalf("an agent closed %s (err %v); want it refused, saying it takes a person", in.ID, err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: in.ID, To: api.IncrementDone, By: session}); err == nil {
		t.Fatalf("an agent moved %s done through cwm; want its workflow to refuse it", in.ID)
	}
	if got, _ := increments(t, ctx).Get(ctx, api.GetInput{ID: in.ID}); got.Increment.State != api.IncrementOpen {
		t.Fatalf("%s after the agent's tries: %s; want open", in.ID, got.Increment.State)
	}
	if _, err := increments(t, ctx).Done(ctx, api.IncrementMoveInput{ID: in.ID, By: person, Force: true, Note: forcedNote}); err != nil {
		t.Fatalf("the person closing %s: %v", in.ID, err)
	}
}
