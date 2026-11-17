// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A TO-DO'S WAIT ON ITS INCREMENT'S DEPENDENCY CLEARS ON THE NEXT SWEEP
// : C depends on B, so the build of the work C holds
// waits « its increment C waits on B ». Once B is dropped - or once C's
// depends-on edge to B is removed - nothing settles that work by itself; the
// reconciler's next sweep finds the stale wait and clears it.
func TestTheReconcilerClearsAnIncrementDependencyWaitOnceItEndsOrIsRemoved(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		lift   func(t *testing.T, ctx context.Context, inc api.IncrementService, cwm cwmapi.CWMService, c, b string)
		reason string
	}{
		{"the dependency is dropped", func(t *testing.T, ctx context.Context, inc api.IncrementService, _ cwmapi.CWMService, _, b string) {
			if _, err := inc.Drop(ctx, api.IncrementMoveInput{ID: b, By: person}); err != nil {
				t.Fatal(err)
			}
		}, "which has ended"},
		{"the depends-on edge is removed", func(t *testing.T, ctx context.Context, _ api.IncrementService, cwm cwmapi.CWMService, c, b string) {
			if _, err := cwm.EdgeDelete(ctx, cwmapi.EdgeDeleteInput{From: c, To: b, Kind: api.EdgeDependsOn}); err != nil {
				t.Fatal(err)
			}
		}, "no longer depends on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cwm, req := worldConfigured(t, nil, reconcilerOn(api.RepairClearWait))
			inc := increments(t, ctx)
			create := func(title string) string {
				t.Helper()
				out, err := inc.Create(ctx, api.CreateIncrementInput{Module: "billing", Stream: streamIn(t, ctx, "billing"), Title: title, Goals: []string{title}, By: person})
				if err != nil {
					t.Fatal(err)
				}
				return out.Increment.ID
			}
			b, c := create("B first"), create("C after B")
			if _, err := req.Depend(ctx, api.DependInput{ID: c, On: b}); err != nil {
				t.Fatal(err)
			}
			fix := heldFix(t, ctx, cwm, req, c, "A fix C holds")
			if fix.State != schedapi.StateWaiting || fix.Waits != "its increment "+c+" waits on "+b {
				t.Fatalf("the fix C holds: %s (%q); want it waiting on %s", fix.State, fix.Waits, b)
			}
			tc.lift(t, ctx, inc, cwm, c, b)
			// The broken state: the settle that should follow the change is
			// lost (a move's observer runs on its own, and a hub can stop
			// before it does) - once any observer is through, the step is put
			// back waiting as it was.
			for end := time.Now().Add(2 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
				if stepOf(t, ctx, fix.Request, fix.Packet.Step).State == schedapi.StateReady {
					break
				}
			}
			s := schedOf(t, ctx)
			cur, err := s.Get(ctx, schedapi.GetInput{ID: fix.ID})
			if err != nil {
				t.Fatal(err)
			}
			stale := cur.Todo
			stale.State, stale.Waits = schedapi.StateWaiting, fix.Waits
			if _, err := s.Put(ctx, schedapi.PutInput{Todo: stale}); err != nil {
				t.Fatal(err)
			}
			found := repairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairClearWait, fix.Request)
			if !strings.Contains(found.Reason, tc.reason) {
				t.Errorf("the repair says %q; want it to say %q", found.Reason, tc.reason)
			}
			got := repairOn(t, reconcile(t, ctx, api.ReconcileInput{Apply: true}), api.RepairClearWait, fix.Request)
			if !got.Applied {
				t.Fatalf("not applied: %s", got.Skipped)
			}
			if td := stepOf(t, ctx, fix.Request, fix.Packet.Step); td.State != schedapi.StateReady || td.Waits != "" {
				t.Errorf("after the sweep: %s (%q); want ready", td.State, td.Waits)
			}
			noRepairOn(t, reconcile(t, ctx, api.ReconcileInput{}), api.RepairClearWait, fix.Request)
		})
	}
}
