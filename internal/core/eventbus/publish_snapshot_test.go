// SPDX-License-Identifier: Apache-2.0

package eventbus

import "testing"

// A publish sees exactly the handlers registered when it started: a handler
// that unsubscribes itself during the publish neither skips nor repeats the
// others.
func TestUnsubscribeDuringPublishSkipsNoOne(t *testing.T) {
	b := NewMemoryEventBus()
	calls := map[string]int{}
	var unsubA func()
	unsubA = b.Subscribe("t", func(string, any) { calls["a"]++; unsubA() })
	b.Subscribe("t", func(string, any) { calls["b"]++ })
	b.Subscribe("t", func(string, any) { calls["c"]++ })
	_ = b.Publish("t", nil)
	if calls["a"] != 1 || calls["b"] != 1 || calls["c"] != 1 {
		t.Fatalf("calls %v, want each handler once", calls)
	}
	_ = b.Publish("t", nil)
	if calls["a"] != 1 || calls["b"] != 2 || calls["c"] != 2 {
		t.Fatalf("after the unsubscribe: calls %v, want a gone, b and c twice", calls)
	}
}

// A handler that panics does not stop the others, on a topic or on SubscribeAny.
func TestAPanickingHandlerDoesNotStopTheOthers(t *testing.T) {
	b := NewMemoryEventBus()
	got := 0
	b.Subscribe("t", func(string, any) { panic("boom") })
	b.Subscribe("t", func(string, any) { got++ })
	b.SubscribeAny(func(string, any) { got++ })
	if err := b.Publish("t", nil); err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("%d handlers ran after the panic, want 2", got)
	}
}
