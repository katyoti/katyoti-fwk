// SPDX-License-Identifier: Apache-2.0

package messagebus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
)

// waitingEndpoint serves hello/wait: it tells started when it runs, then
// waits until its context ends and tells stopped.
func waitingEndpoint(started, stopped chan struct{}) *ktsp.Endpoint {
	ep := ktsp.NewEndpoint("test", "", "")
	slot := ktsp.NewSlot("hello", ktsp.DefaultInstance, "", nil)
	slot.Add(ktsp.RawMethod("wait", ktsp.MethodOpts{},
		func(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
			close(started)
			select {
			case <-ctx.Done():
				close(stopped)
				return nil, ctx.Err()
			case <-time.After(30 * time.Second):
				return json.RawMessage(`"never cancelled"`), nil
			}
		}))
	ep.AddSlot(slot)
	return ep
}

// assertCancelReachesTheMethod: a caller cancels a call in flight; the
// method's context ends in its sidecar.
func assertCancelReachesTheMethod(t *testing.T, sender *Sender, started, stopped chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() {
		_, err := sender.Call(ctx, testProvider+":/hello/wait", nil)
		errc <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the method never started")
	}
	cancel()
	if err := <-errc; err == nil {
		t.Fatal("a cancelled call returned no error")
	}
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("the caller cancelled the call, but the method's context never ended: the cancel did not reach the sidecar")
	}
}

// TestACancelledCallStopsItsMethod_RealNATS: over a real NATS server.
func TestACancelledCallStopsItsMethod_RealNATS(t *testing.T) {
	h := startEmbeddedHub(t)
	bus := natsxport.New(h.Client)
	started, stopped := make(chan struct{}), make(chan struct{})
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, waitingEndpoint(started, stopped))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()
	assertCancelReachesTheMethod(t, NewSender(bus, testTenant, testUniverse), started, stopped)
}

// TestACancelledCallStopsItsMethod_Stub: over the in-memory bus.
func TestACancelledCallStopsItsMethod_Stub(t *testing.T) {
	bus := teststub.New()
	started, stopped := make(chan struct{}), make(chan struct{})
	group, err := Subscribe(context.Background(), bus, testTenant, testUniverse, testProvider, waitingEndpoint(started, stopped))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer group.Unsubscribe()
	assertCancelReachesTheMethod(t, NewSender(bus, testTenant, testUniverse), started, stopped)
}

// TestACancelBeforeItsCallStartsItCancelled: a cancel that arrives while its
// call still waits for a worker is kept, and the call starts cancelled; a
// call never cancelled is not.
func TestACancelBeforeItsCallStartsItCancelled(t *testing.T) {
	r := newCallRegistry()
	r.cancel("early")
	ctx, done := r.begin(context.Background(), "early")
	defer done()
	if ctx.Err() == nil {
		t.Fatal("a call whose cancel came first started live")
	}
	other, done2 := r.begin(context.Background(), "other")
	if other.Err() != nil {
		t.Fatal("a call never cancelled started cancelled")
	}
	r.cancel("other")
	if other.Err() == nil {
		t.Fatal("cancelling a running call left its context live")
	}
	done2()
	if len(r.running) != 0 {
		t.Fatalf("ended calls left in the registry: %v", r.running)
	}
}

// TestAnOldEarlyCancelIsForgotten: early cancels are not kept forever.
func TestAnOldEarlyCancelIsForgotten(t *testing.T) {
	r := newCallRegistry()
	now := time.Now()
	r.now = func() time.Time { return now }
	r.cancel("stale")
	now = now.Add(2 * earlyCancelTTL)
	r.cancel("fresh")
	if _, ok := r.early["stale"]; ok {
		t.Fatal("an early cancel older than its TTL was kept")
	}
	if _, ok := r.early["fresh"]; !ok {
		t.Fatal("a fresh early cancel was lost")
	}
}

// slowPublishBus is a bus whose Publish takes until its context ends: a
// cancel notice that cannot leave quickly.
type slowPublishBus struct {
	mbapi.MessageBus
	published chan string
}

func (b slowPublishBus) Publish(ctx context.Context, subject string, _ []byte, _ ...mbapi.PubOpt) error {
	b.published <- subject
	<-ctx.Done()
	return ctx.Err()
}

// TestACancelledCallReturnsAtOnce: the caller that gave up gets its error at
// once - the cancel notice leaves without making it wait - and
// the notice is still published.
func TestACancelledCallReturnsAtOnce(t *testing.T) {
	bus := slowPublishBus{MessageBus: teststub.New(), published: make(chan string, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if _, err := NewSender(bus, testTenant, testUniverse).Call(ctx, testProvider+":/hello/wait", nil); err == nil {
		t.Fatal("a cancelled call returned no error")
	}
	if took := time.Since(start); took > cancelPublishTimeout/4 {
		t.Fatalf("a cancelled call took %v to return: it waited for its cancel notice", took)
	}
	select {
	case subject := <-bus.published:
		if want := subjects.SlotCancelSubject(testTenant, testUniverse, testProvider); subject != want {
			t.Fatalf("the cancel went on %q, want %q", subject, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the cancel notice was never published")
	}
}
