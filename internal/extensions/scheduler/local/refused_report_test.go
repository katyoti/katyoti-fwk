// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
)

// A REPORT ITS OWNER REFUSES KEEPS ITS REPORTER'S LIVE CLAIM:
// an owner whose gate refuses a report puts the to-do back in the queue, ready
// and unclaimed (planification's back-to-queue); while the reporter's lease
// runs the to-do stays the reporter's - another worker's pull gets nothing for
// it, the reporter's pull by id answers it still held, and its report again
// is collected. Once the lease has passed, a refused report frees it for
// anyone.
func TestARefusedReportKeepsItsReportersLiveClaim(t *testing.T) {
	var refusals atomic.Int32 // the reports the gate still refuses
	gate := func(init *application.ApplicationInit) {
		init.AddContribute("scheduler-test-gate", ".collector", func(app *application.Application) error {
			return schedlocal.ContributeCollector(app, api.Collector{Owner: "gate", Collect: func(ctx context.Context, td api.Todo) error {
				svc, err := application.UseService[api.TodoService](ctx)
				if err != nil {
					return err
				}
				if refusals.Add(-1) >= 0 {
					back := td
					back.State, back.Claim, back.Result = api.StateReady, nil, nil
					if _, err := svc.Put(ctx, api.PutInput{Todo: back}); err != nil {
						return err
					}
					return errors.New("the step's verdict is missing")
				}
				_, err = svc.Archive(ctx, api.ArchiveInput{ID: td.ID, Note: "collected"})
				return err
			}})
		})
	}
	ctx, _, svc, _ := world(t, gate)
	report := func(id, summary string) error {
		_, err := svc.Report(ctx, api.ReportInput{ID: id, Result: api.Result{Status: api.StatusSuccess, Summary: summary, By: alice}})
		return err
	}

	review := put(t, ctx, svc, api.Todo{Owner: "gate", Module: "billing", Title: "review", Writes: []string{"billing"}})
	if got, err := svc.Pull(ctx, api.PullInput{ID: review.ID, By: alice}); err != nil || got.Todo == nil {
		t.Fatalf("alice pulls the review: %+v, %v", got, err)
	}
	refusals.Store(1)
	if err := report(review.ID, "reviewed, no verdict"); err == nil {
		t.Fatal("the gate's refusal was not returned to the reporter")
	}
	if got, err := svc.Pull(ctx, api.PullInput{By: bob, Filter: api.Filter{Module: "billing"}}); err != nil || got.Todo != nil {
		t.Fatalf("bob pulls while alice's lease runs: %+v, %v; want nothing (the review stays alice's)", got.Todo, err)
	}
	got, err := svc.Pull(ctx, api.PullInput{ID: review.ID, By: alice})
	if err != nil || got.Todo == nil || got.Todo.ID != review.ID || got.Todo.Claim == nil || got.Todo.Claim.By != alice {
		t.Fatalf("alice pulls her refused review by id: %+v, %v; want it answered, still hers", got, err)
	}
	if err := report(review.ID, "reviewed, approved"); err != nil {
		t.Fatalf("alice reports again: %v", err)
	}
	if got, err := svc.Get(ctx, api.GetInput{ID: review.ID}); err != nil || got.Todo.Archived == "" {
		t.Errorf("alice's report again: %+v, %v; want it collected (archived)", got.Todo, err)
	}

	// The lease passed: a refused report frees the to-do for anyone.
	late := put(t, ctx, svc, api.Todo{Owner: "gate", Module: "billing", Title: "verify", Writes: []string{"billing"}})
	if got, err := svc.Pull(ctx, api.PullInput{ID: late.ID, By: alice, LeaseSeconds: 1}); err != nil || got.Todo == nil {
		t.Fatalf("alice pulls the verify: %+v, %v", got, err)
	}
	time.Sleep(1500 * time.Millisecond)
	refusals.Store(1)
	if err := report(late.ID, "verified late, no verdict"); err == nil {
		t.Fatal("the gate's refusal was not returned to the lapsed reporter")
	}
	if got, err := svc.Pull(ctx, api.PullInput{ID: late.ID, By: bob}); err != nil || got.Todo == nil {
		t.Fatalf("bob pulls the lapsed refused verify: %+v, %v; want it his", got, err)
	}
}
