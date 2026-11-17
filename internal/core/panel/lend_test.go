// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/progress"
)

// A run lends its panel: the lent handle resumes at the run's
// sequence, reports and asks but never completes or closes, and the run's
// handle goes on after the lent one's last event once it takes it back.
func TestALentHandleReportsAfterTheRunAndNeverCompletes(t *testing.T) {
	ctx := context.Background()
	publish, got := recorder(t)
	run, err := OpenPanel(ctx, scope, publish, Open{Type: "ktw-test:/panel-type/progress"})
	if err != nil {
		t.Fatal(err)
	}
	ref := run.Ref()
	if ref.ID != run.ID() || ref.Seq != 1 || ref.Tenant != scope.Tenant || ref.Universe != scope.Universe {
		t.Fatalf("ref %+v", ref)
	}
	lent, err := Resume(ref, publish)
	if err != nil {
		t.Fatal(err)
	}
	if !lent.Lent() || run.Lent() {
		t.Fatal("only the resumed handle is lent")
	}
	// The method reports with its own progress.Emit.
	work := Lend(ctx, lent)
	if h, ok := HandleFrom(work); !ok || h != lent {
		t.Fatal("Lend did not put the handle on the context")
	}
	progress.Emit(work, Progress{Label: "a", Current: 1, Total: 2})
	progress.Emit(work, Progress{Label: "a", Current: 2, Total: 2})
	if err := lent.Complete(ctx, "ok", nil); err == nil {
		t.Fatal("a lent handle completed the panel")
	}
	if err := lent.Close(ctx); err == nil {
		t.Fatal("a lent handle closed the panel")
	}
	run.Advance(lent.Seq())
	run.Advance(1) // an older sequence changes nothing
	if err := run.Complete(ctx, "ok", nil); err != nil {
		t.Fatal(err)
	}
	evs := got()
	if len(evs) != 4 {
		t.Fatalf("%d events, want the open, two reports and the complete: %+v", len(evs), evs)
	}
	for i, e := range evs {
		if e.event.Seq != int64(i+1) {
			t.Fatalf("event %d numbered %d: %+v", i+1, e.event.Seq, evs)
		}
	}
	if evs[3].event.Kind != KindComplete {
		t.Fatalf("the last event is a %s", evs[3].event.Kind)
	}

	if _, err := Resume(Ref{ID: "nope"}, publish); err == nil {
		t.Fatal("a ref naming no panel resumed")
	}
	if _, ok := HandleFrom(ctx); ok {
		t.Fatal("a bare context carries a handle")
	}
}
