// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A WORKER WHOSE LEASE PASSED STILL REPORTS ITS WORK: while
// no one pulled the to-do again, its last holder reports it — even while
// another session holds the module it writes (a report writes only the
// scheduler's files); once another worker pulled it, the report is refused.
func TestALapsedHolderReportsWhileNoOneTookItsToDo(t *testing.T) {
	ctx, _, svc, _ := world(t)
	carol := cwmapi.Actor{ID: "session-c", Kind: "agent"}
	first := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", Writes: []string{"billing"}, Priority: "P3"})
	if got, err := svc.Pull(ctx, api.PullInput{ID: first.ID, By: alice, LeaseSeconds: 1}); err != nil || got.Todo == nil {
		t.Fatalf("alice pulls: %+v, %v", got, err)
	}
	time.Sleep(1500 * time.Millisecond)
	other := put(t, ctx, svc, api.Todo{Module: "billing", Title: "review", Writes: []string{"billing"}, Priority: "P0"})
	if got, err := svc.Pull(ctx, api.PullInput{ID: other.ID, By: bob}); err != nil || got.Todo == nil {
		t.Fatalf("bob takes the module meanwhile: %+v, %v", got, err)
	}
	out, err := svc.Report(ctx, api.ReportInput{ID: first.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "done after the lease", By: alice}})
	if err != nil {
		t.Fatalf("the lapsed holder's report, no one having pulled it again: %v", err)
	}
	if out.Todo.Result == nil || out.Todo.Result.Summary != "done after the lease" {
		t.Errorf("reported to-do = %+v", out.Todo)
	}

	second := put(t, ctx, svc, api.Todo{Module: "billing", Title: "document"})
	if got, err := svc.Pull(ctx, api.PullInput{ID: second.ID, By: alice, LeaseSeconds: 1}); err != nil || got.Todo == nil {
		t.Fatalf("alice pulls the second: %+v, %v", got, err)
	}
	time.Sleep(1500 * time.Millisecond)
	if got, err := svc.Pull(ctx, api.PullInput{ID: second.ID, By: carol}); err != nil || got.Todo == nil {
		t.Fatalf("carol pulls it again: %+v, %v", got, err)
	}
	_, err = svc.Report(ctx, api.ReportInput{ID: second.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "late", By: alice}})
	if err == nil || !strings.Contains(err.Error(), "held by session-c") {
		t.Errorf("alice's report after carol pulled it again: %v, want refused (held by session-c)", err)
	}
	if _, err := svc.Release(ctx, api.ReleaseInput{ID: second.ID, By: alice}); err == nil {
		t.Errorf("alice released a to-do carol pulled again")
	}
}
