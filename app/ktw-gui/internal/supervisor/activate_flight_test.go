// SPDX-License-Identifier: Apache-2.0

package supervisor

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// At a restart the frontend activates the same universe from several calls at
// once. They must not each connect to (or spawn) its hub: one does, the others
// wait for it.
func TestActivate_OneUniverseConnectsOnceAtATime(t *testing.T) {
	m := NewMaster()
	var inFlight, maxInFlight, starts int32
	m.start = func(ctx context.Context, name string) (*ActiveCore, error) {
		atomic.AddInt32(&starts, 1)
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxInFlight)
			if n <= old || atomic.CompareAndSwapInt32(&maxInFlight, old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		// A failed start: nothing is installed, and each waiter leads a retry
		// in turn — never several at once.
		return nil, errors.New("hub did not come up")
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, err := m.Activate(context.Background(), "admin"); err == nil {
				t.Error("Activate: no error, want the start's failure")
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&maxInFlight); got != 1 {
		t.Fatalf("concurrent starts of one universe = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&starts); got < 1 {
		t.Fatalf("starts = %d, want at least one", got)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.activating) != 0 {
		t.Fatalf("activations left in progress: %v", m.activating)
	}
}

// A caller whose context ends stops waiting for the activation in progress.
func TestActivate_AWaiterHonoursItsContext(t *testing.T) {
	m := NewMaster()
	release := make(chan struct{})
	m.start = func(ctx context.Context, name string) (*ActiveCore, error) {
		<-release
		return nil, errors.New("stopped")
	}
	go func() { _, _ = m.Activate(context.Background(), "u") }()
	for {
		m.mu.RLock()
		_, busy := m.activating["u"]
		m.mu.RUnlock()
		if busy {
			break
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.Activate(ctx, "u"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter err = %v, want its deadline", err)
	}
	close(release)
}
