// SPDX-License-Identifier: Apache-2.0

package messagebus

// How a slot's requests are delivered over a real embedded NATS: served
// concurrently by a bounded pool, so a handler may call back into its own
// slot; one at a time, in arrival order, for a slot tagged ordered.

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
)

// TestRealNATS_ReentrantCallAnswers — a handler that calls its own slot
// while it serves a request (CWM evaluating a guard whose function reads
// the object back from CWM) gets its answer. Served inline, the inner call
// waits behind the outer one until it times out.
func TestRealNATS_ReentrantCallAnswers(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)
	sender := NewSender(bus, testTenant, testUniverse)

	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("loop", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("inner", ktsp.MethodOpts{},
		func(context.Context, json.RawMessage) (json.RawMessage, error) {
			return json.Marshal("inner answered")
		}))
	slot.Add(ktsp.RawMethod("outer", ktsp.MethodOpts{},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return sender.Call(cctx, testProvider+":/loop/inner", nil)
		}))
	ep.AddSlot(slot)

	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := sender.Call(ctx, testProvider+":/loop/outer", nil)
	if err != nil {
		t.Fatalf("re-entrant call: %v", err)
	}
	var got string
	if err := json.Unmarshal(out, &got); err != nil || got != "inner answered" {
		t.Fatalf("re-entrant call answered %s (%v), want \"inner answered\"", out, err)
	}
}

// TestRealNATS_ConcurrencyIsBounded — no more than the bound are served at
// once; the rest wait their turn and are all answered.
func TestRealNATS_ConcurrencyIsBounded(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  []SubscribeOption
		tags  ktsp.SlotTags
		bound int64
	}{
		{name: "option", opts: []SubscribeOption{WithMaxConcurrent(3)}, bound: 3},
		{name: "slot tag", tags: ktsp.SlotTags{ktsp.TagSlotMaxConcurrent: "2"}, bound: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startEmbeddedHub(t)
			bus := natsxport.New(h.Client)

			var active, peak int64
			release := make(chan struct{})
			ep := ktsp.NewEndpoint("test", "", "")
			slot := ktsp.NewSlot("pool", ktsp.DefaultInstance, "", tc.tags)
			slot.Add(ktsp.RawMethod("hold", ktsp.MethodOpts{},
				func(context.Context, json.RawMessage) (json.RawMessage, error) {
					n := atomic.AddInt64(&active, 1)
					for {
						p := atomic.LoadInt64(&peak)
						if n <= p || atomic.CompareAndSwapInt64(&peak, p, n) {
							break
						}
					}
					<-release
					atomic.AddInt64(&active, -1)
					return json.Marshal("ok")
				}))
			ep.AddSlot(slot)
			group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep, tc.opts...)
			if err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			defer group.Unsubscribe()

			sender := NewSender(bus, testTenant, testUniverse)
			const calls = 8
			errs := make(chan error, calls)
			var wg sync.WaitGroup
			for i := 0; i < calls; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					_, err := sender.Call(ctx, testProvider+":/pool/hold", nil)
					errs <- err
				}()
			}
			// The pool fills up to its bound and no further.
			deadline := time.Now().Add(5 * time.Second)
			for atomic.LoadInt64(&active) < tc.bound && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			time.Sleep(100 * time.Millisecond)
			if got := atomic.LoadInt64(&active); got != tc.bound {
				t.Fatalf("%d requests served at once, want the bound %d", got, tc.bound)
			}
			close(release)
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatalf("call: %v", err)
				}
			}
			if p := atomic.LoadInt64(&peak); p != tc.bound {
				t.Fatalf("peak %d requests at once, want %d", p, tc.bound)
			}
		})
	}
}

// recorder is a slot method that records the order it serves requests in
// and whether two ever overlapped. Earlier requests take longer, so served
// concurrently they finish out of order.
type recorder struct {
	n       int
	mu      sync.Mutex
	seen    []int
	active  int64
	overlap int64
	done    chan struct{}
}

func newRecorder(n int) *recorder { return &recorder{n: n, done: make(chan struct{})} }

func (r *recorder) method() *ktsp.MethodDef {
	return ktsp.RawMethod("append", ktsp.MethodOpts{},
		func(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
			if atomic.AddInt64(&r.active, 1) > 1 {
				atomic.StoreInt64(&r.overlap, 1)
			}
			defer atomic.AddInt64(&r.active, -1)
			var in struct{ N int }
			_ = json.Unmarshal(raw, &in)
			if in.N < 0 { // the request that holds the slot while the others queue
				time.Sleep(300 * time.Millisecond)
				return nil, nil
			}
			time.Sleep(time.Duration(r.n-in.N) * 200 * time.Microsecond)
			r.mu.Lock()
			r.seen = append(r.seen, in.N)
			if len(r.seen) == r.n {
				close(r.done)
			}
			r.mu.Unlock()
			return nil, nil
		})
}

func (r *recorder) check(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(10 * time.Second):
		t.Fatal("not every request was served")
	}
	if atomic.LoadInt64(&r.overlap) != 0 {
		t.Fatal("two requests were served at once")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, v := range r.seen {
		if v != i {
			t.Fatalf("served out of order: %s", fmt.Sprint(r.seen))
		}
	}
}

func subscribeRecorder(t *testing.T, tags ktsp.SlotTags, n int) (*recorder, *Sender, mbapi.MessageBus) {
	t.Helper()
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)
	r := newRecorder(n)
	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("log", ktsp.DefaultInstance, "", tags)
	slot.Add(r.method())
	ep.AddSlot(slot)
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, ep)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(func() { _ = group.Unsubscribe() })
	return r, NewSender(bus, testTenant, testUniverse), bus
}

// TestRealNATS_OrderedSlotServesInArrivalOrder — a slot tagged ordered
// serves its requests one at a time, in the order they arrived: while one
// request holds the slot, the next ones queue, and are then served in turn.
//
// The arrival order is the test's own, never left to the goroutine
// scheduler: each call waits on its own goroutine for its reply, but the
// next one leaves only once the hub has received the previous one — a
// watcher subscribed to the slot's subject on the same connection sees it
// (NATS delivers one connection's publishes to every subscriber in the
// order it received them).
func TestRealNATS_OrderedSlotServesInArrivalOrder(t *testing.T) {
	const n = 20
	r, sender, bus := subscribeRecorder(t, ktsp.SlotTags{ktsp.TagSlotOrdered: "true"}, n)
	arrived := make(chan int, n+1)
	watch, err := bus.Subscribe(context.Background(),
		subjects.SlotSubject(testTenant, testUniverse, testProvider, "log", ktsp.DefaultInstance, "append"),
		func(_ context.Context, msg mbapi.Message) {
			var in struct{ N int }
			_ = json.Unmarshal(msg.Data(), &in)
			arrived <- in.N
		})
	if err != nil {
		t.Fatalf("Subscribe the watcher: %v", err)
	}
	t.Cleanup(func() { _ = watch.Unsubscribe() })
	call := func(i int) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := sender.Call(ctx, testProvider+":/log/append", map[string]int{"N": i}); err != nil {
			t.Errorf("Call %d: %v", i, err)
		}
	}
	var wg sync.WaitGroup
	for i := -1; i < n; i++ { // -1 holds the slot while the others queue
		wg.Add(1)
		go func(i int) { defer wg.Done(); call(i) }(i)
		select {
		case got := <-arrived:
			if got != i {
				t.Fatalf("the hub received request %d, want %d", got, i)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("request %d never reached the hub", i)
		}
	}
	wg.Wait()
	r.check(t)
}

// TestRealNATS_NotificationsKeepTheirOrder — on a slot served
// concurrently, notifications (nobody waits for an answer) are still
// served one at a time, in the order they were sent.
func TestRealNATS_NotificationsKeepTheirOrder(t *testing.T) {
	const n = 40
	r, sender, _ := subscribeRecorder(t, nil, n)
	for i := 0; i < n; i++ {
		if err := sender.Notify(context.Background(), testProvider+":/log/append", map[string]int{"N": i}); err != nil {
			t.Fatalf("Notify %d: %v", i, err)
		}
	}
	r.check(t)
}
