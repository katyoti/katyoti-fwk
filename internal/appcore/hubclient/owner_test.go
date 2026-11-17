// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// TestHelperOwnerSleeps is not a test: the owner process the owned-hub test
// starts and kills (it re-runs this binary with the variable set).
func TestHelperOwnerSleeps(t *testing.T) {
	if os.Getenv("KTW_TEST_OWNER_SLEEP") != "1" {
		t.Skip("helper process")
	}
	time.Sleep(5 * time.Minute)
}

// AN OWNED HUB ENDS ONCE NO CLIENT USES IT, NEVER WHILE ONE DOES: a client
// held keeps it up; with none it waits for a first one (firstClientWait), and
// once clients are gone it is hidden, counted again, and ended; a client
// arriving while it is hidden shows it again and the watch goes on.
func TestEndWhenUnused(t *testing.T) {
	keepPoll, keepFirst := unusedPoll, firstClientWait
	unusedPoll, firstClientWait = 10*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { unusedPoll, firstClientWait = keepPoll, keepFirst })

	var hidden atomic.Bool
	run := func(clients func() (int, int)) (ended bool, hides, shows int32) {
		var h, s int32
		hidden.Store(false)
		done := make(chan struct{})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		go EndWhenUnused(ctx, clients,
			func() bool { atomic.AddInt32(&h, 1); hidden.Store(true); return true },
			func() { atomic.AddInt32(&s, 1); hidden.Store(false) },
			func() { close(done) })
		select {
		case <-done:
			return true, atomic.LoadInt32(&h), atomic.LoadInt32(&s)
		case <-ctx.Done():
			return false, atomic.LoadInt32(&h), atomic.LoadInt32(&s)
		}
	}

	if ended, _, _ := run(func() (int, int) { return 1, 1 }); ended {
		t.Fatal("a hub with a client connected was ended")
	}
	start := time.Now()
	if ended, hides, _ := run(func() (int, int) { return 0, 0 }); !ended || hides != 1 {
		t.Fatalf("a hub no client ever used: ended=%v hides=%d, want ended once hidden", ended, hides)
	}
	if took := time.Since(start); took < firstClientWait {
		t.Fatalf("a hub no client ever used ended after %v, before waiting %v for its first", took, firstClientWait)
	}
	// A client that came and went, even between two counts: ended at once.
	start = time.Now()
	if ended, _, _ := run(func() (int, int) { return 0, 1 }); !ended || time.Since(start) >= firstClientWait {
		t.Fatalf("a hub its last client left: ended=%v after %v, want at once", ended, time.Since(start))
	}
	// A client that arrives while the hub is hidden keeps it up.
	var arrivals int32
	ended, hides, shows := run(func() (int, int) {
		if hidden.Load() && atomic.AddInt32(&arrivals, 1) == 1 {
			return 1, 2
		}
		return 0, 1
	})
	if !ended || hides != 2 || shows != 1 {
		t.Fatalf("a client arriving while hidden: ended=%v hides=%d shows=%d, want shown again once, then hidden and ended", ended, hides, shows)
	}
}

// AN OWNED HUB ENDS CLEANLY WHEN NO CLIENT USES IT, its owner killed with no
// client ever connected — no close, no exit path of its own, as an os.Exit or
// a crash leaves it: the hub shuts down once its wait for a first client is
// over, and removes its
// lockfile. Real hub, skipped in -short mode.
func TestOwnedHubEndsCleanlyOnceNoClientUsesIt(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; skipped in -short mode")
	}
	binary := hubBinary(t)
	dataDir := t.TempDir()

	owner := exec.Command(os.Args[0], "-test.run=TestHelperOwnerSleeps")
	owner.Env = append(os.Environ(), "KTW_TEST_OWNER_SLEEP=1")
	if err := owner.Start(); err != nil {
		t.Fatalf("start the owner: %v", err)
	}
	t.Cleanup(func() { _ = owner.Process.Kill() })

	hub := exec.Command(binary, "serve")
	hub.Env = append(os.Environ(), "KTW_DATA="+dataDir, OwnerEnv+"="+strconv.Itoa(owner.Process.Pid))
	if err := hub.Start(); err != nil {
		t.Fatalf("start the hub: %v", err)
	}
	exited := make(chan struct{})
	go func() { _ = hub.Wait(); close(exited) }()
	t.Cleanup(func() { _ = hub.Process.Kill() })

	deadline := time.Now().Add(60 * time.Second)
	for {
		if lf, _ := ReadLockFile(dataDir); lf != nil && lf.NATSURL != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the hub never wrote its lockfile")
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = owner.Process.Kill()
	_, _ = owner.Process.Wait()

	select {
	case <-exited:
	case <-time.After(30 * time.Second):
		t.Fatal("an owned hub with no client was still up 30s after its owner died")
	}
	if lf, _ := ReadLockFile(dataDir); lf != nil {
		t.Fatalf("the hub ended but left its lockfile: %+v", lf)
	}
}
