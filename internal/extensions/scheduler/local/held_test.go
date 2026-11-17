// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A BLOCKED REPORT WRITES ITS STRUCTURED WAIT ON THE TO-DO:
// kind, target, reason, since and by land in the report's own write - read back
// from the to-do's file; a blocked report naming no kind is kept unclassified,
// its summary its reason; a wait with another status, or of an unknown kind,
// is refused; a pull drops it.
func TestABlockedReportWritesItsStructuredWait(t *testing.T) {
	ctx, _, svc, _ := world(t)
	report := func(td api.Todo, status string, wait *api.HeldOn) (api.TodoOutput, error) {
		t.Helper()
		if got, err := svc.Pull(ctx, api.PullInput{ID: td.ID, By: alice}); err != nil || got.Todo == nil {
			t.Fatalf("pull %s: %+v, %v", td.ID, got, err)
		}
		return svc.Report(ctx, api.ReportInput{ID: td.ID, Result: api.Result{Status: status, Summary: "waits on the invoice numbering", By: alice}, Wait: wait})
	}
	read := func(id string) api.Todo {
		t.Helper()
		got, err := svc.Get(ctx, api.GetInput{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		return got.Todo
	}

	classified := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement"})
	if _, err := report(classified, api.StatusBlocked, &api.HeldOn{Kind: api.HeldObject, Target: "billing.RQ-7", Reason: "the numbering ships first"}); err != nil {
		t.Fatal(err)
	}
	h := read(classified.ID).Held
	if h == nil || h.Kind != api.HeldObject || h.Target != "billing.RQ-7" || h.Reason != "the numbering ships first" || h.Since == "" || h.By != alice {
		t.Fatalf("the blocked to-do's wait = %+v; want object billing.RQ-7, its reason, since and by alice", h)
	}

	unclassified := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review"})
	if _, err := report(unclassified, api.StatusBlocked, nil); err != nil {
		t.Fatal(err)
	}
	if h := read(unclassified.ID).Held; h == nil || h.Kind != "" || h.Reason != "waits on the invoice numbering" || h.By != alice {
		t.Fatalf("a blocked report naming no wait = %+v; want an unclassified wait, its summary its reason", h)
	}

	refused := put(t, ctx, svc, api.Todo{Module: "billing", Title: "document"})
	if _, err := report(refused, api.StatusSuccess, &api.HeldOn{Kind: api.HeldPerson}); err == nil {
		t.Error("a wait on a successful report was accepted")
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: refused.ID, Result: api.Result{Status: api.StatusBlocked, Summary: "stuck", By: alice}, Wait: &api.HeldOn{Kind: "weather"}}); err == nil {
		t.Error("a wait of an unknown kind was accepted")
	}

	// Put back to wait by its owner, then pulled again: the wait goes.
	back := read(classified.ID)
	back.State, back.Result, back.Archived = api.StateReady, nil, ""
	again := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement again", Held: back.Held})
	if read(again.ID).Held == nil {
		t.Fatal("an owner's put dropped the wait")
	}
	if got, err := svc.Pull(ctx, api.PullInput{ID: again.ID, By: bob}); err != nil || got.Todo == nil || got.Todo.Held != nil {
		t.Fatalf("pulled again: %+v, %v; want its wait dropped", got.Todo, err)
	}
}
