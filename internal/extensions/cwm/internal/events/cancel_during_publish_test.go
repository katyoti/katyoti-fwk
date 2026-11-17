// SPDX-License-Identifier: Apache-2.0

package events

import (
	stdctx "context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CANCEL DURING PUBLISH. The bus calls handlers from a snapshot of its list, so
// a subscription cancelled while a publish is under way can still have its
// handler called. That late send must be dropped quietly: no panic, and every
// other subscriber and the NATS leg of that publish still get the event.
//
// No -race here, so this is a stress loop with deterministic counts: churners
// subscribe and cancel as fast as they can while publishers emit; a long-lived
// subscriber and an observer must see EVERY event, NATS must get every publish,
// and not one panic may have been recovered.
func TestSubscribeCancelDuringPublish_DropsNothingElse(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	mb := &recordingBus{}
	svc := New("acme", "prod", "ws-1", bus, mb)

	// Registered FIRST, before any churner, so its place in the bus's list
	// never moves while churners come and go.
	var direct atomic.Int64
	unsubDirect := bus.SubscribeAny(func(string, any) { direct.Add(1) })
	defer unsubDirect()

	var observed atomic.Int64
	stopObserve := svc.Observe("", func(api.EventEnvelope) { observed.Add(1) })

	// A long-lived Subscribe, drained, so its buffer never fills.
	longCh, longCancel, _ := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{})
	var longGot atomic.Int64
	longDone := make(chan struct{})
	go func() {
		defer close(longDone)
		for range longCh {
			longGot.Add(1)
		}
	}()

	before := recoveredPanics.Load()

	const (
		publishers = 4
		perPub     = 2000
		churners   = 8
	)
	stop := make(chan struct{})
	var churn sync.WaitGroup
	for c := 0; c < churners; c++ {
		churn.Add(1)
		go func() {
			defer churn.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_, cancel, _ := svc.Subscribe(stdctx.Background(), api.EventsSubscribeInput{})
				cancel()
			}
		}()
	}

	var pubs sync.WaitGroup
	for p := 0; p < publishers; p++ {
		pubs.Add(1)
		go func() {
			defer pubs.Done()
			for i := 0; i < perPub; i++ {
				svc.PublishObject("updated", "m.x", "m", "doc", "", "", "", api.EventMeta{})
			}
		}()
	}
	pubs.Wait()
	close(stop)
	churn.Wait()

	const total = publishers * perPub
	if n := recoveredPanics.Load() - before; n != 0 {
		t.Errorf("%d panics recovered during the stress, want 0", n)
	}
	if n := len(mb.all()); n != total {
		t.Errorf("NATS got %d publishes, want %d", n, total)
	}
	if n := direct.Load(); n != total {
		t.Errorf("long-lived bus subscriber got %d events, want %d", n, total)
	}
	deadline := time.Now().Add(10 * time.Second)
	for observed.Load() < total && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	stopObserve()
	if n := observed.Load(); n != total {
		t.Errorf("observer got %d events, want %d", n, total)
	}
	longCancel()
	<-longDone
	if n := longGot.Load(); n == 0 {
		t.Error("long-lived Subscribe received nothing")
	}
	longCancel() // a second cancel is a no-op
}

// A panicking handler stops neither the NATS leg of the publish nor this
// package's other handlers.
func TestPublish_PanickingHandlerKeepsNATSLeg(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	mb := &recordingBus{}
	svc := New("acme", "prod", "ws-1", bus, mb)
	unsub := bus.SubscribeAny(func(string, any) { panic("boom") })
	defer unsub()
	before := recoveredPanics.Load()
	svc.PublishObject("created", "m.x", "m", "doc", "", "", "", api.EventMeta{})
	if n := len(mb.all()); n != 1 {
		t.Errorf("NATS got %d publishes after a handler panicked, want 1", n)
	}
	// The in-process bus isolates each handler itself; this package's own
	// recover is the second line, for a bus that does not.
	if n := recoveredPanics.Load() - before; n > 1 {
		t.Errorf("recovered %d panics, want at most 1", n)
	}
}
