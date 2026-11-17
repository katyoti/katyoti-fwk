// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// fakePublisher captures every emitted ContributionsChangedEvent for
// assertion. Thread-safe — emission paths may run from any goroutine.
type fakePublisher struct {
	mu     sync.Mutex
	events []contribapi.ContributionsChangedEvent
	err    error // optional: pre-loaded to verify error path is non-fatal
}

func (p *fakePublisher) PublishContributionsChanged(ev contribapi.ContributionsChangedEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
	return p.err
}

func (p *fakePublisher) snapshot() []contribapi.ContributionsChangedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]contribapi.ContributionsChangedEvent, len(p.events))
	copy(out, p.events)
	return out
}

// TestCatalog_EmitsOnMarkReady — MarkReady fires a coarse "*" event so
// subscribers refresh once the catalog is first fully indexed.
func TestCatalog_EmitsOnMarkReady(t *testing.T) {
	cache := NewCache(t.TempDir())
	c, err := NewCatalog(cache, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	pub := &fakePublisher{}
	c.SetEventPublisher(pub)

	before := time.Now().UTC()
	if err := c.MarkReady(context.Background()); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	after := time.Now().UTC()

	got := pub.snapshot()
	if len(got) != 1 {
		t.Fatalf("emitted %d events, want 1", len(got))
	}
	ev := got[0]
	if ev.Provider != "" {
		t.Errorf("Provider = %q, want \"\" (universe-wide)", ev.Provider)
	}
	if len(ev.KindsChanged) != 1 || ev.KindsChanged[0] != "*" {
		t.Errorf("KindsChanged = %v, want [\"*\"]", ev.KindsChanged)
	}
	if ev.ChangedAt.Before(before) || ev.ChangedAt.After(after) {
		t.Errorf("ChangedAt = %v, want in [%v, %v]", ev.ChangedAt, before, after)
	}
}

// TestCatalog_EmitsOnReload — Reload fires a coarse "*" event so
// subscribers refresh after a fresh manifest cache file is written.
func TestCatalog_EmitsOnReload(t *testing.T) {
	dir := t.TempDir()
	cache := NewCache(dir)
	// Seed one manifest so Reload has something to load.
	m := makeManifest("ktw-docs", "1.0.0", "sha256:abc")
	if err := SeedCacheSave(t, cache, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c, err := NewCatalog(cache, MembersOf(cache, "ktw-docs"))
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	pub := &fakePublisher{}
	c.SetEventPublisher(pub)

	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	got := pub.snapshot()
	if len(got) != 1 {
		t.Fatalf("Reload emitted %d events, want 1", len(got))
	}
	if got[0].Provider != "" || len(got[0].KindsChanged) != 1 || got[0].KindsChanged[0] != "*" {
		t.Errorf("Reload event = %+v, want Provider=\"\" KindsChanged=[\"*\"]", got[0])
	}
}

// TestCatalog_NoEmissionWithoutPublisher — when SetEventPublisher was
// never called (or set to nil), Reload/MarkReady are silent. Binaries
// that don't need cross-process notification pay no overhead.
func TestCatalog_NoEmissionWithoutPublisher(t *testing.T) {
	cache := NewCache(t.TempDir())
	c, err := NewCatalog(cache, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	// No SetEventPublisher call.
	if err := c.MarkReady(context.Background()); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	// If this test runs without panicking, the nil-publisher branch in
	// emitChanged worked — there's nothing else to assert.
}

// TestCatalog_PublisherErrorIsNonFatal — a broken publisher logs but
// never fails the operation. Catalog operations must NOT be blocked by
// transport failures (NATS disconnect, etc.).
func TestCatalog_PublisherErrorIsNonFatal(t *testing.T) {
	cache := NewCache(t.TempDir())
	c, err := NewCatalog(cache, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	pub := &fakePublisher{err: errAlwaysFail}
	c.SetEventPublisher(pub)

	if err := c.MarkReady(context.Background()); err != nil {
		t.Errorf("MarkReady should ignore publisher error, got %v", err)
	}
	if err := c.Reload(); err != nil {
		t.Errorf("Reload should ignore publisher error, got %v", err)
	}

	// The publisher was still called (events recorded) even though it
	// returned an error.
	if got := pub.snapshot(); len(got) != 2 {
		t.Errorf("publisher called %d times, want 2 (MarkReady + Reload)", len(got))
	}
}

// errAlwaysFail is a sentinel error returned by the fake publisher in
// the non-fatal path test.
var errAlwaysFail = &publisherError{msg: "simulated transport failure"}

type publisherError struct{ msg string }

func (e *publisherError) Error() string { return e.msg }

// TestCatalog_SetEventPublisher_ReplacesPrevious — SetEventPublisher is
// idempotent: passing a new publisher replaces the old one; subsequent
// emissions go only to the latest.
func TestCatalog_SetEventPublisher_ReplacesPrevious(t *testing.T) {
	cache := NewCache(t.TempDir())
	c, err := NewCatalog(cache, nil)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}

	first := &fakePublisher{}
	second := &fakePublisher{}

	c.SetEventPublisher(first)
	if err := c.MarkReady(context.Background()); err != nil {
		t.Fatalf("MarkReady (1st pub): %v", err)
	}

	c.SetEventPublisher(second)
	if err := c.Reload(); err != nil {
		t.Fatalf("Reload (2nd pub): %v", err)
	}

	if got := len(first.snapshot()); got != 1 {
		t.Errorf("first publisher saw %d events, want 1 (only the MarkReady before swap)", got)
	}
	if got := len(second.snapshot()); got != 1 {
		t.Errorf("second publisher saw %d events, want 1 (only the Reload after swap)", got)
	}
}
