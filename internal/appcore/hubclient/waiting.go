// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// A CLIENT ON ITS WAY KEEPS THE HUB UP. A process that found no hub to reuse
// queues behind the spawn guard and connects once the hub it waited for is
// up; until it has, the hub cannot count it among its connections, and a hub
// whose first clients already left would end under it (EndWhenUnused) — the
// waiters then find no hub and fail. So a process announces itself while it
// finds or spawns (a file named by its pid under hub/waiting/), and an owned
// hub stays up while one is announced by a live process (Waiting).

func waitingDir(dataDir string) string { return filepath.Join(dataDir, "hub", "waiting") }

// announceWaiting announces this caller until the returned func is called.
// Best-effort: an announcement that cannot be written only loses the guard it
// adds.
func announceWaiting(dataDir string) (done func()) {
	dir := waitingDir(dataDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return func() {}
	}
	path := filepath.Join(dir, fmt.Sprintf("%d-%s", os.Getpid(), newNonce()))
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		return func() {}
	}
	return func() { _ = os.Remove(path) }
}

// waitingStale bounds how long an announcement is believed: a caller finds or
// spawns within spawnGuardWait plus a spawn's lockfile wait (90 s each). One
// older was left by a process killed on its way — and Windows soon gives its
// pid to another process, which an alive check would count for ever, keeping
// an owned hub up with no client.
var waitingStale = 4 * time.Minute

// Waiting counts the callers announced as finding or spawning this hub whose
// process is alive; an announcement left by a process that died, or older
// than waitingStale, is removed.
func Waiting(dataDir string) int {
	entries, err := os.ReadDir(waitingDir(dataDir))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		pidText, _, _ := strings.Cut(e.Name(), "-")
		info, ierr := e.Info()
		fresh := ierr == nil && time.Since(info.ModTime()) < waitingStale
		if pid, err := strconv.Atoi(pidText); err == nil && fresh && ProcessAlive(pid) {
			n++
			continue
		}
		_ = os.Remove(filepath.Join(waitingDir(dataDir), e.Name()))
	}
	return n
}
