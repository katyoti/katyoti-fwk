// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"os"
	"testing"
	"time"
)

// keepRegisteredWorld makes os.Getpid() the one live hub and every other pid
// dead, and polls fast.
func keepRegisteredWorld(t *testing.T, live ...int) {
	t.Helper()
	keepPoll, keepGrace := supplantedPoll, supplantedGrace
	supplantedPoll, supplantedGrace = 20*time.Millisecond, 100*time.Millisecond
	alive := map[int]bool{os.Getpid(): true}
	for _, pid := range live {
		alive[pid] = true
	}
	prevProbe, prevListed, prevHub := liveProbe, listedHub, hubProcess
	liveProbe = func(pid int) bool { return alive[pid] }
	listedHub = func(int) bool { return false }
	hubProcess = func(pid int) bool { return alive[pid] }
	t.Cleanup(func() {
		supplantedPoll, supplantedGrace = keepPoll, keepGrace
		liveProbe, listedHub, hubProcess = prevProbe, prevListed, prevHub
	})
}

// runKeepRegistered watches dir as the hub pid; it answers a stop function
// and a channel closed when the hub ends as supplanted.
func runKeepRegistered(t *testing.T, dir string, pid int) (stop func(), ended chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ended = make(chan struct{})
	done := make(chan struct{})
	rewrite := func() error {
		return ClaimLockFile(dir, LockFile{PID: pid, NATSURL: "nats://127.0.0.1:1"})
	}
	go func() {
		defer close(done)
		KeepRegistered(ctx, dir, pid, rewrite, func() { close(ended) })
	}()
	stop = func() { cancel(); <-done }
	t.Cleanup(stop)
	return stop, ended
}

func waitLockFileNames(dir string, pid int, within time.Duration) bool {
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if lf, err := ReadLockFile(dir); err == nil && lf != nil && lf.PID == pid {
			return true
		}
	}
	return false
}

// A RUNNING HUB WHOSE LOCKFILE IS GONE WRITES IT AGAIN: the
// person's universe was served by a hub no lockfile named, and every client
// answered « no hub running for this universe (no lockfile) ».
func TestARunningHubWhoseLockFileIsGoneWritesItAgain(t *testing.T) {
	keepRegisteredWorld(t)
	dir, self := t.TempDir(), os.Getpid()
	if err := WriteLockFile(dir, LockFile{PID: self, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	runKeepRegistered(t, dir, self)
	if err := RemoveLockFile(dir); err != nil {
		t.Fatal(err)
	}
	if !waitLockFileNames(dir, self, 2*time.Second) {
		t.Fatal("the hub's lockfile was removed under it and the hub ran on unregistered")
	}
}

// One left naming a hub that ended (a hub that overwrote it, then was
// killed by its spawner) is the running hub's again.
func TestARunningHubWhoseLockFileNamesADeadHubWritesItAgain(t *testing.T) {
	keepRegisteredWorld(t)
	dir, self := t.TempDir(), os.Getpid()
	runKeepRegistered(t, dir, self)
	if err := WriteLockFile(dir, LockFile{PID: self + 7919, NATSURL: "nats://127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	if !waitLockFileNames(dir, self, 2*time.Second) {
		t.Fatal("the lockfile named a dead hub and the running hub stayed unregistered")
	}
}

// While the spawn guard is held - the hub hiding its lockfile to end, or a
// client starting a hub - the hub does not write it back; once freed it does.
func TestARunningHubDoesNotWriteItsLockFileBackWhileTheGuardIsHeld(t *testing.T) {
	keepRegisteredWorld(t)
	dir, self := t.TempDir(), os.Getpid()
	if err := WriteLockFile(dir, LockFile{PID: self, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	hidden, ok := HideOwnLockFile(dir, self)
	if !ok {
		t.Fatal("the hub could not hide its lockfile")
	}
	runKeepRegistered(t, dir, self)
	if waitLockFileNames(dir, self, 300*time.Millisecond) {
		t.Fatal("a hub hiding its lockfile wrote it back on its own")
	}
	hidden.guard.release()
	if !waitLockFileNames(dir, self, 2*time.Second) {
		t.Fatal("the guard freed, the hub with no lockfile stayed unregistered")
	}
}

// A lockfile naming another LIVE hub is never overwritten: this hub ends.
func TestARunningHubSupplantedByALiveHubEndsAndLeavesItsLockFile(t *testing.T) {
	other := os.Getpid() + 104729
	keepRegisteredWorld(t, other)
	dir, self := t.TempDir(), os.Getpid()
	if err := WriteLockFile(dir, LockFile{PID: other, NATSURL: "nats://127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	_, ended := runKeepRegistered(t, dir, self)
	select {
	case <-ended:
	case <-time.After(3 * time.Second):
		t.Fatal("a hub another live hub's lockfile names ran on")
	}
	if lf, _ := ReadLockFile(dir); lf == nil || lf.PID != other {
		t.Fatalf("the live hub's lockfile was replaced: %+v", lf)
	}
}

// A hub only ever removes a lockfile naming its own pid: another hub's, or
// one it cannot read, is left.
func TestRemoveOwnLockFileRemovesOnlyItsOwn(t *testing.T) {
	dir, self := t.TempDir(), os.Getpid()
	if err := WriteLockFile(dir, LockFile{PID: self + 1, NATSURL: "nats://127.0.0.1:2"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOwnLockFile(dir, self); err != nil {
		t.Fatal(err)
	}
	if lf, _ := ReadLockFile(dir); lf == nil || lf.PID != self+1 {
		t.Fatalf("a hub ending removed another hub's lockfile: %+v", lf)
	}
	if err := os.WriteFile(LockFilePath(dir), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOwnLockFile(dir, self); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(LockFilePath(dir)); err != nil {
		t.Fatal("a hub ending removed a lockfile it could not read as its own")
	}
	if err := WriteLockFile(dir, LockFile{PID: self, NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOwnLockFile(dir, self); err != nil {
		t.Fatal(err)
	}
	if lf, _ := ReadLockFile(dir); lf != nil {
		t.Fatalf("a hub ending left its own lockfile: %+v", lf)
	}
}

// A removal judged on one lockfile never removes the one a hub wrote since
// (same pid reused or not: another start).
func TestALockFileIsRemovedOnlyWhileItIsTheOneJudged(t *testing.T) {
	dir := t.TempDir()
	judged := LockFile{PID: 4242, NATSURL: "nats://127.0.0.1:1", StartedAt: time.Now().Add(-time.Minute).UTC()}
	if err := WriteLockFile(dir, LockFile{PID: 4242, NATSURL: "nats://127.0.0.1:2", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if removed, err := removeLockFileNaming(dir, &judged); err != nil || removed {
		t.Fatalf("removed a lockfile written after the judgement: removed=%v err=%v", removed, err)
	}
	if lf, _ := ReadLockFile(dir); lf == nil {
		t.Fatal("the newer hub's lockfile is gone")
	}
}
