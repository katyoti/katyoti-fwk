// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// A LIVE HUB'S LOCKFILE IS NEVER REMOVED ON A WRONG "DEAD": a
// client whose liveness probe calls the running hub dead - the hub still in
// the system's process list as a ktw-hub - neither removes its lockfile nor
// starts a second hub beside it.
func TestALiveHubTheProbeCallsDeadKeepsItsLockFileAndNoHubIsStarted(t *testing.T) {
	self := os.Getpid()
	prevProbe, prevListed, prevSpawn, prevWait := liveProbe, listedHub, spawnHub, slowHubWait
	liveProbe = func(int) bool { return false } // the probe lies
	listedHub = func(pid int) bool { return pid == self }
	slowHubWait = 300 * time.Millisecond
	var spawns int32
	spawnHub = func(context.Context, ConnectOptions) (*HubClient, error) {
		atomic.AddInt32(&spawns, 1)
		return nil, errors.New("spawned")
	}
	t.Cleanup(func() { liveProbe, listedHub, spawnHub, slowHubWait = prevProbe, prevListed, prevSpawn, prevWait })
	asHub(t)

	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: self, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := ConnectToHub(ctx, ConnectOptions{DataDir: dir, Mode: ModeFindOrSpawn})
	if !errors.Is(err, ErrHubNotAnswering) {
		t.Fatalf("err = %v, want ErrHubNotAnswering (the live hub reported, not replaced)", err)
	}
	if n := atomic.LoadInt32(&spawns); n != 0 {
		t.Fatalf("%d hub(s) started beside the live one", n)
	}
	if lf, _ := ReadLockFile(dir); lf == nil || lf.PID != self {
		t.Fatalf("the live hub's lockfile was removed or replaced: %+v", lf)
	}
	// A plain reuse (the commands that never spawn) keeps it too.
	if _, err := ConnectToHub(ctx, ConnectOptions{DataDir: dir, Mode: ModeReuse}); errors.Is(err, ErrStaleLockfile) {
		t.Fatalf("a reuse called the live hub stale: %v", err)
	}
	if lf, _ := ReadLockFile(dir); lf == nil {
		t.Fatal("a reuse removed the live hub's lockfile")
	}
}

// removeDeadLockFile removes a lockfile naming a dead hub, and leaves one
// naming a live hub or none.
func TestRemoveDeadLockFileRemovesOnlyADeadHubsLockFile(t *testing.T) {
	self := os.Getpid()
	prevProbe, prevListed := liveProbe, listedHub
	liveProbe = func(pid int) bool { return pid == self }
	listedHub = func(int) bool { return false }
	t.Cleanup(func() { liveProbe, listedHub = prevProbe, prevListed })

	dir := t.TempDir()
	if removeDeadLockFile(dir) {
		t.Fatal("removed a lockfile that is not there")
	}
	if err := WriteLockFile(dir, LockFile{PID: self, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	if removeDeadLockFile(dir) {
		t.Fatal("removed a live hub's lockfile")
	}
	if err := WriteLockFile(dir, LockFile{PID: self + 1000000, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	if !removeDeadLockFile(dir) {
		t.Fatal("kept a dead hub's lockfile")
	}
	if lf, _ := ReadLockFile(dir); lf != nil {
		t.Fatalf("lockfile still there: %+v", lf)
	}
}
