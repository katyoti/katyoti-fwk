// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// countingBus counts unsubscriptions.
type countingBus struct {
	EventBus
	unsubs atomic.Int64
}

func (b *countingBus) Subscribe(topic string, h func(string, any)) func() {
	unsub := b.EventBus.Subscribe(topic, h)
	return func() { b.unsubs.Add(1); unsub() }
}

// Waiting again on the same topics keeps the live subscriptions (an event
// published in between is not dropped); other topics replace them.
func TestWaitingAgainOnTheSameTopicsKeepsTheSubscriptions(t *testing.T) {
	bus := &countingBus{EventBus: NewMemoryEventBus()}
	ch := NewChannel(bus)
	inbox := NewMachineInbox()
	ch.Register("r", inbox)
	defer ch.Unregister("r")
	wait := func(topic string) *WaitSpec {
		return &WaitSpec{Mode: WaitAny, Conditions: []WaitCondition{{Topic: topic}}}
	}

	ch.Wait("r", wait("a"))
	ch.Wait("r", wait("a"))
	if n := bus.unsubs.Load(); n != 0 {
		t.Fatalf("the same wait again unsubscribed %d times; want the subscription kept", n)
	}
	_ = bus.Publish("a", "x")
	got, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if evs, err := inbox.Get(got, wait("a")); err != nil || len(evs) != 1 {
		t.Fatalf("the kept subscription delivers nothing: %v (%v)", evs, err)
	}

	ch.Wait("r", wait("b"))
	if bus.unsubs.Load() == 0 {
		t.Error("another wait did not replace the subscription")
	}
}

// A step that leaves its wait ends it: the subscriptions go, and the events
// still queued for it are dropped — a race loser's late event never reaches
// the next step's wait.
func TestEndingAWaitDropsItsSubscriptionsAndQueuedEvents(t *testing.T) {
	bus := &countingBus{EventBus: NewMemoryEventBus()}
	ch := NewChannel(bus)
	inbox := NewMachineInbox()
	ch.Register("r", inbox)
	defer ch.Unregister("r")
	any := func(topics ...string) *WaitSpec {
		w := &WaitSpec{Mode: WaitAny}
		for _, tp := range topics {
			w.Conditions = append(w.Conditions, WaitCondition{Topic: tp})
		}
		return w
	}
	ch.Wait("r", any("child.a.done", "child.b.done"))
	_ = bus.Publish("child.b.done", "late loser") // queued, never taken

	ch.EndWait("r")
	if bus.unsubs.Load() != 2 {
		t.Errorf("%d subscriptions removed; want both", bus.unsubs.Load())
	}
	_ = bus.Publish("child.a.done", "after the end") // nobody listens now

	ch.Wait("r", any("child.b.done", "next"))
	_ = bus.Publish("next", "the next step's own")
	got, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	evs, err := inbox.Get(got, any("child.b.done", "next"))
	if err != nil || len(evs) != 1 || evs[0].Type != "next" {
		t.Fatalf("the next wait got %v (%v); want its own event, not the loser's", evs, err)
	}
}
