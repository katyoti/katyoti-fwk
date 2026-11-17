// SPDX-License-Identifier: Apache-2.0

package core

// Restart-policy tests for decideRetryLocked. The policy is exercised
// in isolation here (the full retry loop in Acquire is exercised by
// the spawn-failure tests in manager_test.go indirectly).

import (
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
)

func newConn(spec cmapi.SpawnSpec) *stdioConnection {
	return &stdioConnection{spec: spec}
}

func TestRestart_DefaultPolicyIsFailFast(t *testing.T) {
	c := newConn(cmapi.SpawnSpec{Name: "x", Transport: "stdio"})
	retry, _ := decideRetryLocked(c, time.Now())
	if retry {
		t.Error("default policy (MaxRestarts=0) should not retry")
	}
}

func TestRestart_RemoteNeverRetries(t *testing.T) {
	c := newConn(cmapi.SpawnSpec{
		Name:        "x",
		Transport:   "remote",
		MaxRestarts: 5, // even with retries configured
	})
	retry, _ := decideRetryLocked(c, time.Now())
	if retry {
		t.Error("transport=remote should never retry (sidecar lifecycle owned elsewhere)")
	}
}

func TestRestart_RetriesUpToMax(t *testing.T) {
	c := newConn(cmapi.SpawnSpec{
		Name:             "x",
		Transport:        "stdio",
		MaxRestarts:      3,
		MaxRestartWindow: time.Hour, // generous, won't expire mid-test
	})
	now := time.Now()

	// 3 retries succeed; 4th call returns retry=false.
	for i := 0; i < 3; i++ {
		retry, backoff := decideRetryLocked(c, now)
		if !retry {
			t.Errorf("attempt %d: expected retry=true, got false", i+1)
		}
		if backoff <= 0 {
			t.Errorf("attempt %d: expected positive backoff, got %v", i+1, backoff)
		}
	}
	retry, _ := decideRetryLocked(c, now)
	if retry {
		t.Error("4th call should return retry=false (max exhausted)")
	}
}

func TestRestart_BackoffIsExponential(t *testing.T) {
	c := newConn(cmapi.SpawnSpec{
		Name:             "x",
		Transport:        "stdio",
		MaxRestarts:      5,
		MaxRestartWindow: time.Hour,
	})
	now := time.Now()
	want := []time.Duration{
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
		2 * time.Second, // capped
	}
	for i, w := range want {
		_, got := decideRetryLocked(c, now)
		if got != w {
			t.Errorf("attempt %d: backoff = %v, want %v", i+1, got, w)
		}
	}
}

func TestRestart_WindowExpiryResetsCounter(t *testing.T) {
	c := newConn(cmapi.SpawnSpec{
		Name:             "x",
		Transport:        "stdio",
		MaxRestarts:      2,
		MaxRestartWindow: 100 * time.Millisecond, // tiny window
	})

	t0 := time.Now()
	// Exhaust the window's retries.
	if r, _ := decideRetryLocked(c, t0); !r {
		t.Fatal("attempt 1 should retry")
	}
	if r, _ := decideRetryLocked(c, t0); !r {
		t.Fatal("attempt 2 should retry")
	}
	if r, _ := decideRetryLocked(c, t0); r {
		t.Fatal("attempt 3 should be exhausted")
	}

	// Advance past the window — counter resets.
	t1 := t0.Add(200 * time.Millisecond)
	if r, _ := decideRetryLocked(c, t1); !r {
		t.Error("after window expiry: expected fresh retry budget")
	}
	if c.restartCount != 1 {
		t.Errorf("after window expiry: restartCount = %d, want 1", c.restartCount)
	}
}

func TestRestart_ZeroWindowUsesDefault(t *testing.T) {
	// MaxRestartWindow == 0 should use the 5-minute default, not be
	// treated as "never expires" or "always expired".
	c := newConn(cmapi.SpawnSpec{
		Name:             "x",
		Transport:        "stdio",
		MaxRestarts:      2,
		MaxRestartWindow: 0,
	})
	t0 := time.Now()
	if r, _ := decideRetryLocked(c, t0); !r {
		t.Fatal("first attempt should retry")
	}
	// 4 minutes later — within the 5min default window — counter
	// should still be incrementing.
	t1 := t0.Add(4 * time.Minute)
	if r, _ := decideRetryLocked(c, t1); !r {
		t.Fatal("second attempt at t+4min should retry")
	}
	// 6 minutes after t0 = 2 minutes past the implicit 5min window.
	// Counter should NOT auto-reset because the window started at t0
	// and stays open for 5min from t0 — at t+6min it expires only
	// after we evaluate the next attempt.
	t2 := t0.Add(6 * time.Minute)
	if r, _ := decideRetryLocked(c, t2); !r {
		t.Error("after default-window expiry: expected fresh retry budget")
	}
}
