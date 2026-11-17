// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
)

// A PULL THAT WAITS IS NOT FINISHED WHILE A COLLECT OF ITS SCOPE RUNS
// : an owner archives a reported step first and releases the
// step after it last - after work that can take minutes (a request's checks).
// While that collect runs the scope is not finished, however long it takes; a
// wait scoped to the request takes the next step once released. Once no
// collect runs and nothing is ready, the wait is finished.
func TestAPullThatWaitsIsNotFinishedWhileACollectOfItsScopeRuns(t *testing.T) {
	archived, release := make(chan struct{}), make(chan struct{})
	var next api.Todo // the step the slow owner releases at the end of its collect
	slow := func(init *application.ApplicationInit) {
		init.AddContribute("scheduler-test-slow", ".collector", func(app *application.Application) error {
			return schedlocal.ContributeCollector(app, api.Collector{Owner: "slow", Collect: func(ctx context.Context, td api.Todo) error {
				svc, err := application.UseService[api.TodoService](ctx)
				if err != nil {
					return err
				}
				if _, err := svc.Archive(ctx, api.ArchiveInput{ID: td.ID, Note: "collected"}); err != nil {
					return err
				}
				if next.ID == "" {
					return nil
				}
				close(archived)
				<-release // the long part of the collect (a request's checks)
				next.State = api.StateReady
				_, err = svc.Put(ctx, api.PutInput{Todo: next})
				return err
			}})
		})
	}
	ctx, _, svc, _ := world(t, slow)
	analyse := put(t, ctx, svc, api.Todo{Owner: "slow", Module: "billing", Request: "billing.RQ-A", Title: "analyse", Writes: []string{"billing"}})
	next = put(t, ctx, svc, api.Todo{Owner: "slow", Module: "billing", Request: "billing.RQ-A", Title: "implement", Writes: []string{"billing"}, State: api.StateWaiting, After: []string{analyse.ID}})
	if got, err := svc.Pull(ctx, api.PullInput{ID: analyse.ID, By: bob}); err != nil || got.Todo == nil {
		t.Fatalf("bob pulls the analysis: %+v, %v", got, err)
	}
	reported := make(chan error, 1)
	go func() {
		_, err := svc.Report(ctx, api.ReportInput{ID: analyse.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "analysed", By: bob}})
		reported <- err
	}()
	<-archived // the step is archived; the next one is not released yet

	type answer struct {
		out api.PullOutput
		err error
	}
	done := make(chan answer, 1)
	go func() {
		out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Request: "billing.RQ-A"}, Wait: "30s"})
		done <- answer{out, err}
	}()
	select {
	case a := <-done:
		close(release)
		t.Fatalf("the wait answered while the collect that releases the next step ran: %+v, %v", a.out, a.err)
	case <-time.After(5 * time.Second): // more than two looks of the wait
	}
	close(release)
	if err := <-reported; err != nil {
		t.Fatal(err)
	}
	select {
	case a := <-done:
		if a.err != nil || a.out.Todo == nil || a.out.Todo.ID != next.ID {
			t.Fatalf("the wait's answer: %+v, %v; want the implement step pulled", a.out, a.err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the wait did not take the next step once released")
	}
	if _, err := svc.Release(ctx, api.ReleaseInput{ID: next.ID, By: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Archive(ctx, api.ArchiveInput{ID: next.ID, Note: "done"}); err != nil {
		t.Fatal(err)
	}

	// No collect running, nothing ready: the scope is finished at once.
	start := time.Now()
	out, err := svc.Pull(ctx, api.PullInput{By: alice, Filter: api.Filter{Request: "billing.RQ-A"}, Wait: "30s"})
	if err != nil || out.Ended != api.EndedFinished || time.Since(start) > 5*time.Second {
		t.Fatalf("the wait on an emptied request: %+v, %v after %s; want finished at once", out, err, time.Since(start))
	}
}
