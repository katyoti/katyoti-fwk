// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// A second process for the same provider and universe is refused — and told
// which process serves; another provider of the same universe is not.
func TestASecondServingProcessIsRefused(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	first, err := acquireServingLock(ctx, dir, "ktw-agents", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unlock()
	start := time.Now()
	_, err = acquireServingLock(ctx, dir, "ktw-agents", 300*time.Millisecond)
	if err == nil {
		t.Fatal("a second serving process got the lock")
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("pid %d", os.Getpid())) {
		t.Errorf("error %q does not say which process serves", err)
	}
	if time.Since(start) < 300*time.Millisecond {
		t.Error("it did not wait for a draining predecessor")
	}
	other, err := acquireServingLock(ctx, dir, "ktw-docs", time.Second)
	if err != nil {
		t.Fatalf("another provider of the universe was refused: %v", err)
	}
	_ = other.Unlock()
}

// A process starting while its predecessor drains gets the lock once the
// predecessor lets it go.
func TestAServingProcessTakesOverFromADrainingOne(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	first, err := acquireServingLock(ctx, dir, "ktw-agents", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	go func() { time.Sleep(300 * time.Millisecond); _ = first.Unlock() }()
	second, err := acquireServingLock(ctx, dir, "ktw-agents", 5*time.Second)
	if err != nil {
		t.Fatalf("after its predecessor let go: %v", err)
	}
	_ = second.Unlock()
}

type fakeLock struct{ held atomic.Bool }

func (f *fakeLock) StillHeld() bool { return f.held.Load() }

// A serving process whose lock no longer holds its path stops serving: the
// watch says so once.
func TestALostServingLockStopsServing(t *testing.T) {
	l := &fakeLock{}
	l.held.Store(true)
	var lost atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { watchServingLock(ctx, l, 10*time.Millisecond, func() { lost.Add(1) }); close(done) }()
	time.Sleep(50 * time.Millisecond)
	if lost.Load() != 0 {
		t.Fatal("a held lock was reported lost")
	}
	l.held.Store(false)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a lost lock was not reported")
	}
	if lost.Load() != 1 {
		t.Errorf("reported %d times; once", lost.Load())
	}
}
