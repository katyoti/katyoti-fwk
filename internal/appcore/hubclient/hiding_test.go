// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// A COMMAND ARRIVING WHILE AN OWNED HUB HIDES JOINS IT: the
// hub hides its lockfile under the spawn guard, so the command that finds no
// lockfile queues behind the hub's decision instead of starting a second hub;
// announced as waiting, it makes the hub show itself again, and is served by
// it. Before, the command started a hub of its own in that window, and the
// first one ran on unregistered, its members holding the serving locks.
func TestACommandArrivingWhileTheHubHidesJoinsItAndStartsNoSecondHub(t *testing.T) {
	asHub(t)
	keepPoll := unusedPoll
	unusedPoll = 400 * time.Millisecond
	t.Cleanup(func() { unusedPoll = keepPoll })

	dir := t.TempDir()
	bus := startBus(t)
	var spawns int32
	fakeSpawn(t, &spawns, nil)
	write := func() error {
		return ClaimLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: bus.Server.ClientURL()})
	}
	if err := write(); err != nil {
		t.Fatal(err)
	}

	// The hub: its last client gone, it hides at the first poll; then it
	// counts the commands on their way to it (Waiting).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ended := make(chan struct{})
	var shows int32
	go EndOwnedWhenUnused(ctx, dir, os.Getpid(),
		func() (int, int) { return Waiting(dir), 1 },
		func() error { atomic.AddInt32(&shows, 1); return write() },
		func() { close(ended) })

	// The command arrives while the lockfile is hidden.
	for deadline := time.Now().Add(5 * time.Second); ; {
		if lf, _ := ReadLockFile(dir); lf == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the hub never hid its lockfile")
		}
		time.Sleep(5 * time.Millisecond)
	}
	c, err := ConnectToHub(ctx, ConnectOptions{DataDir: dir, Mode: ModeFindOrSpawn})
	if err != nil {
		t.Fatalf("the command arriving while the hub hid failed: %v", err)
	}
	if got := atomic.LoadInt32(&spawns); got != 0 {
		t.Fatalf("the command started %d hub(s) beside the hiding one, want 0 - it must queue and join it", got)
	}
	if c.LockFile().PID != os.Getpid() || atomic.LoadInt32(&shows) == 0 {
		t.Fatalf("the command was not served by the hub shown again: lockfile %+v, shows %d", c.LockFile(), shows)
	}
	_ = c.Close()

	// The command gone, the hub hides again and ends — keeping the guard
	// (its process ends): nothing else removed it.
	select {
	case <-ended:
	case <-ctx.Done():
		t.Fatal("the hub did not end once the command left")
	}
	if _, got, _ := acquireSpawnGuard(dir); got {
		t.Fatal("the ending hub let the spawn guard go before its process ended")
	}
}

// A hub never hides while a command holds the spawn guard: that command is
// starting a hub or re-checking for this one, and would miss it.
func TestAHubDoesNotHideWhileACommandHoldsTheSpawnGuard(t *testing.T) {
	asHub(t)
	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	guard, got, err := acquireSpawnGuard(dir)
	if err != nil || !got {
		t.Fatalf("take the guard: got=%v err=%v", got, err)
	}
	if _, ok := HideOwnLockFile(dir, os.Getpid()); ok {
		t.Fatal("the hub hid while a command held the spawn guard")
	}
	if lf, _ := ReadLockFile(dir); lf == nil {
		t.Fatal("a refused hide removed the lockfile")
	}
	guard.release()
	h, ok := HideOwnLockFile(dir, os.Getpid())
	if !ok {
		t.Fatal("the hub could not hide once the guard was free")
	}
	if lf, _ := ReadLockFile(dir); lf != nil {
		t.Fatalf("hidden, the lockfile is still there: %+v", lf)
	}
	if err := h.Show(func() error {
		return WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:1"})
	}); err != nil {
		t.Fatal(err)
	}
	if _, got, _ := acquireSpawnGuard(dir); !got {
		t.Fatal("shown again, the hub kept the spawn guard")
	}
}

// A HUB ANOTHER LIVE HUB'S LOCKFILE NAMES THE UNIVERSE OVER ENDS within a
// bound, whatever made it unregistered: its members let their serving locks
// go. A missing lockfile (hiding) or its own keeps it.
func TestAHubSupplantedByAnotherRegisteredHubEnds(t *testing.T) {
	keepPoll, keepGrace := supplantedPoll, supplantedGrace
	supplantedPoll, supplantedGrace = 20*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { supplantedPoll, supplantedGrace = keepPoll, keepGrace })
	other := os.Getpid()
	prev := hubProcess
	hubProcess = func(pid int) bool { return pid == other }
	t.Cleanup(func() { hubProcess = prev })
	self := other + 1 // this "hub": any pid but the registered one

	watch := func(dir string, within time.Duration) bool {
		ctx, cancel := context.WithTimeout(context.Background(), within)
		defer cancel()
		ended := make(chan struct{})
		go EndWhenSupplanted(ctx, dir, self, func() { close(ended) })
		select {
		case <-ended:
			return true
		case <-ctx.Done():
			return false
		}
	}

	dir := t.TempDir()
	if watch(dir, 400*time.Millisecond) {
		t.Fatal("a hub with no lockfile (hiding) ended as supplanted")
	}
	if err := WriteLockFile(dir, LockFile{PID: self, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	if watch(dir, 400*time.Millisecond) {
		t.Fatal("a hub its own lockfile names ended as supplanted")
	}
	if err := WriteLockFile(dir, LockFile{PID: other, NATSURL: "nats://127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	if !watch(dir, 3*time.Second) {
		t.Fatal("a hub another live hub's lockfile replaced ran on - its members keep the serving locks")
	}
}
