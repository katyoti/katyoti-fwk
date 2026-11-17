// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	schedcore "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/internal/core"
)

// THE QUEUE NAMES WHO WOULD TAKE A TO-DO — each pull that
// is not by id is recorded; on each takeable to-do the queue names the
// recorded pulls whose filter would take it: one open now (waiting), one ended
// within the default lease, an older one only while its session holds a
// to-do. A pull whose filter would not take it, or a pull by id, is never a
// taker.
func TestAPullIsRecordedAndNamedAsTakerOfWhatItWouldTake(t *testing.T) {
	ctx, _, svc, _ := world(t)
	rec, err := application.UseService[*schedcore.PullRecord](ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	clock := time.Now().UTC()
	rec.SetClock(func() time.Time { mu.Lock(); defer mu.Unlock(); return clock })
	later := func(d time.Duration) { mu.Lock(); clock = clock.Add(d); mu.Unlock() }

	label := []string{"increment:IN-1"}
	other := []string{"increment:IN-2"}
	// A comes first in the queue, given to no one. S writes A's module too,
	// is given to bob, and comes first for a pull that prefers shipping: a
	// pull of alice's that prefers shipping finds A reserved behind S and S
	// not hers - it waits, while the queue holds A takeable.
	a := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-A", Title: "implement A", Writes: []string{"billing"}, Labels: label})
	bobs := bob
	s := put(t, ctx, svc, api.Todo{Module: "shipping", Request: "shipping.RQ-S", Title: "implement S", Writes: []string{"billing"}, Labels: label, Assignee: &bobs})
	o := put(t, ctx, svc, api.Todo{Module: "shipping", Request: "shipping.RQ-O", Title: "implement O", Writes: []string{"shipping"}, Labels: other})
	o2 := put(t, ctx, svc, api.Todo{Module: "billing", Request: "billing.RQ-P", Title: "analyse P", Labels: other, Priority: "P3"})

	from := func(session string) context.Context {
		return cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code", Session: session})
	}
	waitCtx, stop := context.WithCancel(from("s-alice"))
	defer stop()
	done := make(chan api.PullOutput, 1)
	go func() {
		out, _ := svc.Pull(waitCtx, api.PullInput{By: alice, Filter: api.Filter{Labels: label}, Prefer: "shipping", Wait: "30s"})
		done <- out
	}()

	takers := func(id string) ([]api.Taker, string) {
		t.Helper()
		q, err := svc.Queue(ctx, api.Filter{})
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range q.Queue {
			if e.Todo.ID == id {
				return e.Takers, q.TakersSince
			}
		}
		return nil, q.TakersSince
	}
	named := func(id string, by cwmapi.Actor) *api.Taker {
		t.Helper()
		got, _ := takers(id)
		for _, tk := range got {
			if tk.By == by {
				return &tk
			}
		}
		return nil
	}

	// While it waits, alice is a taker of A - waiting - and of nothing else.
	deadline := time.Now().Add(10 * time.Second)
	for named(a.ID, alice) == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	tk := named(a.ID, alice)
	if tk == nil || !tk.Waiting || tk.Session != "s-alice" || tk.Holds != "" {
		got, _ := takers(a.ID)
		t.Fatalf("A's takers while alice's pull waits: %+v; want alice waiting from s-alice", got)
	}
	if _, since := takers(a.ID); since == "" {
		t.Error("the queue names no start of its pull record (TakersSince)")
	}
	if got := named(s.ID, alice); got != nil {
		t.Errorf("alice named a taker of S, given to bob: %+v", got)
	}
	if got := named(o.ID, alice); got != nil {
		t.Errorf("alice named a taker of O, another label: %+v", got)
	}

	// A pull by id is never recorded: carol takes P by id; O, which an
	// unfiltered pull of hers would take, does not name her.
	carol := cwmapi.Actor{ID: "session-c", Kind: "agent"}
	if got, err := svc.Pull(from("s-carol"), api.PullInput{ID: o2.ID, By: carol}); err != nil || got.Todo == nil {
		t.Fatalf("carol pulls P by id: %+v, %v", got, err)
	}
	if got := named(o.ID, carol); got != nil {
		t.Errorf("a pull by id named a taker: %+v", got)
	}

	// The wait ends: alice is still a taker, no longer waiting, for the
	// default lease.
	stop()
	select {
	case out := <-done:
		if out.Todo != nil {
			t.Fatalf("alice's wait took %s; want none", out.Todo.ID)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("alice's wait did not end once stopped")
	}
	if tk := named(a.ID, alice); tk == nil || tk.Waiting || tk.At == "" {
		t.Fatalf("alice after her pull ended: %+v; want a taker since %s, not waiting", tk, "its end")
	}

	// Past the default lease, with nothing held, she is no taker.
	later(16 * time.Minute)
	if tk := named(a.ID, alice); tk != nil {
		t.Fatalf("alice 16 min after her pull, holding nothing: %+v; want no taker", tk)
	}

	// Her session holds a to-do: she is a taker again, naming it.
	if got, err := svc.Pull(from("s-alice"), api.PullInput{ID: o.ID, By: alice}); err != nil || got.Todo == nil {
		t.Fatalf("alice pulls O by id: %+v, %v", got, err)
	}
	if tk := named(a.ID, alice); tk == nil || tk.Holds != o.ID || tk.Waiting {
		t.Fatalf("alice holding O: %+v; want a taker of A holding %s", tk, o.ID)
	}
}
