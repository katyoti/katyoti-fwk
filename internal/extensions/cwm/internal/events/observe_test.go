// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestObserveLosesNothingAndKeepsOrder: a burst far past Subscribe's buffer
// reaches an observer whole and in order, even when the observer is slow.
func TestObserveLosesNothingAndKeepsOrder(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	s := &Service{bus: bus}
	const n = 1000
	var mu sync.Mutex
	var got []string
	done := make(chan struct{})
	stop := s.Observe("", func(ev api.EventEnvelope) {
		time.Sleep(10 * time.Microsecond) // a slow observer
		mu.Lock()
		got = append(got, ev.Topic)
		if len(got) == n {
			close(done)
		}
		mu.Unlock()
	})
	defer stop()
	for i := 0; i < n; i++ {
		_ = bus.Publish("t."+string(rune('a'+i%26))+"."+itoa(i), map[string]int{"i": i})
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("observer got %d of %d", len(got), n)
	}
	for i, topic := range got {
		if want := "t." + string(rune('a'+i%26)) + "." + itoa(i); topic != want {
			t.Fatalf("event %d = %s, want %s — order lost", i, topic, want)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// TestStopReturnsOnceTheObserverHasEnded: a handler still running when stop is
// called has returned when stop returns, and nothing queued runs afterwards —
// an observer never writes into a workspace already closed.
func TestStopReturnsOnceTheObserverHasEnded(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	s := &Service{bus: bus}
	var mu sync.Mutex
	handled := 0
	started := make(chan struct{}, 8)
	stop := s.Observe("", func(api.EventEnvelope) {
		started <- struct{}{}
		time.Sleep(150 * time.Millisecond) // a write still under way
		mu.Lock()
		handled++
		mu.Unlock()
	})
	for i := 0; i < 5; i++ {
		_ = bus.Publish("t.e"+itoa(i), nil)
	}
	<-started
	stop()
	mu.Lock()
	atStop := handled
	mu.Unlock()
	if atStop != 1 {
		t.Fatalf("handled %d when stop returned, want the running one finished (1)", atStop)
	}
	time.Sleep(400 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if handled != atStop {
		t.Fatalf("the observer handled %d more event(s) after stop returned", handled-atStop)
	}
}

// TestDrainHandlesWhatWasReceivedAndTakesNothingNew: Drain returns once every
// event already published has been handled — the service the handler writes
// into is still open — and an event published after it is not taken; Stop
// afterwards returns at once.
func TestDrainHandlesWhatWasReceivedAndTakesNothingNew(t *testing.T) {
	bus := eventbus.NewMemoryEventBus()
	s := &Service{bus: bus}
	var mu sync.Mutex
	var got []string
	o := s.ObserveDrainable("", func(ev api.EventEnvelope) {
		time.Sleep(20 * time.Millisecond) // a write still under way
		mu.Lock()
		got = append(got, ev.Topic)
		mu.Unlock()
	})
	for i := 0; i < 5; i++ {
		_ = bus.Publish("t.e"+itoa(i), nil)
	}
	o.Drain(context.Background())
	mu.Lock()
	if len(got) != 5 {
		mu.Unlock()
		t.Fatalf("Drain returned after %d of 5 events", len(got))
	}
	mu.Unlock()
	_ = bus.Publish("t.late", nil)
	start := time.Now()
	o.Stop()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Stop after Drain took %v", d)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 5 {
		t.Fatalf("an event published after Drain was handled: %v", got)
	}
}
