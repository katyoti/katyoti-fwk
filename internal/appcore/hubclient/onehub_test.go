// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ONE HUB PER UNIVERSE: a live hub's lockfile is never removed
// by another hub, never overwritten by a second hub, and never replaced by a
// spawn because the hub was slow to answer.

// asHub makes this test process the live hub a lockfile names.
func asHub(t *testing.T) {
	t.Helper()
	prev := hubProcess
	self := os.Getpid()
	hubProcess = func(pid int) bool { return pid == self }
	t.Cleanup(func() { hubProcess = prev })
}

func TestAHubRemovesOnlyItsOwnLockFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	// Another hub hiding, ending or cut short leaves this one registered.
	if err := RemoveOwnLockFile(dir, os.Getpid()+1); err != nil {
		t.Fatal(err)
	}
	if lf, _ := ReadLockFile(dir); lf == nil || lf.PID != os.Getpid() {
		t.Fatalf("another hub's removal unregistered the live hub: lockfile %+v", lf)
	}
	if err := RemoveOwnLockFile(dir, os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if lf, _ := ReadLockFile(dir); lf != nil {
		t.Fatalf("the hub's own removal left its lockfile: %+v", lf)
	}
}

func TestASecondHubForAUniverseWithALiveHubIsRefused(t *testing.T) {
	asHub(t)
	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:4222"}); err != nil {
		t.Fatal(err)
	}
	err := ClaimLockFile(dir, LockFile{PID: os.Getpid() + 1, NATSURL: "nats://127.0.0.1:5555"})
	var running *HubRunningError
	if !errors.As(err, &running) || running.PID != os.Getpid() {
		t.Fatalf("a second hub claimed a live hub's universe: %v", err)
	}
	if !strings.Contains(err.Error(), "pid "+strconv.Itoa(os.Getpid())) {
		t.Errorf("the refusal does not name the running hub: %v", err)
	}
	if lf, _ := ReadLockFile(dir); lf == nil || lf.PID != os.Getpid() {
		t.Fatalf("the refused hub overwrote the lockfile: %+v", lf)
	}
	// The hub itself, re-showing, may write its own lockfile.
	if err := ClaimLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:4222"}); err != nil {
		t.Fatalf("the hub could not write its own lockfile again: %v", err)
	}
}

func TestALockFileOfAProcessThatIsNoHubIsReplaced(t *testing.T) {
	// The pid is alive but is no hub (the system gave it to another program):
	// the universe has no hub, a new one may claim it.
	prev := hubProcess
	hubProcess = func(int) bool { return false }
	t.Cleanup(func() { hubProcess = prev })
	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:4222"}); err != nil {
		t.Fatal(err)
	}
	if err := ClaimLockFile(dir, LockFile{PID: os.Getpid() + 1, NATSURL: "nats://127.0.0.1:5555"}); err != nil {
		t.Fatalf("a lockfile naming no hub process blocked a new hub: %v", err)
	}
}

// A hub that runs but does not answer is never doubled: find-or-spawn reports
// it, names it, and starts nothing.
func TestFindOrSpawnStartsNoSecondHubBesideALiveOneThatDoesNotAnswer(t *testing.T) {
	asHub(t)
	prevWait := slowHubWait
	slowHubWait = time.Second
	t.Cleanup(func() { slowHubWait = prevWait })
	var spawns int32
	prev := spawnHub
	spawnHub = func(context.Context, ConnectOptions) (*HubClient, error) {
		atomic.AddInt32(&spawns, 1)
		return nil, errors.New("spawned")
	}
	t.Cleanup(func() { spawnHub = prev })

	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := ConnectToHub(ctx, ConnectOptions{DataDir: dir, Mode: ModeFindOrSpawn})
	if !errors.Is(err, ErrHubNotAnswering) {
		t.Fatalf("err = %v, want ErrHubNotAnswering", err)
	}
	if n := atomic.LoadInt32(&spawns); n != 0 {
		t.Fatalf("%d hub(s) spawned beside the live one", n)
	}
	if !strings.Contains(err.Error(), "pid "+strconv.Itoa(os.Getpid())) {
		t.Errorf("the error does not name the hub: %v", err)
	}
	if lf, _ := ReadLockFile(dir); lf == nil {
		t.Fatal("the live hub's lockfile was removed")
	}
}

// The same lockfile naming a process that is no hub (a reused pid) is stale:
// a new hub is spawned.
func TestFindOrSpawnReplacesALockFileWhosePidIsNoHub(t *testing.T) {
	prevHub := hubProcess
	hubProcess = func(int) bool { return false }
	t.Cleanup(func() { hubProcess = prevHub })
	var spawns int32
	prev := spawnHub
	spawnHub = func(context.Context, ConnectOptions) (*HubClient, error) {
		atomic.AddInt32(&spawns, 1)
		return nil, errors.New("spawned")
	}
	t.Cleanup(func() { spawnHub = prev })

	dir := t.TempDir()
	if err := WriteLockFile(dir, LockFile{PID: os.Getpid(), NATSURL: "nats://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = ConnectToHub(ctx, ConnectOptions{DataDir: dir, Mode: ModeFindOrSpawn})
	if n := atomic.LoadInt32(&spawns); n != 1 {
		t.Fatalf("spawns = %d, want 1 (the pid is no hub)", n)
	}
}

// An announcement older than a find-or-spawn can last was left by a killed
// process: its pid, alive again under another program, keeps no hub up.
func TestAnOldWaitingAnnouncementKeepsNoHubUp(t *testing.T) {
	dir := t.TempDir()
	done := announceWaiting(dir)
	defer done()
	if n := Waiting(dir); n != 1 {
		t.Fatalf("Waiting = %d with one fresh announcement, want 1", n)
	}
	entries, err := os.ReadDir(waitingDir(dir))
	if err != nil || len(entries) != 1 {
		t.Fatalf("announcements: %v %v", entries, err)
	}
	old := time.Now().Add(-waitingStale - time.Minute)
	if err := os.Chtimes(filepath.Join(waitingDir(dir), entries[0].Name()), old, old); err != nil {
		t.Fatal(err)
	}
	if n := Waiting(dir); n != 0 {
		t.Fatalf("Waiting = %d with an announcement older than %s by a live pid, want 0", n, waitingStale)
	}
}

func TestDescribeProcessNamesTheProgramAndItsParent(t *testing.T) {
	got := DescribeProcess(os.Getpid())
	if !strings.HasPrefix(got, "pid "+strconv.Itoa(os.Getpid())+" (") || !strings.Contains(got, "started by pid ") {
		t.Fatalf("DescribeProcess(self) = %q, want its program and its parent", got)
	}
	if got := DescribeHolder("pid " + strconv.Itoa(os.Getpid())); !strings.Contains(got, "started by") {
		t.Errorf("DescribeHolder of a lock note = %q", got)
	}
	if got := DescribeHolder("another process"); got != "another process" {
		t.Errorf("DescribeHolder kept no plain note: %q", got)
	}
	for name, want := range map[string]bool{"ktw-hub.exe": true, "KTW-HUB.EXE": true, "ktw-hub": true, "ktw-agents.exe": false, "hubclient.test.exe": false} {
		if isHubProgram(name) != want {
			t.Errorf("isHubProgram(%q) = %v, want %v", name, !want, want)
		}
	}
}
