// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cmapi "github.com/katyoti/katyoti-fwk/internal/extensions/connmgr/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	ktransport "github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/jsonrpc"
	natsxport "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats"
	natsembedded "github.com/katyoti/katyoti-fwk/internal/kts/transport/nats/embedded"
)

// start_backoff_test.go — a member that cannot come up used to be started
// again by every caller that missed it: each start paid the whole readiness
// wait, was killed, and the next caller started it again (a restart loop).
// After a failed start the member is now refused new starts for a back-off
// that doubles per failed start (2 s, 4 s, 8 s … 60 s) and that a successful
// start clears.

// fakeClock moves the manager's clock by hand.
func fakeClock(t *testing.T) func(time.Duration) {
	t.Helper()
	var mu sync.Mutex
	at := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	prev := now
	now = func() time.Time { mu.Lock(); defer mu.Unlock(); return at }
	t.Cleanup(func() { now = prev })
	return func(d time.Duration) { mu.Lock(); at = at.Add(d); mu.Unlock() }
}

// scriptedSpawns answers each spawn with the next outcome (nil = ready).
func scriptedSpawns(t *testing.T, outcomes ...error) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	prev := spawnAttempt
	spawnAttempt = func(context.Context, cmapi.SpawnSpec, *BusScope) (ktsp.Sender, *ktransport.Session, *exec.Cmd, error) {
		i := int(calls.Add(1)) - 1
		if i < len(outcomes) && outcomes[i] != nil {
			return nil, nil, nil, outcomes[i]
		}
		return ktsp.NewSender(nil), nil, nil, nil
	}
	t.Cleanup(func() { spawnAttempt = prev })
	return &calls
}

// refuseEveryBackoff makes every call in a back-off window refused, none
// waited out (start_detached_test.go tests the wait).
func refuseEveryBackoff(t *testing.T) {
	t.Helper()
	prev := maxBackoffWait
	maxBackoffWait = 0
	t.Cleanup(func() { maxBackoffWait = prev })
}

func TestStartBackoff_GrowsPerFailedStartAndASuccessClearsIt(t *testing.T) {
	advance := fakeClock(t)
	refuseEveryBackoff(t)
	notReady := errors.New("ready-probe ktw-x: readiness deadline elapsed")
	calls := scriptedSpawns(t, notReady, notReady, notReady, nil, notReady)
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-x", Transport: "stdio", Binary: "ktw-x"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	acquire := func() error { _, err := m.Acquire(ctx, "ktw-x"); return err }

	// Failed start 1 → 2 s back-off.
	if err := acquire(); err == nil || errors.Is(err, ErrStartBackoff) {
		t.Fatalf("first start: %v, want the spawn's failure", err)
	}
	for _, step := range []struct {
		wait    time.Duration
		refused bool
		spawns  int32
	}{
		{0, true, 1},                        // at once: refused, no process started
		{1900 * time.Millisecond, true, 1},  // still within 2 s
		{200 * time.Millisecond, false, 2},  // 2.1 s: started again, fails → 4 s
		{3 * time.Second, true, 2},          // within 4 s
		{1100 * time.Millisecond, false, 3}, // fails → 8 s
		{7 * time.Second, true, 3},
		{1100 * time.Millisecond, false, 4}, // this start succeeds
	} {
		advance(step.wait)
		err := acquire()
		if refused := errors.Is(err, ErrStartBackoff); refused != step.refused {
			t.Fatalf("after +%s: err %v, refused=%v want %v", step.wait, err, refused, step.refused)
		}
		if n := calls.Load(); n != step.spawns {
			t.Fatalf("after +%s: %d starts, want %d", step.wait, n, step.spawns)
		}
	}
	if st, _ := m.Status("ktw-x"); st.State != cmapi.ConnReady || !st.RetryAfter.IsZero() {
		t.Fatalf("after the successful start: %+v", st)
	}

	// The success cleared the back-off: the next failure opens 2 s, not 16 s.
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := acquire(); err == nil {
		t.Fatal("fifth start: want its failure")
	}
	st, _ := m.Status("ktw-x")
	if got := st.RetryAfter.Sub(now()); got != startBackoffBase {
		t.Fatalf("back-off after a success then a failure = %s, want %s", got, startBackoffBase)
	}
}

func TestStartBackoff_IsCapped(t *testing.T) {
	d := time.Duration(0)
	var seen []time.Duration
	for i := 0; i < 8; i++ {
		d = nextStartBackoff(d)
		seen = append(seen, d)
	}
	want := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		32 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("back-offs %v, want %v", seen, want)
		}
	}
}

// A caller giving up (its context cancelled) is not the member failing: no
// back-off opens.
func TestStartBackoff_NotOpenedByTheCallersCancellation(t *testing.T) {
	fakeClock(t)
	calls, release := heldSpawns(t, nil)
	m, err := NewManager([]cmapi.SpawnSpec{{Name: "ktw-x", Transport: "stdio", Binary: "ktw-x"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := m.Acquire(ctx, "ktw-x"); done <- err }()
	for calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("want the cancelled start's error")
	}
	close(release)
	if _, err := m.Acquire(context.Background(), "ktw-x"); errors.Is(err, ErrStartBackoff) {
		t.Fatalf("a caller's cancellation opened a back-off: %v", err)
	}
}

// The real thing: a member process that starts but never answers its
// readiness ping (the fake sidecar, subscribed in another universe). Its
// declared readiness limit is honoured — the start fails after 300 ms, not the
// default 10 s — and the next callers are refused without a second process,
// logged once for the window.
func TestStartBackoff_NeverReadySidecarIsNotRestartedInALoop(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	hub, err := natsembedded.StartEmbedded(natsembedded.EmbeddedOptions(filepath.Join(t.TempDir(), "js"), true))
	if err != nil {
		t.Fatalf("StartEmbedded: %v", err)
	}
	defer hub.Stop()
	t.Setenv("KTW_NATS_URL", hub.Server.ClientURL())
	refuseEveryBackoff(t)
	binary := buildFakeSidecar(t)
	// Run the fresh executable once (it exits at once without a bus URL) so
	// the OS's first-run scan of a new build is not timed below.
	warm := exec.Command(binary)
	warm.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
	_ = warm.Run()

	var starts atomic.Int32
	prev := spawnAttempt
	spawnAttempt = func(ctx context.Context, spec cmapi.SpawnSpec, bus *BusScope) (ktsp.Sender, *ktransport.Session, *exec.Cmd, error) {
		starts.Add(1)
		return prev(ctx, spec, bus)
	}
	t.Cleanup(func() { spawnAttempt = prev })

	var logs bytes.Buffer
	prevLog := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLog) })

	m, err := NewManagerWithBus([]cmapi.SpawnSpec{{
		Name: "fake", Transport: "nats", Binary: binary,
		Env:          map[string]string{"KTW_UNIVERSE_ID": "another-universe"}, // never answers here
		ReadyTimeout: 300 * time.Millisecond,
	}}, BusScope{Bus: natsxport.New(hub.Client), TenantID: "local", UniverseID: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := m.Acquire(ctx, "fake"); err == nil || errors.Is(err, ErrStartBackoff) {
		t.Fatalf("first start: %v, want a readiness failure", err)
	}
	// The executable's first start can wait on the OS (a scan of a fresh
	// build); the readiness wait itself is the declared 300 ms, far below 10 s.
	took := time.Since(start)
	t.Logf("the never-ready start failed after %s", took)
	if took >= cmapi.DefaultReadyTimeout {
		t.Errorf("the start took %s: the declared readiness limit (300ms) was not honoured", took)
	}
	for i := 0; i < 5; i++ {
		if _, err := m.Acquire(ctx, "fake"); !errors.Is(err, ErrStartBackoff) {
			t.Fatalf("call %d after the failed start: %v, want a back-off refusal", i, err)
		}
	}
	if n := starts.Load(); n != 1 {
		t.Fatalf("%d starts of a member that never became ready, want 1 (no restart loop)", n)
	}
	if n := strings.Count(logs.String(), "start refused during back-off"); n != 1 {
		t.Errorf("refusal logged %d times in one window, want once:\n%s", n, logs.String())
	}
	st, _ := m.Status("fake")
	if st.State != cmapi.ConnFailed || st.RetryAfter.IsZero() {
		t.Errorf("status during the back-off: %+v", st)
	}
}
