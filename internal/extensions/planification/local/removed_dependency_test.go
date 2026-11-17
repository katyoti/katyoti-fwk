// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	planlocal "github.com/katyoti/katyoti-fwk/internal/extensions/planification/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A REMOVED DEPENDS-ON EDGE RELEASES THE TO-DOS IT HELD:
// the edge removed past the requests service (cwm edge-delete), the to-do that
// waited on it is settled again - ready, no stale reason left.
func TestRemovedDependencyReleasesItsTodos(t *testing.T) {
	t.Parallel()
	ctx, cwm, req := world(t)
	first := file(t, ctx, req, api.FileInput{Title: "Invoices carry a number", Kind: api.KindFix})
	then := file(t, ctx, req, api.FileInput{Title: "Reminders name the invoice number", Kind: api.KindFix})
	if _, err := req.Depend(ctx, api.DependInput{ID: then.ID, On: first.ID}); err != nil {
		t.Fatal(err)
	}
	pullAndReport(t, ctx, stepOf(t, ctx, then.ID, api.StepIntake).ID, session, schedapi.Result{Summary: "a fix"})
	held := stepOf(t, ctx, then.ID, "implement")
	if held.State != schedapi.StateWaiting || !strings.Contains(held.Waits, first.ID) {
		t.Fatalf("before the edge is removed the implement is %s (%q); want waiting on %s", held.State, held.Waits, first.ID)
	}

	if _, err := cwm.EdgeDelete(ctx, cwmapi.EdgeDeleteInput{From: then.ID, Kind: cwmapi.EdgeType(api.EdgeDependsOn), To: first.ID}); err != nil {
		t.Fatal(err)
	}
	planlocal.Settle(ctx, 5*time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := stepOf(t, ctx, then.ID, "implement")
		if got.State == schedapi.StateReady && got.Waits == "" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("5 s after the edge was removed the implement is %s (%q); want ready", got.State, got.Waits)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
