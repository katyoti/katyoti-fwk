// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventslocal "github.com/katyoti/katyoti-fwk/internal/extensions/events/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// AN ATOMIC REACTION: its subscription declares
// `atomic: true`, so the runner opens the reaction's change set before the
// action, commits it before reporting success, discards it on a failed attempt.

var atomicPack = fstest.MapFS{
	"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")},
	"object-types/node.yaml":   {Data: []byte("name: node\n")},
	"object-types/batch.yaml": {Data: []byte(`name: batch
hooks:
  on_update:
    - uri: x:batch/write
      async: {atomic: true}
`)},
}

// batchActions writes m.a and m.b (or rewrites m.target) under the reaction's
// cause; it fails while fail > 0 — after writing; meddle, once, rewrites the
// target with no cause between the reaction's read and its return.
type batchActions struct {
	mu       sync.Mutex
	fail     int
	conflict bool
	meddled  bool
}

func (a *batchActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a *batchActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	a.mu.Lock()
	failing := a.fail > 0
	if failing {
		a.fail--
	}
	meddle := a.conflict && !a.meddled
	a.meddled = a.meddled || meddle
	a.mu.Unlock()
	if a.conflict {
		got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.target"})
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		o := got.Object
		o.Description = "set by the reaction"
		if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o, Revision: got.Revision}); err != nil {
			return actionsapi.RunOutput{}, err
		}
		if meddle { // someone else writes the target meanwhile: no cause, written at once
			plain := context.WithoutCancel(application.WithApplication(context.Background(), application.AppFromContext(ctx)))
			cur, err := cwm.ObjectGet(plain, cwmapi.ObjectGetInput{Module: "m", ID: "m.target"})
			if err != nil {
				return actionsapi.RunOutput{}, err
			}
			p := cur.Object
			p.Name = "Target, renamed by a person"
			if _, err := cwm.ObjectUpdate(plain, cwmapi.ObjectUpdateInput{Module: "m", Object: p}); err != nil {
				return actionsapi.RunOutput{}, err
			}
		}
		return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{}`)}, nil
	}
	for _, id := range []string{"a", "b"} {
		o := cwmapi.NewContainerObject("m", id, "node", "m", "Node "+id, "")
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			return actionsapi.RunOutput{}, err
		}
	}
	if failing {
		return actionsapi.RunOutput{}, errors.New("the batch failed after writing both")
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{}`)}, nil
}

type atomicWorld struct {
	ctx   context.Context
	cwm   cwmapi.CWMService
	todos schedapi.TodoService
}

func newAtomicWorld(t *testing.T, acts *batchActions) *atomicWorld {
	t.Helper()
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(atomicPack),
		schedlocal.Extension(),
		eventslocal.ReactionsExtensionWith(eventslocal.ReactionsOptions{BackoffSeconds: 1}),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	for _, o := range []cwmapi.Object{
		cwmapi.NewContainerObject("m", "batch", "batch", "m", "Batch", ""),
		cwmapi.NewContainerObject("m", "target", "node", "m", "Target", ""),
	} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: o}); err != nil {
			t.Fatal(err)
		}
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &atomicWorld{ctx: ctx, cwm: cwm, todos: todos}
}

// trigger updates the batch and answers its reaction to-do.
func (w *atomicWorld) trigger(t *testing.T) string {
	t.Helper()
	got, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.batch"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Name = "Batch, changed"
	if _, err := w.cwm.ObjectUpdate(w.ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		list, err := w.todos.List(w.ctx, schedapi.ListInput{Module: "m"})
		if err != nil {
			t.Fatal(err)
		}
		for _, td := range list.Todos {
			if td.Owner == eventsapi.Owner {
				return td.ID
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no reaction to-do")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// attempt runs the reaction once it may be pulled (after its backoff).
func (w *atomicWorld) attempt(t *testing.T, id string) schedapi.Todo {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := w.todos.Run(w.ctx, schedapi.RunInput{ID: id, By: cwmapi.Actor{ID: "tester", Kind: "person"}}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the reaction was never runnable")
		}
		time.Sleep(100 * time.Millisecond)
	}
	got, err := w.todos.Get(w.ctx, schedapi.GetInput{ID: id})
	if err == nil {
		return got.Todo
	}
	list, _ := w.todos.List(w.ctx, schedapi.ListInput{Module: "m", Done: true})
	for _, td := range list.Todos {
		if td.ID == id {
			return td
		}
	}
	t.Fatalf("%s is gone", id)
	return schedapi.Todo{}
}

func (w *atomicWorld) exists(id string) bool {
	_, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: id})
	return err == nil
}

// An atomic reaction writing two objects then failing leaves neither written;
// its next attempt, succeeding, writes both at once.
func TestAnAtomicReactionThatFailsWritesNothing(t *testing.T) {
	w := newAtomicWorld(t, &batchActions{fail: 1})
	id := w.trigger(t)
	td := w.attempt(t, id)
	if td.Attempts != 1 || td.State != schedapi.StateReady {
		t.Fatalf("after the failed attempt: %+v; want ready again, 1 attempt", td)
	}
	if w.exists("m.a") || w.exists("m.b") {
		t.Fatalf("a failed atomic attempt left writes: a %v, b %v", w.exists("m.a"), w.exists("m.b"))
	}
	td = w.attempt(t, id)
	if td.Result == nil || td.Result.Status != schedapi.StatusSuccess {
		t.Fatalf("the second attempt: %+v; want success", td.Result)
	}
	if !w.exists("m.a") || !w.exists("m.b") {
		t.Fatalf("the succeeding attempt's writes: a %v, b %v; want both", w.exists("m.a"), w.exists("m.b"))
	}
}

// A revision moved since the reaction read it: the commit conflicts, the
// attempt fails and writes nothing; the next attempt commits on the current
// state — the person's rename kept, the reaction's description written.
func TestAConflictAtCommitRerunsOnTheCurrentState(t *testing.T) {
	w := newAtomicWorld(t, &batchActions{conflict: true})
	id := w.trigger(t)
	td := w.attempt(t, id)
	if td.Attempts != 1 || td.State != schedapi.StateReady || td.Result != nil && td.Result.Status == schedapi.StatusSuccess {
		t.Fatalf("after the conflicting attempt: %+v; want failed, ready again", td)
	}
	got, _ := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.target"})
	if got.Object.Description == "set by the reaction" {
		t.Fatal("a conflicting commit wrote the reaction's change")
	}
	td = w.attempt(t, id)
	if td.Result == nil || td.Result.Status != schedapi.StatusSuccess {
		t.Fatalf("the second attempt: %+v; want success", td.Result)
	}
	got, _ = w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.target"})
	if got.Object.Description != "set by the reaction" || got.Object.Name != "Target, renamed by a person" {
		t.Fatalf("after the rerun: name %q description %q; want the person's rename kept and the reaction's description", got.Object.Name, got.Object.Description)
	}
}
