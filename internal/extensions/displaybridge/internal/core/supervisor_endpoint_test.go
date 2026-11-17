// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/displaybridge/api"
)

// fakeProc stands in for the bun ExternalProcess: it "runs" the given
// httptest server and records Start / Close.
type fakeProc struct {
	port     int
	startErr error
	delay    time.Duration
	starts   *atomic.Int32
	closes   atomic.Int32
	running  atomic.Bool
}

func (f *fakeProc) Start(context.Context) error {
	f.starts.Add(1)
	time.Sleep(f.delay)
	if f.startErr != nil {
		return f.startErr
	}
	f.running.Store(true)
	return nil
}
func (f *fakeProc) Close() error  { f.closes.Add(1); f.running.Store(false); return nil }
func (f *fakeProc) Port() int     { return f.port }
func (f *fakeProc) Token() string { return "test-token" }
func (f *fakeProc) Status() publish.ExternalProcessStatus {
	if f.running.Load() {
		return publish.StatusRunning
	}
	return publish.StatusStopped
}

// spawnCounter builds fake processes and remembers every one it built.
type spawnCounter struct {
	mu       sync.Mutex
	starts   atomic.Int32
	procs    []*fakeProc
	port     int
	failNext int // the first failNext processes fail to start
	delay    time.Duration
}

func (c *spawnCounter) newProc() supervisedProcess {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := &fakeProc{port: c.port, starts: &c.starts, delay: c.delay}
	if c.failNext > 0 {
		c.failNext--
		p.startErr = errors.New("healthz probe failed")
	}
	c.procs = append(c.procs, p)
	return p
}

func displayServer(t *testing.T) (*httptest.Server, int) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/widget-url", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"http://x.test/widget?token=test-token"}`))
	})
	mux.HandleFunc("/sse/schema-changed", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	_, portStr, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return srv, port
}

// A bridge that is built and closed without being used spawns nothing —
// the one-shot CLI path (f13).
func TestBridge_BootAndCloseSpawnNothing(t *testing.T) {
	c := &spawnCounter{}
	b := newWithSupervisor(newSupervisorEndpoint(c.newProc, context.Background()))
	time.Sleep(50 * time.Millisecond) // give an eager SSE loop time to show itself
	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := len(c.procs); n != 0 {
		t.Fatalf("boot + close built %d display processes, want 0", n)
	}
	if got := b.supervisor.Status(); got != publish.StatusStopped {
		t.Fatalf("Status = %q, want stopped", got)
	}
}

// The first use starts the display process exactly once, even when many
// callers arrive together; Close then stops it and refuses a restart.
func TestBridge_FirstUseStartsOnceAndCloseStops(t *testing.T) {
	_, port := displayServer(t)
	c := &spawnCounter{port: port, delay: 30 * time.Millisecond}
	b := newWithSupervisor(newSupervisorEndpoint(c.newProc, context.Background()))
	t.Cleanup(func() { _ = b.Close() }) // before srv.Close: an open SSE stream blocks it

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := b.GetWidgetURL(context.Background(), api.GetWidgetURLInput{Kind: "k"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("GetWidgetURL: %v", err)
		}
	}
	if n := c.starts.Load(); n != 1 {
		t.Fatalf("16 concurrent first uses started %d processes, want 1", n)
	}

	if err := b.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if n := c.procs[0].closes.Load(); n == 0 {
		t.Fatal("Close after first use did not stop the display process")
	}
	if _, err := b.GetWidgetURL(context.Background(), api.GetWidgetURLInput{Kind: "k"}); err == nil {
		t.Fatal("GetWidgetURL after Close succeeded, want an error")
	}
	if n := c.starts.Load(); n != 1 {
		t.Fatalf("a use after Close started a process (starts=%d)", n)
	}
}

// A failed start is reported to its caller and retried on the next use,
// with a fresh process.
func TestBridge_FailedStartRetriedOnNextUse(t *testing.T) {
	_, port := displayServer(t)
	c := &spawnCounter{port: port, failNext: 1}
	b := newWithSupervisor(newSupervisorEndpoint(c.newProc, context.Background()))
	t.Cleanup(func() { _ = b.Close() })

	if _, err := b.GetWidgetURL(context.Background(), api.GetWidgetURLInput{Kind: "k"}); err == nil {
		t.Fatal("first use with a failing start succeeded, want an error")
	}
	if n := c.procs[0].closes.Load(); n == 0 {
		t.Fatal("the failed process was not closed")
	}
	if _, err := b.GetWidgetURL(context.Background(), api.GetWidgetURLInput{Kind: "k"}); err != nil {
		t.Fatalf("second use: %v", err)
	}
	if n := len(c.procs); n != 2 {
		t.Fatalf("built %d processes, want 2 (one failed, one retried)", n)
	}
}

// Close while the first start is still running stops that process.
func TestEndpoint_CloseDuringStartStopsIt(t *testing.T) {
	c := &spawnCounter{port: 1, delay: 100 * time.Millisecond}
	e := newSupervisorEndpoint(c.newProc, context.Background())
	done := make(chan string)
	go func() { done <- e.BaseURL() }()
	time.Sleep(30 * time.Millisecond)
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := <-done; got != "" {
		t.Fatalf("BaseURL during Close = %q, want empty", got)
	}
	if n := c.procs[0].closes.Load(); n == 0 {
		t.Fatal("Close during start left the process running")
	}
}
