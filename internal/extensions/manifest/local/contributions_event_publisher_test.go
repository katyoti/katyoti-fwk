// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// fakeBus is a minimal mbapi.MessageBus stub — Publish records calls;
// everything else panics so a regression that wires the publisher to
// an unintended bus method surfaces loudly. Thread-safe.
type fakeBus struct {
	mu         sync.Mutex
	published  []publishCall
	publishErr error
}

type publishCall struct {
	subject string
	payload []byte
}

func (b *fakeBus) Publish(_ context.Context, subject string, payload []byte, _ ...mbapi.PubOpt) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, publishCall{subject: subject, payload: append([]byte(nil), payload...)})
	return b.publishErr
}

func (b *fakeBus) Subscribe(context.Context, string, mbapi.Handler, ...mbapi.SubOpt) (mbapi.Subscription, error) {
	panic("fakeBus.Subscribe: not exercised by contributions publisher tests")
}
func (b *fakeBus) QueueSubscribe(context.Context, string, string, mbapi.Handler, ...mbapi.SubOpt) (mbapi.Subscription, error) {
	panic("fakeBus.QueueSubscribe: not exercised")
}
func (b *fakeBus) Request(context.Context, string, []byte, ...mbapi.ReqOpt) ([]byte, error) {
	panic("fakeBus.Request: not exercised")
}
func (b *fakeBus) JetStream() mbapi.JetStreamContext { return nil }
func (b *fakeBus) Drain(context.Context) error       { return nil }

func (b *fakeBus) snapshot() []publishCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]publishCall, len(b.published))
	copy(out, b.published)
	return out
}

// TestNATSContributionsPublisher_PublishesToCorrectSubject — the
// payload is JSON-marshalled and the subject is the canonical universe-
// scoped form from `subjects.ContributionsChangedSubject`.
func TestNATSContributionsPublisher_PublishesToCorrectSubject(t *testing.T) {
	bus := &fakeBus{}
	pub := NewNATSContributionsEventPublisher(bus, "local", "test-uni")

	ev := contribapi.ContributionsChangedEvent{
		Provider:     "ktw-docs",
		KindsChanged: []string{"skill", "reference"},
		ChangedAt:    time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
	}
	if err := pub.PublishContributionsChanged(ev); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	calls := bus.snapshot()
	if len(calls) != 1 {
		t.Fatalf("Publish called %d times, want 1", len(calls))
	}

	wantSubject := subjects.ContributionsChangedSubject("local", "test-uni")
	if calls[0].subject != wantSubject {
		t.Errorf("subject = %q, want %q", calls[0].subject, wantSubject)
	}

	var roundTrip contribapi.ContributionsChangedEvent
	if err := json.Unmarshal(calls[0].payload, &roundTrip); err != nil {
		t.Fatalf("payload not JSON: %v (raw: %s)", err, calls[0].payload)
	}
	if roundTrip.Provider != ev.Provider {
		t.Errorf("Provider round-trip = %q, want %q", roundTrip.Provider, ev.Provider)
	}
	if len(roundTrip.KindsChanged) != 2 || roundTrip.KindsChanged[0] != "skill" || roundTrip.KindsChanged[1] != "reference" {
		t.Errorf("KindsChanged round-trip = %v, want [skill reference]", roundTrip.KindsChanged)
	}
	if !roundTrip.ChangedAt.Equal(ev.ChangedAt) {
		t.Errorf("ChangedAt round-trip = %v, want %v", roundTrip.ChangedAt, ev.ChangedAt)
	}
}

// TestNATSContributionsPublisher_BusErrorPropagates — when the bus
// fails (NATS disconnect, etc.), the publisher returns a wrapped error.
// CatalogImpl logs + continues per its non-fatal contract; the test
// just confirms the error reaches the caller and includes the subject
// for debugging.
func TestNATSContributionsPublisher_BusErrorPropagates(t *testing.T) {
	bus := &fakeBus{publishErr: errors.New("NATS disconnected")}
	pub := NewNATSContributionsEventPublisher(bus, "local", "test-uni")

	err := pub.PublishContributionsChanged(contribapi.ContributionsChangedEvent{
		Provider:  "ktw-docs",
		ChangedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected error from bus failure, got nil")
	}
	if !strings.Contains(err.Error(), "NATS disconnected") {
		t.Errorf("error %q should wrap the bus error", err.Error())
	}
}

// TestNATSContributionsPublisher_NilBusErrors — defensive: a nil bus
// passed at construction returns a clear error rather than a panic.
func TestNATSContributionsPublisher_NilBusErrors(t *testing.T) {
	pub := NewNATSContributionsEventPublisher(nil, "local", "test-uni")
	err := pub.PublishContributionsChanged(contribapi.ContributionsChangedEvent{ChangedAt: time.Now()})
	if err == nil {
		t.Fatal("expected error for nil bus, got nil")
	}
	if !strings.Contains(err.Error(), "nil MessageBus") {
		t.Errorf("error %q should mention nil MessageBus", err.Error())
	}
}
