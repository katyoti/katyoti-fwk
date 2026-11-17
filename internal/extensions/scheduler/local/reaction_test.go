// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A TO-DO AS A REACTION — run by its owner without an agent, retried with a
// backoff up to its limit, put once per key.

// An owner that contributed a runner runs its to-dos itself: no work-todo task
// is started, the runner reports through the contract and the owner collects.
// An owner without one still gets the work-todo task.
func TestAnOwnersRunnerRunsItsToDos(t *testing.T) {
	tasks := &fakeTasks{status: map[string]taskapi.RunStatus{}}
	var ran []string
	ctx, _, svc, got := world(t,
		application.DeclareSimpleService[taskapi.TaskService](func(context.Context) (taskapi.TaskService, error) { return tasks, nil }),
		func(init *application.ApplicationInit) {
			init.AddContribute("scheduler-test", ".runner", func(app *application.Application) error {
				return schedlocal.ContributeRunner(app, api.Runner{Owner: "test", Run: func(ctx context.Context, td api.Todo) (string, string, error) {
					ran = append(ran, td.ID)
					svc, err := application.UseService[api.TodoService](ctx)
					if err != nil {
						return "", "", err
					}
					_, err = svc.Report(ctx, api.ReportInput{ID: td.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "its action ran", By: api.Worker}})
					return "", "", err
				}})
			})
		})
	mine := put(t, ctx, svc, api.Todo{Module: "billing", Title: "react"})
	if _, err := svc.Run(ctx, api.RunInput{ID: mine.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ran, []string{mine.ID}) {
		t.Errorf("the owner's runner ran %v; want %s", ran, mine.ID)
	}
	if runs, _ := tasks.work(); len(runs) != 0 {
		t.Errorf("work-todo runs %v; an owner's runner needs none", runs)
	}
	if !slices.Contains(got.seen(), mine.ID) {
		t.Errorf("collected %v; want %s reported by its runner and collected", got.seen(), mine.ID)
	}

	other := put(t, ctx, svc, api.Todo{Module: "billing", Title: "implement", Owner: "someone-else"})
	if _, err := svc.Run(ctx, api.RunInput{ID: other.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if runs, _ := tasks.work(); len(runs) != 1 {
		t.Errorf("work-todo runs %v; an owner with no runner gets one", runs)
	}
}

// A failed to-do with attempts left is ready again after its backoff — not
// pulled before, its owner not called; past its limit it is collected failed.
// A blocked one is never retried.
func TestAFailedToDoIsRetriedAfterItsBackoff(t *testing.T) {
	ctx, _, svc, got := world(t)
	td := put(t, ctx, svc, api.Todo{Module: "billing", Title: "react", MaxAttempts: 2, BackoffSeconds: 1})
	fail := func(want string) api.TodoOutput {
		t.Helper()
		p, err := svc.Pull(ctx, api.PullInput{ID: td.ID, By: alice})
		if err != nil || p.Todo == nil {
			t.Fatalf("pull %s: %+v %v; want it", want, p, err)
		}
		out, err := svc.Report(ctx, api.ReportInput{ID: td.ID, Result: api.Result{Status: api.StatusFail, Summary: "it failed", By: alice}})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	out := fail("the first attempt")
	if out.Todo.State != api.StateReady || out.Todo.Attempts != 1 || out.Todo.NotBefore == "" {
		t.Fatalf("after a failure with attempts left: %+v; want ready again, 1 attempt, a not_before", out.Todo)
	}
	if len(got.seen()) != 0 {
		t.Errorf("collected %v; its owner is not called before its last attempt", got.seen())
	}
	if p, _ := svc.Pull(ctx, api.PullInput{ID: td.ID, By: alice}); p.Todo != nil || len(p.Waiting) != 1 || !strings.Contains(p.Waiting[0], "retried after") {
		t.Fatalf("pulled before its backoff: %+v; want it waiting until then", p)
	}
	time.Sleep(1100 * time.Millisecond)

	out = fail("after its backoff")
	if out.Todo.State != api.StateReported || out.Todo.Attempts != 2 || !slices.Contains(got.seen(), td.ID) {
		t.Errorf("past its limit: %+v, collected %v; want reported failed and collected", out.Todo, got.seen())
	}

	blocked := put(t, ctx, svc, api.Todo{Module: "shipping", Title: "react", MaxAttempts: 3})
	if _, err := svc.Pull(ctx, api.PullInput{ID: blocked.ID, By: bob}); err != nil {
		t.Fatal(err)
	}
	b, err := svc.Report(ctx, api.ReportInput{ID: blocked.ID, Result: api.Result{Status: api.StatusBlocked, Summary: "needs a person", By: bob}})
	if err != nil || b.Todo.State != api.StateReported {
		t.Errorf("blocked: %+v %v; want reported, never retried", b.Todo, err)
	}
}

// A Put with a key an open to-do of the module carries returns that to-do
// unchanged; another key makes another; once the first is archived, its key
// makes a new one.
func TestAKeyedPutIsPutOnce(t *testing.T) {
	ctx, _, svc, _ := world(t)
	first := put(t, ctx, svc, api.Todo{Module: "billing", Title: "react", Key: "sub-1/event-7"})
	again := put(t, ctx, svc, api.Todo{Module: "billing", Title: "react again", Key: "sub-1/event-7"})
	if again.ID != first.ID || again.Title != first.Title {
		t.Errorf("a replayed put: %s %q; want %s unchanged", again.ID, again.Title, first.ID)
	}
	if other := put(t, ctx, svc, api.Todo{Module: "billing", Title: "react", Key: "sub-1/event-8"}); other.ID == first.ID {
		t.Error("another key gave the same to-do")
	}
	list, _ := svc.List(ctx, api.ListInput{Module: "billing"})
	if len(list.Todos) != 2 {
		t.Errorf("billing holds %d to-dos; want 2", len(list.Todos))
	}

	if _, err := svc.Pull(ctx, api.PullInput{ID: first.ID, By: alice}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Report(ctx, api.ReportInput{ID: first.ID, Result: api.Result{Status: api.StatusSuccess, Summary: "done", By: alice}}); err != nil {
		t.Fatal(err)
	}
	if later := put(t, ctx, svc, api.Todo{Module: "billing", Title: "react", Key: "sub-1/event-7"}); later.ID == first.ID {
		t.Error("an archived to-do's key still answered: an ended to-do holds no key")
	}
}
