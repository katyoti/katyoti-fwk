// SPDX-License-Identifier: Apache-2.0

package hubclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// Only ONE caller may hold the spawn guard at a time.
//
// This is the invariant the hub-activation race violated: find-or-spawn was a
// read-then-act sequence with no mutual exclusion, so every caller that saw no
// live hub spawned one — and connectSpawn's first act is to REMOVE the lockfile,
// so the later spawners deleted the earlier hubs' lockfiles on the way past.
// Four ktw-hub processes on four NATS ports, and a GUI wedged against a dead one.
func TestSpawnGuard_OnlyOneHolder(t *testing.T) {
	dir := t.TempDir()

	const callers = 16
	var (
		mu     sync.Mutex
		held   int
		concur int // holders RIGHT NOW
		maxCon int // the most ever held at once
		wg     sync.WaitGroup
		start  = make(chan struct{})
	)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			g, got, err := acquireSpawnGuard(dir)
			if err != nil {
				t.Errorf("acquireSpawnGuard: %v", err)
				return
			}
			if !got {
				return
			}
			mu.Lock()
			held++
			concur++
			if concur > maxCon {
				maxCon = concur
			}
			mu.Unlock()
			// Hold it briefly so the others genuinely contend.
			time.Sleep(20 * time.Millisecond)
			mu.Lock()
			concur--
			mu.Unlock()
			g.release()
		}()
	}
	close(start)
	wg.Wait()

	// Some callers may acquire it AFTER an earlier holder released — that is
	// correct queueing, not a race. What must never happen is two at once, and
	// the guard file must not be left behind.
	if held == 0 {
		t.Fatal("nobody acquired the guard")
	}
	// THE invariant, asserted directly rather than inferred from a leftover
	// file. Two holders is the failure this guard exists to prevent — several
	// hubs on several ports — and counting them says so, where a stray file
	// only hints at it.
	if maxCon > 1 {
		t.Errorf("%d callers held the guard at once; exactly one may", maxCon)
	}
	if _, err := os.Stat(spawnGuardPath(dir)); !os.IsNotExist(err) {
		// A complete, correctly-owned guard used to survive here: os.Remove
		// fails on Windows while another caller has the file open, and the
		// error was discarded. Nobody would clear it until it went stale.
		t.Errorf("guard file left behind after release: %v (held=%d)", err, held)
	}
}

// A guard whose holder is gone must be breakable, or one killed process wedges
// every later activation forever — strictly worse than the race it prevents.
func TestSpawnGuard_StaleHolderIsBroken(t *testing.T) {
	dir := t.TempDir()

	g, got, err := acquireSpawnGuard(dir)
	if err != nil || !got {
		t.Fatalf("first acquire failed: got=%v err=%v", got, err)
	}
	// A second caller must be refused while the holder is alive (this process).
	if _, got2, _ := acquireSpawnGuard(dir); got2 {
		t.Fatal("guard granted twice while the holder is alive")
	}
	g.release()

	// Now forge a guard held by a PID that cannot be running, and an old
	// timestamp, and confirm it is broken rather than honoured.
	body, _ := json.Marshal(guardInfo{PID: 0x7FFFFFFE, StartedAt: time.Now().Add(-2 * guardStale)})
	if err := os.WriteFile(spawnGuardPath(dir), body, 0o644); err != nil {
		t.Fatalf("write forged guard: %v", err)
	}
	g2, got3, err := acquireSpawnGuard(dir)
	if err != nil || !got3 {
		t.Fatalf("stale guard was not broken: got=%v err=%v", got3, err)
	}
	g2.release()
}

// A truncated guard — a process killed mid-write — must not be honoured either,
// once it is old enough to be distinguishable from one being written right now.
//
// # Why the age matters
//
// An unparseable guard is ambiguous: the holder may have died mid-write, or it
// may have been created microseconds ago and not written yet — O_EXCL makes the
// file visible before its content lands. Breaking it on sight deleted LIVE
// guards, which let two callers hold at once.
//
// So the fixture backdates the file, which is also the real shape of the case:
// a process that died left its scrap behind, and whoever finds it arrives later.
func TestSpawnGuard_UnreadableIsBroken(t *testing.T) {
	dir := t.TempDir()
	path := spawnGuardPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("{par"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	old := time.Now().Add(-10 * guardWriteGrace)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	g, got, err := acquireSpawnGuard(dir)
	if err != nil || !got {
		t.Fatalf("unreadable guard was not broken: got=%v err=%v", got, err)
	}
	g.release()
}

// The other half: an unparseable guard that was created MOMENTS ago is a holder
// mid-write, and must be left alone. Without this the fix above is one edit
// away from being undone by someone reading only the test before it.
func TestSpawnGuard_UnwrittenGuardIsNotBroken(t *testing.T) {
	dir := t.TempDir()
	path := spawnGuardPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Exactly what O_EXCL leaves behind for an instant: an empty file.
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, got, err := acquireSpawnGuard(dir); got || err != nil {
		t.Fatalf("a guard created moments ago was broken: got=%v err=%v", got, err)
	}
}
