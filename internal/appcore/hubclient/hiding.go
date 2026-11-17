// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"context"
	"fmt"
	"log"
	"time"
)

// NO SECOND HUB IN THE HIDING WINDOW, AND NONE THAT STAYS. An
// owned hub with no client left hides its lockfile, counts its clients once
// more, and ends — or shows itself again for a client that arrived meanwhile
// (EndWhenUnused). A command arriving in that window found no lockfile and
// started a hub of its own; the first hub, unable to show itself again over
// the second one's lockfile, ran on unregistered with its late client — and
// its members held their providers' serving locks, so the registered hub's
// members could never serve: every command of the universe failed until a
// person ended the stray hub. Two rules close it:
//
//   - a hub hides only while it holds the spawn guard (HideOwnLockFile): a
//     command that finds no lockfile then queues behind the hub's decision,
//     and either finds it shown again or starts the next hub once it has
//     ended — never a second one beside it;
//   - a hub whose lockfile names another live hub ends (EndWhenSupplanted),
//     whatever made it unregistered: its members release their serving
//     locks, its clients re-dial the registered hub (they follow the
//     lockfile), and the universe has one hub again within a bound.

// Hidden is an owned hub's lockfile hidden under the spawn guard; Show puts
// it back and frees the guard. A hub that ends keeps the guard: it is broken
// once the hub's process is gone, so the next hub starts only after this
// one's members have let their serving locks go.
type Hidden struct {
	guard *spawnGuard
}

// HideOwnLockFile hides the lockfile of the hub pid under the spawn guard;
// false when the guard is held — a command is starting a hub or re-checking
// for one: the hub stays shown and tries again later.
func HideOwnLockFile(dataDir string, pid int) (*Hidden, bool) {
	guard, got, err := acquireSpawnGuard(dataDir)
	if err != nil || !got {
		return nil, false
	}
	if err := RemoveOwnLockFile(dataDir, pid); err != nil {
		guard.release()
		return nil, false
	}
	return &Hidden{guard: guard}, true
}

// Show writes the lockfile again (write) and frees the spawn guard.
func (h *Hidden) Show(write func() error) error {
	if h == nil {
		return write()
	}
	defer h.guard.release()
	return write()
}

// EndOwnedWhenUnused is EndWhenUnused for an owned hub pid, its lockfile
// hidden under the spawn guard: write shows it again, end ends the hub (the
// guard kept until its process is gone).
func EndOwnedWhenUnused(ctx context.Context, dataDir string, pid int, clients func() (open, ever int), write func() error, end func()) {
	var hidden *Hidden
	EndWhenUnused(ctx, clients,
		func() bool {
			h, ok := HideOwnLockFile(dataDir, pid)
			hidden = h
			return ok
		},
		func() {
			if err := hidden.Show(write); err != nil {
				log.Printf("hubclient: the hub could not show its lockfile again: %v", err)
			}
			hidden = nil
		},
		end)
}

var (
	// supplantedPoll is how often a hub checks its lockfile names it.
	supplantedPoll = time.Second
	// supplantedGrace is how long a hub runs on while another live hub's
	// lockfile names the universe.
	supplantedGrace = 3 * time.Second
)

// A RUNNING HUB KEEPS ITS LOCKFILE. A hub whose lockfile is
// gone, or names a hub that is no longer alive, runs on unseen: every client
// answers « no hub running for this universe (no lockfile) » while it holds
// the universe's members. Whatever removed or replaced the file (another
// hub's shutdown, a spawner's kill of a hub that had overwritten it, a
// client's stale judgement), the hub itself puts it back: every
// supplantedPoll it reads its lockfile and
//
//   - names itself: nothing to do;
//   - names another LIVE hub for supplantedGrace: it ends (that hub is the
//     universe's, this one's members let their serving locks go);
//   - is gone, or names a hub that is not alive: it writes it again (rewrite),
//     under the spawn guard - never while it hides its lockfile itself
//     (HideOwnLockFile holds the guard) nor while a client starts or looks
//     for a hub (it holds the guard; the next poll looks again).

// KeepRegistered watches the lockfile of the hub pid until ctx ends: it
// writes it again (rewrite) when it is gone or names a hub not alive, and
// calls end once it has named another live hub for supplantedGrace, then
// returns. A nil rewrite only watches for a supplanting hub.
func KeepRegistered(ctx context.Context, dataDir string, pid int, rewrite func() error, end func()) {
	var since time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(supplantedPoll):
		}
		lf, err := ReadLockFile(dataDir)
		if err != nil || (lf != nil && lf.PID == pid) {
			since = time.Time{} // unreadable for now, or its own
			continue
		}
		if lf != nil && hubAlive(lf.PID) && hubProcess(lf.PID) {
			if since.IsZero() {
				since = time.Now()
			}
			if time.Since(since) >= supplantedGrace {
				log.Printf("hubclient: %s is the universe's registered hub, not this one (pid %d): this hub ends - its clients follow the lockfile", DescribeProcess(lf.PID), pid)
				end()
				return
			}
			continue
		}
		since = time.Time{}
		if rewrite != nil {
			reRegister(ctx, dataDir, pid, lf, rewrite)
		}
	}
}

// EndWhenSupplanted is KeepRegistered with no rewrite: it calls end once the
// lockfile has named another live hub than pid for supplantedGrace.
func EndWhenSupplanted(ctx context.Context, dataDir string, pid int, end func()) {
	KeepRegistered(ctx, dataDir, pid, nil, end)
}

// reRegister writes the lockfile of the hub pid again, under the spawn
// guard, while - read again under it - it is still gone or names a hub not
// alive; seen is what the watch read. A guard held elsewhere (this hub
// hiding, a client starting a hub) leaves it to the next poll.
func reRegister(ctx context.Context, dataDir string, pid int, seen *LockFile, rewrite func() error) {
	guard, got, err := acquireSpawnGuard(dataDir)
	if err != nil || !got {
		return
	}
	defer guard.release()
	if ctx.Err() != nil {
		return // ending: its own cleanup removes the file
	}
	cur, err := ReadLockFile(dataDir)
	if err != nil || (cur != nil && (cur.PID == pid || hubAlive(cur.PID))) {
		return
	}
	why := "gone"
	if seen != nil {
		why = fmt.Sprintf("naming pid %d, not alive", seen.PID)
	}
	if err := rewrite(); err != nil {
		log.Printf("hubclient: this hub's lockfile (pid %d) was %s and could not be written again: %v", pid, why, err)
		return
	}
	log.Printf("hubclient: this hub's lockfile (pid %d) was %s: written again", pid, why)
}
