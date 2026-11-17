// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"sync"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// twoWayBus is a minimal in-process pub/sub MessageBus stub: Publish
// dispatches to every Subscribe-registered handler for the matching
// subject. Enough to round-trip Publish → Subscribe end-to-end in unit
// tests, without standing up an embedded NATS server.
type twoWayBus struct {
	mu       sync.Mutex
	handlers map[string][]mbapi.Handler
}

func newTwoWayBus() *twoWayBus { return &twoWayBus{handlers: map[string][]mbapi.Handler{}} }

func (b *twoWayBus) Publish(ctx context.Context, subject string, payload []byte, _ ...mbapi.PubOpt) error {
	b.mu.Lock()
	hs := append([]mbapi.Handler(nil), b.handlers[subject]...)
	b.mu.Unlock()
	msg := &twoWayMsg{subject: subject, data: payload}
	for _, h := range hs {
		h(ctx, msg)
	}
	return nil
}

func (b *twoWayBus) Subscribe(_ context.Context, subject string, h mbapi.Handler, _ ...mbapi.SubOpt) (mbapi.Subscription, error) {
	b.mu.Lock()
	b.handlers[subject] = append(b.handlers[subject], h)
	b.mu.Unlock()
	return &twoWaySub{}, nil
}

func (b *twoWayBus) QueueSubscribe(context.Context, string, string, mbapi.Handler, ...mbapi.SubOpt) (mbapi.Subscription, error) {
	panic("twoWayBus.QueueSubscribe: not used")
}
func (b *twoWayBus) Request(context.Context, string, []byte, ...mbapi.ReqOpt) ([]byte, error) {
	panic("twoWayBus.Request: not used")
}
func (b *twoWayBus) JetStream() mbapi.JetStreamContext { return nil }
func (b *twoWayBus) Drain(context.Context) error       { return nil }

type twoWayMsg struct {
	subject string
	data    []byte
}

func (m *twoWayMsg) Subject() string        { return m.subject }
func (m *twoWayMsg) Header() mbapi.Header   { return nil }
func (m *twoWayMsg) Data() []byte           { return m.data }
func (m *twoWayMsg) Reply() string          { return "" }
func (m *twoWayMsg) Ack() error             { return nil }
func (m *twoWayMsg) Respond(_ []byte) error { return nil }

type twoWaySub struct{}

func (s *twoWaySub) Drain() error       { return nil }
func (s *twoWaySub) Unsubscribe() error { return nil }
func (s *twoWaySub) IsValid() bool      { return true }

// TestContributionsRoundTrip_PublishSubscribe — full publisher →
// subscriber loop: catalog emits via the NATS publisher, our subscriber
// receives the event, debounces, fires onEvent with the round-tripped
// payload intact.
func TestContributionsRoundTrip_PublishSubscribe(t *testing.T) {
	bus := newTwoWayBus()
	pub := NewNATSContributionsEventPublisher(bus, "local", "test-uni")

	var (
		mu       sync.Mutex
		received []contribapi.ContributionsChangedEvent
	)
	sub, err := SubscribeContributionsChanged(
		context.Background(), bus, "local", "test-uni", 0, // no debounce — instant fire
		func(ev contribapi.ContributionsChangedEvent) {
			mu.Lock()
			received = append(received, ev)
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Drain() }()

	want := contribapi.ContributionsChangedEvent{
		Provider:     "ktw-docs",
		KindsChanged: []string{"skill"},
		ChangedAt:    time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC),
	}
	if err := pub.PublishContributionsChanged(want); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	mu.Lock()
	got := append([]contribapi.ContributionsChangedEvent(nil), received...)
	mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("received %d events, want 1", len(got))
	}
	if got[0].Provider != want.Provider {
		t.Errorf("Provider = %q, want %q", got[0].Provider, want.Provider)
	}
	if len(got[0].KindsChanged) != 1 || got[0].KindsChanged[0] != "skill" {
		t.Errorf("KindsChanged = %v, want [skill]", got[0].KindsChanged)
	}
	if !got[0].ChangedAt.Equal(want.ChangedAt) {
		t.Errorf("ChangedAt = %v, want %v", got[0].ChangedAt, want.ChangedAt)
	}
}

// TestContributionsSubscriber_Debounces — three rapid events within
// the debounce window collapse to one onEvent call.
func TestContributionsSubscriber_Debounces(t *testing.T) {
	bus := newTwoWayBus()
	pub := NewNATSContributionsEventPublisher(bus, "local", "uni")

	var (
		mu    sync.Mutex
		count int
	)
	sub, err := SubscribeContributionsChanged(
		context.Background(), bus, "local", "uni", 50*time.Millisecond,
		func(_ contribapi.ContributionsChangedEvent) {
			mu.Lock()
			count++
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Drain() }()

	for i := 0; i < 3; i++ {
		if err := pub.PublishContributionsChanged(contribapi.ContributionsChangedEvent{
			ChangedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("Publish[%d]: %v", i, err)
		}
		time.Sleep(5 * time.Millisecond) // well under debounce
	}

	// Wait past the debounce window for the single fire to happen.
	time.Sleep(120 * time.Millisecond)

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 1 {
		t.Errorf("debounced events fired onEvent %d times, want 1 (three rapid Publish calls within the 50ms quiet window)", got)
	}
}

// TestContributionsSubscriber_FiresOnSubject — only events on the
// matching universe subject are delivered; cross-universe traffic is
// silently dropped. Guard against accidental subject globbing.
func TestContributionsSubscriber_FiresOnSubject(t *testing.T) {
	bus := newTwoWayBus()

	var (
		mu    sync.Mutex
		count int
	)
	sub, err := SubscribeContributionsChanged(
		context.Background(), bus, "local", "uni-a", 0,
		func(_ contribapi.ContributionsChangedEvent) {
			mu.Lock()
			count++
			mu.Unlock()
		},
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Drain() }()

	// Publish on a DIFFERENT universe — subscriber should NOT fire.
	otherSubject := subjects.ContributionsChangedSubject("local", "uni-b")
	if err := bus.Publish(context.Background(), otherSubject, []byte(`{"provider":"x"}`)); err != nil {
		t.Fatalf("cross-publish: %v", err)
	}

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 0 {
		t.Errorf("cross-universe event delivered to subscriber; got %d events, want 0", got)
	}
}
