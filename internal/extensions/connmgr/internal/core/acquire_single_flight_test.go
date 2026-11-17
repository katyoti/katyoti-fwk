// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
)

// heldSpawns replaces the spawn with one that counts its calls and blocks
// until release is closed, then answers what result says — no process runs.
func heldSpawns(t *testing.T, result error) (calls *atomic.Int32, release chan struct{}) {
	t.Helper()
	calls = &atomic.Int32{}
	release = make(chan struct{})
	prev := spawnAttempt
	spawnAttempt = func(ctx context.Context, _ cmapi.SpawnSpec, _ *BusScope) (ktsp.Sender, *ktransport.Session, *exec.Cmd, error) {
		calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		}
		if result != nil {
			return nil, nil, nil, result
		}
		return ktsp.NewSender(nil), nil, nil, nil
	}
	t.Cleanup(func() { spawnAttempt = prev })
	return calls, release
}

// acquireAll runs n Acquire of one member at once and returns their errors
// once the spawn they wait on is released.
func acquireAll(t *testing.T, m *ManagerImpl, name string, n int, calls *atomic.Int32, release chan struct{}) []error {
	t.Helper()
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = m.Acquire(context.Background(), name)
		}(i)
	}
	// Every caller is in: one holds the spawn, the others wait on it.
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	return errs
}

// Several callers missing the same cold member at once start ONE process:
// the others wait for it and share it. Each used to spawn its own copy, and
// the copies fought over what the member holds alone (ktw-agents' CWM
// workspace lock) — the losers died at boot, at every GUI start.
func TestAcquire_ConcurrentCallersShareOneSpawn(t *testing.T) {
	calls, release := heldSpawns(t, nil)
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-agents", Transport: "stdio", Binary: "ktw-agents"}})
	if err != nil {
		t.Fatal(err)
	}
	for i, err := range acquireAll(t, m, "ktw-agents", 5, calls, release) {
		if err != nil {
			t.Errorf("caller %d: %v", i, err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("spawns = %d, want 1 (one process per member)", n)
	}
	st, _ := m.Status("ktw-agents")
	if st.State != cmapi.ConnReady || st.RefCount != 5 {
		t.Errorf("status = %s refcount %d, want READY refcount 5", st.State, st.RefCount)
	}
}

// A spawn that fails fails every caller that waited on it — none of them
// starts a process of its own on the back of it.
func TestAcquire_ConcurrentCallersShareAFailedSpawn(t *testing.T) {
	advance := fakeClock(t)
	calls, release := heldSpawns(t, errors.New("ready-probe: readiness deadline elapsed"))
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-agents", Transport: "stdio", Binary: "ktw-agents"}})
	if err != nil {
		t.Fatal(err)
	}
	for i, err := range acquireAll(t, m, "ktw-agents", 4, calls, release) {
		if err == nil {
			t.Errorf("caller %d: no error, want the spawn's failure", i)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("spawns = %d, want 1", n)
	}
	// The next caller, once the failure settled and its back-off elapsed,
	// tries again (start_backoff_test.go).
	advance(startBackoffBase)
	calls2, release2 := heldSpawns(t, nil)
	close(release2)
	if _, err := m.Acquire(context.Background(), "ktw-agents"); err != nil {
		t.Fatalf("Acquire after a failed spawn: %v", err)
	}
	if n := calls2.Load(); n != 1 {
		t.Errorf("retry spawns = %d, want 1", n)
	}
}

// A caller waiting on another's spawn gives up with its own context, and
// leaves the spawn running for the others.
func TestAcquire_WaiterHonoursItsContext(t *testing.T) {
	calls, release := heldSpawns(t, nil)
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-agents", Transport: "stdio", Binary: "ktw-agents"}})
	if err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { _, err := m.Acquire(context.Background(), "ktw-agents"); first <- err }()
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := m.Acquire(ctx, "ktw-agents"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waiter err = %v, want its context's deadline", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Errorf("spawning caller: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("spawns = %d, want 1", n)
	}
}
