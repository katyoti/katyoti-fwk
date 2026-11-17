// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A ready to-do ahead in the queue reserves its modules against the ones after
// it for a pull without an id - two workers never get two writers of one
// module - and the wait names the to-do that reserves it; a pull of a given
// to-do counts only what is held, so it takes a later one of the same module
// while nothing holds the module.
func TestAPullByIDIsNotShadowedByAReadyToDoAheadOfIt(t *testing.T) {
	ctx, _, svc, _ := world(t)
	first := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-001", Title: "implement", Priority: "P1", Writes: []string{"billing"}})
	later := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-001", Title: "auto-test", Writes: []string{"billing"}})

	q, err := svc.Queue(ctx, api.Filter{Module: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	var waits string
	for _, e := range q.Queue {
		if e.Todo.ID == later.ID {
			waits = e.Waits
		}
	}
	if !strings.HasPrefix(waits, "module billing is in use") || !strings.Contains(waits, "reserved by "+first.ID) {
		t.Fatalf("the later one waits %q; want billing in use, reserved by %s", waits, first.ID)
	}

	got, err := svc.Pull(ctx, api.PullInput{ID: later.ID, By: alice})
	if err != nil {
		t.Fatal(err)
	}
	if got.Todo == nil || got.Todo.ID != later.ID {
		t.Fatalf("pull --id of the later one: %+v; want it taken, nothing holds billing", got)
	}

	// Now it holds billing: the earlier one waits, and says who holds it.
	other, err := svc.Pull(ctx, api.PullInput{ID: first.ID, By: bob})
	if err != nil {
		t.Fatal(err)
	}
	if other.Todo != nil || len(other.Waiting) != 1 || !strings.Contains(other.Waiting[0], "module billing is in use (held by "+later.ID+")") {
		t.Fatalf("pull --id of the earlier one: %+v; want it waiting on billing held by %s", other, later.ID)
	}
}
