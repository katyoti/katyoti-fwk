// SPDX-License-Identifier: Apache-2.0

package proclease

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// : the lease carries an epoch - a take-over writes one above
// the record it replaces, the holder keeps it, and a holder whose epoch moved
// lost the lease even under its own PID.

// AC-008.
func TestATakeOverMovesTheEpochAndTheHoldersBeatsKeepIt(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, ".lock")

	// A missing file starts at 1; a repeated Acquire and the heartbeats keep it.
	a := New(file)
	a.SetLeaseTTL(300 * time.Millisecond)
	if err := a.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if got := readLockRecord(t, root).Epoch; got != 1 {
		t.Fatalf("epoch on a missing file = %d, want 1", got)
	}
	if err := a.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	beat := readLockRecord(t, root).HeartbeatAt
	deadline := time.Now().Add(3 * time.Second)
	for readLockRecord(t, root).HeartbeatAt.Equal(beat) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if rec := readLockRecord(t, root); rec.HeartbeatAt.Equal(beat) || rec.Epoch != 1 {
		t.Fatalf("after the holder's heartbeat and a repeated Acquire: %+v, want a new beat at epoch 1", rec)
	}
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}

	// A take-over writes one above the record it replaces (a dead holder at 5).
	writeLockRecord(t, root, lockRecord{PID: 999999, Epoch: 5, StartedAt: time.Now().UTC(), LeaseTTL: "30s", HeartbeatAt: time.Now().UTC()})
	b := New(file)
	if err := b.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if got := readLockRecord(t, root).Epoch; got != 6 {
		t.Fatalf("epoch after taking over epoch 5 = %d, want 6", got)
	}
	_ = b.Release()

	// A record written before the epoch counts as 0.
	writeLockRecord(t, root, lockRecord{PID: 999999, StartedAt: time.Now().UTC(), LeaseTTL: "30s", HeartbeatAt: time.Now().UTC()})
	c := New(file)
	if err := c.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if got := readLockRecord(t, root).Epoch; got != 1 {
		t.Fatalf("epoch after taking over a pre-epoch record = %d, want 1", got)
	}
	_ = c.Release()
}

// AC-009.
func TestAHolderWhoseEpochMovedIsLostEvenUnderItsOwnPID(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), ".lock")
	a := New(file)
	if err := a.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	// Another instance of this very process takes the lease over: same PID.
	b := New(file)
	if err := b.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	defer b.Release()
	if err := a.Check(); !errors.Is(err, ErrLost) {
		t.Fatalf("the first holder's Check after a same-PID take-over = %v, want ErrLost", err)
	}
	if err := b.Check(); err != nil {
		t.Fatalf("the taker's Check = %v, want nil", err)
	}

	// A taker the OS gave the holder's PID: the file names this PID at a
	// higher epoch.
	c := New(filepath.Join(t.TempDir(), ".lock"))
	if err := c.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	defer c.Release()
	rec, err := c.readRecord()
	if err != nil || rec == nil {
		t.Fatal(rec, err)
	}
	rec.Epoch++
	if err := c.writeRecord(*rec); err != nil {
		t.Fatal(err)
	}
	if err := c.Check(); !errors.Is(err, ErrLost) {
		t.Fatalf("Check with its PID at a higher epoch = %v, want ErrLost", err)
	}
}

// AC-010.
func TestALostHolderNeitherBeatsNorRemovesTheTakersLease(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, ".lock")
	a := New(file)
	a.SetLeaseTTL(300 * time.Millisecond)
	if err := a.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	b := New(file)
	if err := b.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	defer b.Release()

	// The first holder's heartbeat sees the take-over and stops: it never
	// writes its epoch back.
	deadline := time.Now().Add(3 * time.Second)
	for !a.lost.Load() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !a.lost.Load() {
		t.Fatal("the first holder's heartbeat did not see the same-PID take-over")
	}
	if got := readLockRecord(t, root).Epoch; got != 2 {
		t.Fatalf("epoch after the lost holder's heartbeats = %d, want the taker's 2", got)
	}

	// Its Release leaves the taker's lease in place.
	if err := a.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the lost holder's Release removed the taker's lease: %v", err)
	}
	if err := b.Check(); err != nil {
		t.Fatalf("the taker's Check after the lost holder's Release = %v", err)
	}
}
