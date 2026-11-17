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
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	eventsapi "github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	eventslocal "github.com/katyoti/katyoti-fwk/internal/extensions/events/local"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedlocal "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// REACTIONS ON THE SCHEDULER, end to end on the real CWM
// scheduler and reactions extensions: an `item` updated matches its async
// subscription (x:react/mark); the reaction to-do it becomes is run — by the
// internal worker or an outside one — under the reaction's cause; its action
// writes the module's `note`, whose own async subscription (x:react/noted,
// accepting caused writes) is the downstream reaction: staged in the outbox
// while the reaction has not succeeded, released once when it has.

const (
	markURI  = "x:react/mark"
	notedURI = "x:react/noted"
)

var pack = fstest.MapFS{
	"object-types/module.yaml": {Data: []byte("name: module\ncapabilities: [module]\n")},
	"object-types/item.yaml": {Data: []byte(`name: item
hooks:
  on_update:
    - uri: x:react/mark
      async: {}
`)},
	"object-types/noted.yaml": {Data: []byte(`name: noted
hooks:
  on_create:
    - uri: x:react/noted
      async: {accepts_caused: true}
  on_update:
    - uri: x:react/noted
      async: {accepts_caused: true}
`)},
}

// actions is the ActionService the reactions run through: x:react/mark writes
// the module's note (create, then update) and fails while fail > 0; it records
// the cause each call ran under.
type actions struct {
	mu     sync.Mutex
	fail   int
	calls  int
	causes []cause.Cause
}

func (a *actions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (a *actions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	a.mu.Lock()
	a.calls++
	c, _ := cause.CauseFrom(ctx)
	a.causes = append(a.causes, c)
	failing := a.fail > 0
	if failing {
		a.fail--
	}
	a.mu.Unlock()
	if in.URI == markURI {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return actionsapi.RunOutput{}, err
		}
		note := cwmapi.NewContainerObject("m", "note", "noted", "m", "Note", "")
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: note}); err != nil {
			got, gerr := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.note"})
			if gerr != nil {
				return actionsapi.RunOutput{}, err
			}
			o := got.Object
			o.Name = "Note " + time.Now().Format(time.RFC3339Nano)
			if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
				return actionsapi.RunOutput{}, err
			}
		}
		if failing {
			return actionsapi.RunOutput{}, errors.New("the mark failed after writing the note")
		}
	}
	return actionsapi.RunOutput{URI: in.URI, Value: json.RawMessage(`{"ok":true}`)}, nil
}

type world struct {
	ctx   context.Context
	cwm   cwmapi.CWMService
	todos schedapi.TodoService
	react eventsapi.ReactionService
	acts  *actions
}

func newWorld(t *testing.T, fail int) *world {
	t.Helper()
	acts := &actions{fail: fail}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(pack),
		schedlocal.Extension(),
		eventslocal.ReactionsExtensionWith(eventslocal.ReactionsOptions{BackoffSeconds: 1}),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	item := cwmapi.NewContainerObject("m", "item", "item", "m", "Item", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: item}); err != nil {
		t.Fatal(err)
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	react, err := application.UseService[eventsapi.ReactionService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	return &world{ctx: ctx, cwm: cwm, todos: todos, react: react, acts: acts}
}

// touch updates the item: a write with no cause, which matches x:react/mark.
func (w *world) touch(t *testing.T) {
	t.Helper()
	got, err := w.cwm.ObjectGet(w.ctx, cwmapi.ObjectGetInput{Module: "m", ID: "m.item"})
	if err != nil {
		t.Fatal(err)
	}
	o := got.Object
	o.Name = "Item " + time.Now().Format(time.RFC3339Nano)
	if _, err := w.cwm.ObjectUpdate(w.ctx, cwmapi.ObjectUpdateInput{Module: "m", Object: o}); err != nil {
		t.Fatal(err)
	}
}

// reactions is the open reaction to-dos of uri.
func (w *world) reactions(t *testing.T, uri string) []schedapi.Todo {
	t.Helper()
	list, err := w.todos.List(w.ctx, schedapi.ListInput{Module: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var out []schedapi.Todo
	for _, td := range list.Todos {
		if td.Owner == eventsapi.Owner && td.Work[eventsapi.WorkURI] == uri {
			out = append(out, td)
		}
	}
	return out
}

// waitFor polls until n open reactions of uri exist (matches are handed over in
// the background); it fails past the deadline.
func (w *world) waitFor(t *testing.T, uri string, n int) []schedapi.Todo {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := w.reactions(t, uri)
		if len(got) == n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d open reaction(s) of %s, want %d", len(got), uri, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// stays fails if open reactions of uri ever differ from n within d.
func (w *world) stays(t *testing.T, uri string, n int, d time.Duration) {
	t.Helper()
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		if got := w.reactions(t, uri); len(got) != n {
			t.Fatalf("%d open reaction(s) of %s, want %d to stay", len(got), uri, n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (w *world) done(t *testing.T, id string) schedapi.Todo {
	t.Helper()
	list, err := w.todos.List(w.ctx, schedapi.ListInput{Module: "m", Done: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range list.Todos {
		if td.ID == id {
			return td
		}
	}
	t.Fatalf("%s is not archived", id)
	return schedapi.Todo{}
}

// A change makes a reaction to-do of its module — one per match, keyed —
// worked to done by the internal worker under the reaction's cause; its writes'
// downstream reaction is put once it succeeded.
func TestAChangeIsAReactionWorkedByTheInternalWorker(t *testing.T) {
	w := newWorld(t, 0)
	w.touch(t)
	td := w.waitFor(t, markURI, 1)[0]
	if td.Key == "" || td.Target != "m.item" || td.Owner != eventsapi.Owner {
		t.Fatalf("the reaction to-do: key %q target %q owner %q; want keyed, on the item, owned by events", td.Key, td.Target, td.Owner)
	}
	if _, err := w.todos.Run(w.ctx, schedapi.RunInput{ID: td.ID, By: cwmapi.Actor{ID: "tester", Kind: "person"}}); err != nil {
		t.Fatal(err)
	}
	if d := w.done(t, td.ID); d.Result == nil || d.Result.Status != schedapi.StatusSuccess {
		t.Fatalf("the reaction ended %+v; want done, success", d.Result)
	}
	if c := w.acts.causes[0]; c.Kind != cause.KindReaction || c.ID != td.Key || c.Depth != 1 {
		t.Fatalf("its action ran under %+v; want the reaction's cause {reaction %s 1}", c, td.Key)
	}
	w.waitFor(t, notedURI, 1) // its write's downstream reaction, released
}

// The same reaction pulled and run by an outside worker: `reactions run` on a
// to-do it holds — refused on one it does not.
func TestAReactionWorkedByAnOutsideWorker(t *testing.T) {
	w := newWorld(t, 0)
	w.touch(t)
	td := w.waitFor(t, markURI, 1)[0]
	outside := cwmapi.Actor{ID: "outside-session", Kind: "agent"}
	if _, err := w.react.Run(w.ctx, eventsapi.ReactionRunInput{Todo: td.ID, By: outside}); err == nil {
		t.Fatal("ran a reaction the caller does not hold")
	}
	if p, err := w.todos.Pull(w.ctx, schedapi.PullInput{ID: td.ID, By: outside}); err != nil || p.Todo == nil {
		t.Fatalf("pull: %+v %v", p, err)
	}
	out, err := w.react.Run(w.ctx, eventsapi.ReactionRunInput{Todo: td.ID, By: outside})
	if err != nil || out.Status != schedapi.StatusSuccess {
		t.Fatalf("reactions run: %+v %v", out, err)
	}
	if d := w.done(t, td.ID); d.Result.By != outside {
		t.Fatalf("reported by %+v; want the outside worker", d.Result.By)
	}
	w.waitFor(t, notedURI, 1)
}

// A reaction failing twice then succeeding (attempts 3): each failure puts it
// back after its backoff and emits nothing — the note it wrote each time (a
// create, then two updates) stays staged — and its success releases what they
// triggered once: one reaction per subscription of the note (on_create,
// on_update), the three writes coalesced, never one per attempt.
func TestAFailingReactionEmitsNothingAndReruns(t *testing.T) {
	w := newWorld(t, 2)
	w.touch(t)
	td := w.waitFor(t, markURI, 1)[0]
	run := func(want string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for {
			_, err := w.todos.Run(w.ctx, schedapi.RunInput{ID: td.ID, By: cwmapi.Actor{ID: "tester", Kind: "person"}})
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: %v", want, err)
			}
			time.Sleep(100 * time.Millisecond) // its backoff
		}
	}
	for i := 1; i <= 2; i++ {
		run("a failing attempt")
		got, err := w.todos.Get(w.ctx, schedapi.GetInput{ID: td.ID})
		if err != nil || got.Todo.Attempts != i || got.Todo.State != schedapi.StateReady {
			t.Fatalf("after failure %d: %+v %v; want ready again, %d attempt(s)", i, got.Todo, err, i)
		}
		w.stays(t, notedURI, 0, 300*time.Millisecond)
	}
	run("the succeeding attempt")
	if d := w.done(t, td.ID); d.Result.Status != schedapi.StatusSuccess {
		t.Fatalf("ended %+v; want success", d.Result)
	}
	got := w.waitFor(t, notedURI, 2)
	w.stays(t, notedURI, 2, 500*time.Millisecond)
	subs := map[string]bool{}
	for _, td := range got {
		m, _ := td.Work[eventsapi.WorkMatch].(map[string]any)
		sub, _ := m["subscription"].(map[string]any)
		subs[sub["id"].(string)] = true
	}
	if len(subs) != 2 {
		t.Fatalf("the downstream reactions are of %d subscription(s), want 2 (on_create, on_update): %v", len(subs), subs)
	}
}

// A reaction failing past its attempts is given up: reported, its outbox held,
// listed with its result; `reactions retry` puts it back with its attempts
// counted again; `reactions drop` archives it and answers the writes it made
// without their events — and nothing downstream ever ran.
func TestAReactionGivenUpIsRetriedOrDropped(t *testing.T) {
	w := newWorld(t, 100)
	w.touch(t)
	td := w.waitFor(t, markURI, 1)[0]
	exhaust := func() {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for {
			got, err := w.todos.Get(w.ctx, schedapi.GetInput{ID: td.ID})
			if err != nil {
				t.Fatal(err)
			}
			if got.Todo.State == schedapi.StateReported {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("not given up: %+v", got.Todo)
			}
			_, _ = w.todos.Run(w.ctx, schedapi.RunInput{ID: td.ID, By: cwmapi.Actor{ID: "tester", Kind: "person"}})
			time.Sleep(100 * time.Millisecond)
		}
	}
	person := cwmapi.Actor{ID: "jdoe", Kind: "person"}

	exhaust()
	list, err := w.react.List(w.ctx, eventsapi.ReactionListInput{Module: "m"})
	if err != nil || len(list.Reactions) != 1 || list.Reactions[0].State != schedapi.StateReported || list.Reactions[0].Result == "" {
		t.Fatalf("list: %+v %v; want the reaction given up, with its result", list, err)
	}
	if _, err := w.react.Retry(w.ctx, eventsapi.ReactionRetryInput{Todo: td.ID, By: person}); err != nil {
		t.Fatal(err)
	}
	if got, _ := w.todos.Get(w.ctx, schedapi.GetInput{ID: td.ID}); got.Todo.State != schedapi.StateReady || got.Todo.Attempts != 0 {
		t.Fatalf("after retry: %+v; want ready, attempts 0", got.Todo)
	}

	exhaust()
	dropped, err := w.react.Drop(w.ctx, eventsapi.ReactionDropInput{Todo: td.ID, By: person, Why: "the mark cannot be written"})
	if err != nil || len(dropped.Writes) == 0 {
		t.Fatalf("drop: %+v %v; want the note's writes listed", dropped, err)
	}
	if d := w.done(t, td.ID); d.Note == "" {
		t.Fatal("dropped without a note")
	}
	w.stays(t, notedURI, 0, 500*time.Millisecond)
}
