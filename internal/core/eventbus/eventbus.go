// SPDX-License-Identifier: Apache-2.0

// Package eventbus provides typed pub/sub for notifications.
// Topics are strings (e.g., "session.{id}.agent.text_delta").
// Phase 1: in-memory Go channels. Phase 2: pluggable backend (Redis/NATS).
package eventbus

import (
	"log/slog"
	"sync"
)

// EventBus provides typed pub/sub for notifications.
type EventBus interface {
	// Publish sends a notification to all subscribers of the topic.
	Publish(topic string, payload any) error

	// Subscribe registers a handler for a topic. Returns an unsubscribe function.
	Subscribe(topic string, handler func(topic string, payload any)) func()

	// SubscribeAny registers a handler invoked on every Publish regardless of
	// topic. Used for cross-cutting observability (tracing, debugging, the
	// dry-run tracer) where the full set of topic strings is unknown upfront.
	// Returns an unsubscribe function.
	SubscribeAny(handler func(topic string, payload any)) func()
}

// MemoryEventBus is an in-process EventBus backed by Go function calls.
// Thread-safe. Handlers are called synchronously in the publisher's goroutine.
type MemoryEventBus struct {
	mu      sync.RWMutex
	subs    map[string][]memSub
	anySubs []memSub // invoked on every Publish (see SubscribeAny)
	nextID  int
}

type memSub struct {
	id      int
	handler func(string, any)
}

// NewMemoryEventBus creates an in-process event bus.
func NewMemoryEventBus() *MemoryEventBus {
	return &MemoryEventBus{
		subs: make(map[string][]memSub),
	}
}

func (b *MemoryEventBus) Publish(topic string, payload any) error {
	b.mu.RLock()
	handlers := b.subs[topic]
	any := b.anySubs
	b.mu.RUnlock()

	// The slices read above are never modified in place (an unsubscribe
	// builds a new one), so this publish sees exactly the handlers that were
	// registered when it started. Each handler runs isolated: one that panics
	// is logged and the others still run.
	for _, sub := range handlers {
		callIsolated(sub.handler, topic, payload)
	}
	for _, sub := range any {
		callIsolated(sub.handler, topic, payload)
	}
	return nil
}

func callIsolated(handler func(string, any), topic string, payload any) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("eventbus: a handler panicked; the other handlers still run", "topic", topic, "panic", r)
		}
	}()
	handler(topic, payload)
}

// without returns subs minus the entry with id, as a NEW slice: a publish
// holding the old one keeps an intact snapshot.
func without(subs []memSub, id int) []memSub {
	out := make([]memSub, 0, len(subs))
	for _, s := range subs {
		if s.id != id {
			out = append(out, s)
		}
	}
	return out
}

func (b *MemoryEventBus) Subscribe(topic string, handler func(string, any)) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[topic] = append(b.subs[topic], memSub{id: id, handler: handler})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.subs[topic] = without(b.subs[topic], id)
	}
}

// TotalSubscribers returns the total handler count across all topics plus
// SubscribeAny handlers. Intended for leak-detection tests and observability
// metrics — not a hot-path API.
func (b *MemoryEventBus) TotalSubscribers() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	n := len(b.anySubs)
	for _, subs := range b.subs {
		n += len(subs)
	}
	return n
}

// SubscribeAny registers a handler invoked on every Publish regardless of topic.
func (b *MemoryEventBus) SubscribeAny(handler func(string, any)) func() {
	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.anySubs = append(b.anySubs, memSub{id: id, handler: handler})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.anySubs = without(b.anySubs, id)
	}
}

// NopEventBus is a no-op EventBus that discards all events.
type NopEventBus struct{}

func (NopEventBus) Publish(string, any) error                  { return nil }
func (NopEventBus) Subscribe(string, func(string, any)) func() { return func() {} }
func (NopEventBus) SubscribeAny(func(string, any)) func()      { return func() {} }
