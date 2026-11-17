// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A PERSON CONFIRMING AN INCREMENT'S LAST FEATURE ASKS ITS DELIVERY
// : the increment holds one feature, realized - built
// a person confirms it. No request's work ends: the person confirms it done
// (its Lifecycle tab, « À faire »). Everything the increment holds is done,
// so the person's approval of its delivery is asked at once - never an
// increment left delivering with nothing asked - and asked once.
func TestAPersonConfirmingAnIncrementsLastFeatureAsksItsDelivery(t *testing.T) {
	t.Parallel()
	ctx, cwm, _ := world(t)
	in := increment(t, ctx, "Invoices reach their customers")
	f := want(t, ctx, cwm, "feature", "Print the invoices")
	if _, err := increments(t, ctx).Target(ctx, api.IncrementTargetInput{ID: f, Increment: in.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := cwm.ObjectTransition(ctx, cwmapi.ObjectTransitionInput{ID: f, To: "realized", By: session, Note: "built"}); err != nil {
		t.Fatal(err)
	}
	approvals := func() []api.WorkTodo {
		t.Helper()
		got, err := workOf(t, ctx).List(ctx, api.ListWorkInput{Request: in.ID})
		if err != nil {
			t.Fatal(err)
		}
		var out []api.WorkTodo
		for _, td := range got.Todos {
			if td.Packet.Step == "approve-delivery" {
				out = append(out, td)
			}
		}
		return out
	}
	if a := approvals(); len(a) != 0 {
		t.Fatalf("the delivery asked while %s is realized: %+v", f, a)
	}

	if err := move(ctx, cwm, f, api.StateDone, person); err != nil {
		t.Fatal(err)
	}
	// Its transitioned event asks the delivery, shortly after.
	for i := 0; i < 250 && len(approvals()) == 0; i++ {
		time.Sleep(40 * time.Millisecond)
	}
	a := approvals()
	if len(a) != 1 {
		t.Fatalf("after the person confirmed the increment's last feature: %d approve-delivery steps; want one", len(a))
	}
	if a[0].For != schedapi.ForPerson || a[0].Packet.Template != "increment-accept" {
		t.Fatalf("the delivery's approval: %+v; want the person's (increment-accept)", a[0])
	}
}
