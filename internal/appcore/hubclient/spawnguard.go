// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// spawnGuard serialises find-or-spawn ACROSS PROCESSES for one DataDir.
//
// Without it, `connectFindOrSpawn` is a read-then-act race with a destructive
// first step: every caller that sees no live hub calls connectSpawn, which
// begins by REMOVING the lockfile and then execs `ktw-hub serve`. Two callers a
// moment apart therefore both spawn, and the second one deletes the first hub's
// lockfile on its way past — so the losing hubs stay alive holding NATS ports
// nobody can find, and any client that connected to one times out on every call
// for the rest of its life.
//
// Observed on 3 of 4 ktw-gui starts: four ktw-hub processes on four ports, a
// lockfile naming the last writer, and a GUI wedged against a dead one. It has
// to be cross-PROCESS because several binaries activate the same universe —
// ktw-gui, the katyoti CLI and ktw-agents all do.
//
// The guard is an O_EXCL file next to the lockfile. Whoever creates it spawns;
// everyone else waits and then re-checks for the hub the winner just started,
// which is the common case and the whole point.
type spawnGuard struct {
	path string
	// own is exactly what this holder wrote. release() deletes the file only
	// while it still holds these bytes, so a slow release cannot remove a guard
	// someone else has since taken — the same ownership rule acquire follows
	// when it breaks a stale one.
	own []byte
}

// guardStale bounds how long a guard file is believed. A process killed between
// creating the guard and writing the lockfile would otherwise wedge every later
// activation — strictly worse than the race it prevents. The budget must exceed
// a cold hub spawn, which re-probes every stale sidecar manifest before writing
// its lockfile.
const guardStale = 2 * time.Minute

type guardInfo struct {
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"started_at"`

	// Nonce identifies THIS holder, so a releaser can tell its own guard from
	// one taken after it.
	//
	// PID + StartedAt cannot: several callers in ONE process contend for the
	// same guard (ktw-gui activating a universe while the CLI does), so the PID
	// is shared, and time.Now() on Windows moves in ~1 ms steps — two guards
	// taken in the same step are byte-identical. Ownership then answers "yes"
	// for somebody else's guard, and the releaser deletes it.
	Nonce string `json:"nonce"`
}

// newNonce returns a random per-acquisition identity. On the vanishingly
// unlikely read failure it falls back to the clock, which is no worse than the
// identity this replaced.
func newNonce() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("t%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func spawnGuardPath(dataDir string) string {
	return filepath.Join(dataDir, "hub", "hub.spawning")
}

// acquireSpawnGuard takes the guard, or reports that someone else holds it.
func acquireSpawnGuard(dataDir string) (*spawnGuard, bool, error) {
	path := spawnGuardPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, err
	}
	// Bounded, because every branch below either returns or removes a file that
	// was there a moment ago — and an unbounded retry on a filesystem that keeps
	// handing back "exists" would spin forever rather than report.
	for attempt := 0; attempt < guardAcquireAttempts; attempt++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			body, _ := json.Marshal(guardInfo{PID: os.Getpid(), StartedAt: time.Now(), Nonce: newNonce()})
			_, _ = f.Write(body)
			_ = f.Close()
			return &spawnGuard{path: path, own: body}, true, nil
		}
		if !os.IsExist(err) {
			return nil, false, err
		}

		held, err := os.ReadFile(path)
		if err != nil {
			// VANISHED between the failed create and this read: the holder
			// released. Retry the create — do NOT remove anything. Removing
			// here is what broke the invariant this guard exists for: by the
			// time we looked, another caller could already have created its
			// own guard, and deleting THAT let two callers hold at once — the
			// very "several hubs on several ports" failure described above.
			continue
		}
		if !guardIsStale(held, guardAge(path)) {
			return nil, false, nil // someone holds it, legitimately
		}
		// Stale: the holder is gone or too old. Remove it, but only while it is
		// still the file we judged — a fresh holder may have replaced it in
		// between, and breaking a LIVE guard is the same bug as above.
		removeIfUnchanged(path, held)
	}
	// Attempts exhausted: every look found the guard taken or just-taken, which
	// means someone else keeps winning the race. That is "not acquired", not a
	// failure — the caller waits and re-checks for the hub the winner is
	// starting, which is the common path and the whole point of the guard.
	//
	// Reporting an error here instead made heavy contention look like a broken
	// guard: 16 callers on one directory, and the losers returned an error the
	// caller had no action for.
	return nil, false, nil
}

// removeGuard deletes a guard file, retrying a transient failure.
//
// os.Remove is not reliably one-shot on Windows: it fails while ANY other
// handle is open on the file, and every contending caller opens this one to
// read the claim. A discarded error there left a COMPLETE, correctly-owned
// guard on disk after its holder released — no second holder, just a file
// nobody would clear until it went stale, wedging activation for two minutes.
func removeGuard(path string) {
	deadline := time.Now().Add(guardRemoveBudget)
	for {
		if err := os.Remove(path); err == nil || os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// guardRemoveBudget bounds that retry. It only has to outlast a concurrent
// read of a small file.
const guardRemoveBudget = 2 * time.Second

// guardWriteGrace is how long a guard whose content has not landed yet is
// believed. It only has to cover an O_EXCL create followed by one small write.
const guardWriteGrace = 2 * time.Second

// guardAge reports how long ago the guard file was last written. A file that
// has vanished reports a huge age, so a caller racing a release still makes
// progress rather than treating it as freshly created.
func guardAge(path string) time.Duration {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Duration(1<<62 - 1)
	}
	return time.Since(fi.ModTime())
}

// guardAcquireAttempts bounds the retry loop. Each iteration follows a guard
// that was released or broken; exceeding the bound means other callers keep
// winning, which the loop reports as "not acquired" rather than spinning.
const guardAcquireAttempts = 16

// guardIsStale reports whether a guard's recorded claim can no longer be
// honoured — its owner is gone, its content is unreadable, or it has outlived
// the budget.
//
// Takes the BYTES rather than the path, so the caller decides what it read and
// can prove the file still holds them before breaking it. Reading inside here
// is what conflated "vanished" with "stale".
func guardIsStale(data []byte, age time.Duration) bool {
	var gi guardInfo
	if err := json.Unmarshal(data, &gi); err != nil {
		// Unparseable. That is EITHER a truncated write from a killed process
		// — genuinely stale — OR a guard created moments ago and not yet
		// written: O_EXCL makes the file visible before its content lands, so
		// a racing acquirer can read zero bytes from a perfectly live guard.
		//
		// Calling that stale deleted live guards, which is how two callers
		// ended up holding at once and how a guard outlived its holder.
		// Within the write grace we treat it as HELD and look again.
		return age > guardWriteGrace
	}
	if gi.PID > 0 && !ProcessAlive(gi.PID) {
		return true
	}
	return !gi.StartedAt.IsZero() && time.Since(gi.StartedAt) > guardStale
}

// removeIfUnchanged deletes the guard only while it still holds `expect`.
//
// Not atomic — nothing portable is — but it closes the window that matters: a
// holder that released and was replaced between our read and our remove wrote
// DIFFERENT bytes (a different PID, a later timestamp), so its guard survives.
func removeIfUnchanged(path string, expect []byte) {
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, expect) {
		return
	}
	removeGuard(path)
}

func (g *spawnGuard) release() {
	if g == nil {
		return
	}
	// Only OUR guard — releasing by path alone meant a holder whose release ran
	// late deleted whatever guard existed by then, including a live one.
	//
	// But "cannot read it" must not mean "leave it": while OUR file still
	// exists nobody else can have created one (the create is O_EXCL), so an
	// unreadable guard here is still ours and removing it by path is safe. The
	// first version returned on any read error, and a transient sharing
	// violation therefore leaked a guard that wedges every later activation
	// until it goes stale — two minutes of a hub that will not start.
	current, err := os.ReadFile(g.path)
	switch {
	case os.IsNotExist(err):
		return // already gone
	case err != nil:
		removeGuard(g.path) // exists but unreadable — it can only be ours
	case bytes.Equal(current, g.own):
		removeGuard(g.path)
	}
	// Anything else is a guard someone took after we released; leave it.
}

// waitForGuard blocks until the guard is free, so the caller can re-check for
// the hub the holder was starting. Returns false on timeout.
func waitForGuard(dataDir string, budget time.Duration) bool {
	path := spawnGuardPath(dataDir)
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		held, err := os.ReadFile(path)
		if err != nil {
			return true // released while we waited — that is what we waited for
		}
		if guardIsStale(held, guardAge(path)) {
			// Same care as acquire: break it only while it is still the file we
			// judged, so a fresh holder is not deleted out from under itself.
			removeIfUnchanged(path, held)
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func guardErr(dataDir string, err error) error {
	return fmt.Errorf("hubclient: spawn guard at %s: %w", spawnGuardPath(dataDir), err)
}
