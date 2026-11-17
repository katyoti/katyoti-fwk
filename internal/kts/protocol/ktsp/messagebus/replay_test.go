// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	teststub "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// A CALL SENT AGAIN LANDS ONCE (criteria to
// AC-007).

// spyBus is a bus that records the request id of every request, and loses the
// answer of the first request to a slot subject when loseFirst is set: the
// request is delivered (its method runs), its caller is told no one answered -
// the « Slot not found » after the write landed.
type spyBus struct {
	mbapi.MessageBus
	mu        sync.Mutex
	ids       []string
	loseFirst string // a subject suffix whose first answer is lost
	lost      bool
}

func (b *spyBus) Request(ctx context.Context, subject string, payload []byte, opts ...mbapi.ReqOpt) ([]byte, error) {
	h := mbapi.ExtractReqOpts(opts).Header
	b.mu.Lock()
	if v := h[HeaderRequestID]; len(v) > 0 && strings.HasSuffix(subject, ".write") {
		b.ids = append(b.ids, v[0])
	}
	lose := b.loseFirst != "" && strings.HasSuffix(subject, b.loseFirst) && !b.lost
	if lose {
		b.lost = true
	}
	b.mu.Unlock()
	out, err := b.MessageBus.Request(ctx, subject, payload, opts...)
	if lose && err == nil {
		return nil, mbapi.ErrNoResponders{Subject: subject}
	}
	return out, err
}

func (b *spyBus) requestIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.ids...)
}

// counting serves counter/write (counts its runs, answers the count) and
// counter/slow (waits for release, then counts).
type counting struct {
	runs    atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (c *counting) endpoint(provider string) *ktsp.Endpoint {
	ep := ktsp.NewEndpoint(provider, "", "")
	slot := ktsp.NewSlot("counter", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("write", ktsp.MethodOpts{}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		n := c.runs.Add(1)
		return json.Marshal(n)
	}))
	slot.Add(ktsp.RawMethod("slow", ktsp.MethodOpts{}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		close(c.started)
		<-c.release
		n := c.runs.Add(1)
		return json.Marshal(n)
	}))
	ep.AddSlot(slot)
	return ep
}

func serve(t *testing.T, bus mbapi.MessageBus, provider string, ep *ktsp.Endpoint) {
	t.Helper()
	g, err := Subscribe(context.Background(), bus, testTenant, testUniverse, provider, ep)
	if err != nil {
		t.Fatalf("Subscribe %s: %v", provider, err)
	}
	t.Cleanup(func() { _ = g.Unsubscribe() })
}

// hubEndpoint answers ConnMgrService/EnsureSpawned by starting the target
// (spawn), as the hub's lazy spawn does.
func hubEndpoint(spawn func()) *ktsp.Endpoint {
	ep := ktsp.NewEndpoint("ktw-hub", "", "")
	slot := ktsp.NewSlot("conn-mgr-service", ktsp.DefaultInstance, "", nil)
	var once sync.Once
	slot.Add(ktsp.RawMethod("ensure-spawned", ktsp.MethodOpts{}, func(context.Context, json.RawMessage) (json.RawMessage, error) {
		once.Do(spawn)
		return json.RawMessage(`{}`), nil
	}))
	ep.AddSlot(slot)
	return ep
}

// AC-003: a call keeps one request id across its retry after a lazy spawn; a
// new call gets a new one.
func TestARetriedCallKeepsItsRequestID(t *testing.T) {
	bus := &spyBus{MessageBus: teststub.New(), loseFirst: ".counter.default.write"}
	c := &counting{}
	serve(t, bus, testProvider, c.endpoint(testProvider))
	serve(t, bus, "ktw-hub", hubEndpoint(func() {}))
	sender := NewRetrySender(NewSender(bus, testTenant, testUniverse), "ktw-hub")

	if _, err := sender.Call(context.Background(), testProvider+":/counter/write", nil); err != nil {
		t.Fatalf("the call retried after its lost answer failed: %v", err)
	}
	ids := bus.requestIDs()
	if len(ids) != 2 || ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("the call and its retry carried request ids %v; want one id, sent twice", ids)
	}
	if _, err := sender.Call(context.Background(), testProvider+":/counter/write", nil); err != nil {
		t.Fatal(err)
	}
	if ids = bus.requestIDs(); len(ids) != 3 || ids[2] == ids[0] {
		t.Fatalf("a new call reused the request id: %v", ids)
	}
}

// AC-004: an id already answered is answered from the replay cache; the
// method runs once.
func TestAReplayedRequestIsAnsweredFromTheCacheAndRunsOnce(t *testing.T) {
	bus := teststub.New()
	c := &counting{}
	serve(t, bus, testProvider, c.endpoint(testProvider))
	sender := NewSender(bus, testTenant, testUniverse)

	ctx := WithRequestID(context.Background(), "req-1")
	first, err := sender.Call(ctx, testProvider+":/counter/write", nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := sender.Call(ctx, testProvider+":/counter/write", nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != "1" || string(again) != "1" || c.runs.Load() != 1 {
		t.Fatalf("answers %s then %s, %d runs; want the first answer twice and one run", first, again, c.runs.Load())
	}
	if other, err := sender.Call(WithRequestID(context.Background(), "req-2"), testProvider+":/counter/write", nil); err != nil || string(other) != "2" {
		t.Fatalf("another request: %s, %v; want it run (2)", other, err)
	}
}

// AC-005: a retry arriving while the first run is in flight waits for its
// answer.
func TestARetryWhileTheFirstRunsWaitsForItsAnswer(t *testing.T) {
	bus := teststub.New()
	c := &counting{started: make(chan struct{}), release: make(chan struct{})}
	serve(t, bus, testProvider, c.endpoint(testProvider))
	sender := NewSender(bus, testTenant, testUniverse)
	ctx := WithRequestID(context.Background(), "req-slow")

	type answer struct {
		out json.RawMessage
		err error
	}
	firstc, againc := make(chan answer, 1), make(chan answer, 1)
	go func() { out, err := sender.Call(ctx, testProvider+":/counter/slow", nil); firstc <- answer{out, err} }()
	select {
	case <-c.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the first run never started")
	}
	go func() { out, err := sender.Call(ctx, testProvider+":/counter/slow", nil); againc <- answer{out, err} }()
	time.Sleep(200 * time.Millisecond) // the retry is in, waiting
	close(c.release)
	for _, ch := range []chan answer{firstc, againc} {
		select {
		case a := <-ch:
			if a.err != nil || string(a.out) != "1" {
				t.Fatalf("answer %s, %v; want the first run's (1)", a.out, a.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no answer")
		}
	}
	if n := c.runs.Load(); n != 1 {
		t.Fatalf("the method ran %d times; want once", n)
	}
}

// AC-006: the cache is bounded in time and size; a request with no id runs
// as today.
func TestTheReplayCacheIsBoundedAndACallWithoutAnIDRunsAsToday(t *testing.T) {
	now := time.Unix(0, 0)
	c := newReplayCache()
	c.now = func() time.Time { return now }
	c.max = 2
	for _, k := range []string{"a", "b", "c"} {
		e, owner := c.begin(k)
		if !owner {
			t.Fatalf("%s: a new id is not run", k)
		}
		c.finish(e, []byte(k))
		now = now.Add(time.Second)
	}
	if len(c.entries) > c.max {
		t.Fatalf("the cache holds %d answers; want at most %d", len(c.entries), c.max)
	}
	if _, owner := c.begin("c"); owner {
		t.Fatal("a fresh answer was not replayed")
	}
	now = now.Add(c.ttl)
	if _, owner := c.begin("c"); !owner {
		t.Fatal("an expired answer was replayed; want the request run again")
	}

	bus := teststub.New()
	cnt := &counting{}
	serve(t, bus, testProvider, cnt.endpoint(testProvider))
	subject := subjects.SlotSubject(testTenant, testUniverse, testProvider, "counter", ktsp.DefaultInstance, "write")
	for i := 0; i < 2; i++ {
		if _, err := bus.Request(context.Background(), subject, []byte(`{}`)); err != nil {
			t.Fatal(err)
		}
	}
	if n := cnt.runs.Load(); n != 2 {
		t.Fatalf("two requests with no id ran %d times; want each run", n)
	}
}

// AC-007: end to end - a write whose answer is lost (« Slot not found » after
// it landed) is sent again by the caller's RetrySender and done once.
func TestAWriteWhoseAnswerIsLostIsDoneOnceWhenSentAgain(t *testing.T) {
	bus := &spyBus{MessageBus: teststub.New(), loseFirst: ".counter.default.write"}
	c := &counting{}
	serve(t, bus, testProvider, c.endpoint(testProvider))
	serve(t, bus, "ktw-hub", hubEndpoint(func() {}))
	sender := NewRetrySender(NewSender(bus, testTenant, testUniverse), "ktw-hub")

	out, err := sender.Call(context.Background(), testProvider+":/counter/write", nil)
	if err != nil {
		t.Fatalf("the write sent again failed: %v", err)
	}
	if n := c.runs.Load(); n != 1 || string(out) != "1" {
		t.Fatalf("the write ran %d times, answered %s; want once, its first answer", n, out)
	}
}

// : the answers kept total at most maxBytes, the oldest going
// first - the hub kept every answer ten minutes whatever its size and held
// gigabytes of manifest slot types no retry asked again.
func TestTheReplayCacheIsBoundedInBytes(t *testing.T) {
	now := time.Unix(0, 0)
	c := newReplayCache()
	c.now = func() time.Time { return now }
	c.maxBytes, c.maxEntry = 100, 60
	for _, k := range []string{"a", "b", "c", "d"} {
		e, owner := c.begin(k)
		if !owner {
			t.Fatalf("%s: a new id is not run", k)
		}
		c.finish(e, make([]byte, 40))
		now = now.Add(time.Second)
	}
	if c.bytes > c.maxBytes {
		t.Fatalf("the cache keeps %d bytes of answers; want at most %d", c.bytes, c.maxBytes)
	}
	if len(c.entries) != 2 {
		t.Fatalf("the cache keeps %d answers; want the 2 newest that fit", len(c.entries))
	}
	if _, owner := c.begin("d"); owner {
		t.Fatal("the newest answer was not replayed")
	}
	if _, owner := c.begin("a"); !owner {
		t.Fatal("the oldest answer is still replayed past the byte budget")
	}
}

// An answer larger than maxEntry reaches the callers waiting on its run, and
// is not kept: a retry of it runs again.
func TestALargeAnswerReachesItsWaitersAndIsNotKept(t *testing.T) {
	c := newReplayCache()
	c.maxEntry = 64
	e, owner := c.begin("big")
	if !owner {
		t.Fatal("a new id is not run")
	}
	w, wOwner := c.begin("big")
	if wOwner || w != e {
		t.Fatal("a call sent again while its first run is in flight does not wait on it")
	}
	body := make([]byte, 1000)
	c.finish(e, body)
	<-w.done
	if len(w.body) != len(body) {
		t.Fatalf("the waiter got %d bytes; want the first run's %d", len(w.body), len(body))
	}
	if c.bytes != 0 || len(c.entries) != 0 {
		t.Fatalf("a %d-byte answer is kept (%d bytes, %d entries); want none", len(body), c.bytes, len(c.entries))
	}
	if _, owner := c.begin("big"); !owner {
		t.Fatal("a retry of an answer too large to keep is not run again")
	}
}
