// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
)

// start_detached_test.go —: under load a start of ktw-agents
// outlasted the caller's wait; the caller gave up, its cancellation killed
// the start, and the next caller started it cold again - the member never
// came up. A start now runs apart from its caller, and a short start
// back-off is waited out instead of refusing a start that would succeed.

// A caller that gives up does not end the start: the member comes up, and
// the next caller takes it without a second process.
func TestAcquire_ACallerGivingUpDoesNotKillTheStart(t *testing.T) {
	calls, release := heldSpawns(t, nil)
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-agents", Transport: "stdio", Binary: "ktw-agents"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := m.Acquire(ctx, "ktw-agents"); done <- err }()
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel() // the caller's request timed out
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("the caller that gave up: %v, want its cancellation", err)
	}
	close(release) // the start completes after the caller left
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, _ := m.Status("ktw-agents")
		if st.State == cmapi.ConnReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the start was abandoned with its caller: %+v", st)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := m.Acquire(context.Background(), "ktw-agents"); err != nil {
		t.Fatalf("next caller: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d starts, want 1: the next caller took the member started for the one who left", n)
	}
	m.Release("ktw-agents")
	_ = m.Close()
}

// A call in a short back-off window waits it out and starts the member,
// which now comes up; a caller whose deadline ends within the window is
// refused at once, as is any caller of a window longer than maxBackoffWait.
func TestStartBackoff_AShortWindowIsWaitedOut(t *testing.T) {
	prevBase := startBackoffBase
	// A window wide enough that a loaded machine still calls within it
	// (the third call below must find more than maxBackoffWait left).
	startBackoffBase = time.Second
	t.Cleanup(func() { startBackoffBase = prevBase })
	notReady := errors.New("ready-probe ktw-x: readiness deadline elapsed")
	calls := scriptedSpawns(t, notReady, nil)
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-x", Transport: "stdio", Binary: "ktw-x"}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.Acquire(context.Background(), "ktw-x"); err == nil || errors.Is(err, ErrStartBackoff) {
		t.Fatalf("first start: %v, want the spawn's failure", err)
	}

	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := m.Acquire(short, "ktw-x"); !errors.Is(err, ErrStartBackoff) {
		t.Fatalf("a caller whose deadline ends within the window: %v, want a refusal", err)
	}

	prevWait := maxBackoffWait
	maxBackoffWait = 200 * time.Millisecond
	if _, err := m.Acquire(context.Background(), "ktw-x"); !errors.Is(err, ErrStartBackoff) {
		maxBackoffWait = prevWait
		t.Fatalf("a window longer than maxBackoffWait: %v, want a refusal", err)
	}
	maxBackoffWait = prevWait

	start := time.Now()
	if _, err := m.Acquire(context.Background(), "ktw-x"); err != nil {
		t.Fatalf("a call in the back-off: %v, want it to wait the window out and start the member", err)
	}
	if took := time.Since(start); took < 150*time.Millisecond {
		t.Errorf("started after %s: the back-off was not honoured", took)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("%d starts, want 2", n)
	}
}
