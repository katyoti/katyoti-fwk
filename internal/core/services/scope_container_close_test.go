// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// funcCloser is an io.Closer running fn.
type funcCloser struct{ fn func() error }

func (c *funcCloser) Close() error { return c.fn() }

// A service whose Close waits for a goroutine that resolves a service from the
// same container — the CWM service stopping an event observer whose handler
// calls application.UseService — must not deadlock: the resolution gets a
// prompt "closed" error, the handler returns, Close returns.
func TestScopeContainer_Close_ResolutionDuringCloseDoesNotBlock(t *testing.T) {
	sc := NewScopeContainer(ApplicationScope)

	handlerStarted := make(chan struct{})
	release := make(chan struct{})
	handlerDone := make(chan error, 1)
	go func() {
		close(handlerStarted)
		<-release // the handler is mid-flight when Close begins
		_, err := sc.GetOrCreate("Other(default)", func() (any, error) { return "other", nil })
		handlerDone <- err
	}()
	<-handlerStarted

	sc.Put("Observed(default)", &funcCloser{fn: func() error {
		close(release)
		select { // the observer stop: wait for the running handler
		case err := <-handlerDone:
			if err == nil || !strings.Contains(err.Error(), "closed") {
				t.Errorf("a resolution during Close = %v, want a \"closed\" error", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("the handler resolving a service during Close never returned")
		}
		return nil
	}})

	start := time.Now()
	_ = sc.Close()
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Close took %v, want well under a second", d)
	}
}

// Close disposes services in the reverse of the order they became available:
// a service built from another is closed before it.
func TestScopeContainer_Close_ReverseAvailabilityOrder(t *testing.T) {
	sc := NewScopeContainer(ApplicationScope)
	var mu sync.Mutex
	var order []string
	closer := func(name string) *funcCloser {
		return &funcCloser{fn: func() error {
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}}
	}
	sc.Put("Boot(default)", closer("boot"))
	_, err := sc.GetOrCreate("Dependent(default)", func() (any, error) {
		// The dependent's factory resolves its dependency: the dependency
		// becomes available first.
		if _, err := sc.GetOrCreate("Dependency(default)", func() (any, error) {
			return closer("dependency"), nil
		}); err != nil {
			return nil, err
		}
		return closer("dependent"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sc.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(order, ","), "dependent,dependency,boot"; got != want {
		t.Fatalf("close order = %s, want %s", got, want)
	}
	if err := sc.Close(); err != nil { // closing twice is a no-op
		t.Fatalf("second Close = %v", err)
	}
	if len(order) != 3 {
		t.Fatalf("second Close closed again: %v", order)
	}
}

// A factory still running when the container closes: Close does not see its
// service, so the factory's caller disposes it and gets a "closed" error —
// nothing created after Close outlives it.
func TestScopeContainer_Close_FactoryFinishingAfterCloseIsDisposed(t *testing.T) {
	sc := NewScopeContainer(ApplicationScope)
	inFactory := make(chan struct{})
	finish := make(chan struct{})
	disposed := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := sc.GetOrCreate("Late(default)", func() (any, error) {
			close(inFactory)
			<-finish
			return &funcCloser{fn: func() error { close(disposed); return nil }}, nil
		})
		result <- err
	}()
	<-inFactory
	_ = sc.Close()
	close(finish)
	if err := <-result; err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("GetOrCreate finishing after Close = %v, want a \"closed\" error", err)
	}
	select {
	case <-disposed:
	case <-time.After(time.Second):
		t.Fatal("the service created after Close was not disposed")
	}
}

// drainingWorker is a service with background work: on Drain it finishes its
// pending job, which resolves a service from the container.
type drainingWorker struct {
	sc      *ScopeContainer
	pending bool
	got     chan error
}

func (w *drainingWorker) Drain(context.Context) {
	if w.pending {
		_, err := w.sc.GetOrCreate("Recorder(default)", func() (any, error) { return "recorder", nil })
		w.got <- err
		w.pending = false
	}
}

func (w *drainingWorker) Close() error { return nil }

// Close drains every Drainer FIRST, with the container still open: the
// pending work resolves its services and is done before anything closes.
func TestScopeContainer_Close_DrainsWhileOpen(t *testing.T) {
	sc := NewScopeContainer(ApplicationScope)
	w := &drainingWorker{sc: sc, pending: true, got: make(chan error, 1)}
	sc.Put("Worker(default)", w)
	if err := sc.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-w.got:
		if err != nil {
			t.Fatalf("the drained work could not resolve a service: %v", err)
		}
	default:
		t.Fatal("Close did not drain the pending work")
	}
	if _, err := sc.GetOrCreate("After(default)", func() (any, error) { return 1, nil }); err == nil {
		t.Fatal("a resolution after Close succeeded")
	}
}
