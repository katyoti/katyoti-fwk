// SPDX-License-Identifier: Apache-2.0

package proclease

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// : a process that lost its lease - another live process took it
// over, judging it dead or wedged - learns it and stops writing.

// takeOver writes the lease file as another live process would on taking it
// over: its PID (this process's parent, alive while the test runs), a fresh
// heartbeat.
func takeOver(t *testing.T, file string) {
	t.Helper()
	rec := lockRecord{PID: os.Getppid(), StartedAt: time.Now().UTC(), LeaseTTL: "30s", HeartbeatAt: time.Now().UTC()}
	data, _ := json.Marshal(rec)
	if err := os.WriteFile(file, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestALeaseTakenOverIsLost(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".lock")
	l := New(file)
	if err := l.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := l.Check(); err != nil {
		t.Fatalf("the holder's Check = %v, want nil", err)
	}
	takeOver(t, file)
	if err := l.Check(); !errors.Is(err, ErrLost) {
		t.Fatalf("Check after a takeover = %v, want ErrLost", err)
	}
	// Release leaves the new holder's lease in place.
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("Release removed the lease another process holds: %v", err)
	}
}

func TestTheHeartbeatSeesATakeover(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".lock")
	l := New(file)
	l.SetLeaseTTL(300 * time.Millisecond)
	if err := l.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	takeOver(t, file)
	deadline := time.Now().Add(3 * time.Second)
	for !l.lost.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !l.lost.Load() {
		t.Fatal("the heartbeat did not see the takeover")
	}
	// It stopped beating: the new holder's record is untouched.
	b1, _ := os.ReadFile(file)
	time.Sleep(400 * time.Millisecond)
	if b2, _ := os.ReadFile(file); string(b1) != string(b2) {
		t.Fatal("the heartbeat kept writing a lease it lost")
	}
}

func TestALeaseNeverAcquiredGuardsNothing(t *testing.T) {
	if err := New(filepath.Join(t.TempDir(), ".lock")).Check(); err != nil {
		t.Fatalf("Check on a lease never acquired = %v", err)
	}
}
