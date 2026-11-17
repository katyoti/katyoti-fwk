// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// THE LIFE OF A HUB NOBODY OWNS: a hub spawned detached —
// `ktw-agents --hub keep`, `katyoti universe start` — or found by whoever
// asks is ended here, with the members it spawned, and its lockfile removed
// (a hub ends with no deferred cleanup: it has no signal handler).

// Running reports the hub a universe's data folder names, when its process
// is alive: the lockfile, else nil.
func Running(dataDir string) *LockFile {
	lf, err := ReadLockFile(dataDir)
	if err != nil || lf == nil || lf.PID <= 0 || !hubAlive(lf.PID) {
		return nil
	}
	return lf
}

// stopWait is how long Stop waits for the hub's process to be gone.
const stopWait = 15 * time.Second

// Stop ends the hub a universe's data folder names — its process and the
// members it spawned — and removes its lockfile; it answers the pid it
// stopped, 0 when none was running (a stale lockfile is removed then too).
func Stop(ctx context.Context, dataDir string) (int, error) {
	lf, err := ReadLockFile(dataDir)
	if err != nil {
		return 0, err
	}
	if lf == nil {
		return 0, nil
	}
	if !hubAlive(lf.PID) {
		removeDeadLockFile(dataDir)
		return 0, nil
	}
	killTree(ctx, lf.PID)
	deadline := time.Now().Add(stopWait)
	for ProcessAlive(lf.PID) {
		if time.Now().After(deadline) {
			return lf.PID, fmt.Errorf("hubclient: the hub (pid %d) did not stop within %s", lf.PID, stopWait)
		}
		select {
		case <-ctx.Done():
			return lf.PID, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	// Ours to remove only while it still names the hub we stopped.
	if _, err := removeLockFileNaming(dataDir, lf); err != nil {
		return lf.PID, err
	}
	return lf.PID, nil
}

// killTree ends a process and the processes it started (a hub's members).
// Windows: the tree, forced (a hub spawned detached is in no job of ours).
// Elsewhere: SIGTERM to the hub's process group — a detached hub leads its
// own session, its members are in it — then to the hub itself; a hub
// spawned attached shares its spawner's group, so its children are signalled
// through pkill as well.
func killTree(ctx context.Context, pid int) {
	if runtime.GOOS == "windows" {
		kill := exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		publish.NoConsoleWindow(kill) // a GUI stopping a universe opens no window
		_ = kill.Run()
		return
	}
	_ = exec.CommandContext(ctx, "pkill", "-TERM", "-P", strconv.Itoa(pid)).Run()
	if p, err := os.FindProcess(pid); err == nil {
		_ = signalHub(p)
	}
}
