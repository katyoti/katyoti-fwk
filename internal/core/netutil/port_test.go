// SPDX-License-Identifier: Apache-2.0

package netutil

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
)

func TestAllocFreePort_ReturnsUsablePort(t *testing.T) {
	port, err := AllocFreePort()
	if err != nil {
		t.Fatalf("AllocFreePort: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Fatalf("port out of range: %d", port)
	}
	// The kernel released the port; we should be able to bind to it (race-prone but fine for test).
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("rebind to allocated port %d: %v", port, err)
	}
	_ = l.Close()
}

func TestAllocFreePort_ConcurrentUnique(t *testing.T) {
	const n = 5
	ports := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			p, err := AllocFreePort()
			if err != nil {
				t.Errorf("alloc[%d]: %v", i, err)
				return
			}
			ports[i] = p
		}(i)
	}
	wg.Wait()
	seen := make(map[int]bool)
	for _, p := range ports {
		if p == 0 {
			t.Fatalf("zero port in result")
		}
		if seen[p] {
			t.Errorf("duplicate port %d", p)
		}
		seen[p] = true
	}
}

func TestAllocFreePortWithRetry_SucceedsFirstTry(t *testing.T) {
	called := 0
	port, err := AllocFreePortWithRetry(3, func(p int) error {
		called++
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if port == 0 {
		t.Errorf("got zero port")
	}
	if called != 1 {
		t.Errorf("spawnFn called %d times, want 1", called)
	}
}

func TestAllocFreePortWithRetry_RetriesOnError(t *testing.T) {
	called := 0
	want := errors.New("EADDRINUSE-ish")
	port, err := AllocFreePortWithRetry(3, func(p int) error {
		called++
		if called < 2 {
			return want
		}
		return nil
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if port == 0 {
		t.Errorf("got zero port")
	}
	if called != 2 {
		t.Errorf("spawnFn called %d times, want 2 (one retry)", called)
	}
}

func TestAllocFreePortWithRetry_ExhaustsAfterMaxRetries(t *testing.T) {
	called := 0
	_, err := AllocFreePortWithRetry(3, func(p int) error {
		called++
		return errors.New("always fails")
	})
	if err == nil {
		t.Fatalf("expected error after exhaustion, got nil")
	}
	if called != 3 {
		t.Errorf("spawnFn called %d times, want 3", called)
	}
}

func TestAllocFreePortWithRetry_TreatsZeroAsOne(t *testing.T) {
	called := 0
	_, err := AllocFreePortWithRetry(0, func(p int) error {
		called++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if called != 1 {
		t.Errorf("spawnFn called %d times, want 1 (zero clamped to 1)", called)
	}
}
