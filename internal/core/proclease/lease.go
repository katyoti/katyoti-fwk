// SPDX-License-Identifier: Apache-2.0

// Package proclease is a process lease: a file that says which live process
// holds something — a PID, a lease TTL and a heartbeat the holder refreshes.
// A holder is gone when its process is dead (seen at once) or when its
// heartbeat is older than twice its TTL (a hung process, or a PID the OS gave
// to another process since). CWM's workspace lock and the task run store's
// owners are leases.
package proclease

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/server"
)

// defaultLeaseTTL is the lease window. Heartbeats fire at
// TTL/3 so we tolerate ~2 missed heartbeats before another process
// can consider us wedged. 30s gives a comfortable margin for VM
// pause / GC stalls without making take-over recovery unreasonably
// slow.
const defaultLeaseTTL = 30 * time.Second

// ErrHeld is Acquire's answer when another live process holds the lease.
var ErrHeld = errors.New("proclease: held by another live process")

// HeldError is Acquire's refusal naming the holder: errors.Is(err, ErrHeld)
// holds, and the message says which process keeps the lease, so a person can
// find it instead of guessing.
type HeldError struct {
	PID int // the holder's process id
}

func (e *HeldError) Error() string {
	return fmt.Sprintf("%s: pid %d", ErrHeld, e.PID)
}

// Is makes a HeldError an ErrHeld.
func (e *HeldError) Is(target error) bool { return target == ErrHeld }

// heldBy is the refusal for the record rec: its holder named, ErrHeld when
// no record says who.
func heldBy(rec *lockRecord) error {
	if rec == nil || rec.PID <= 0 {
		return ErrHeld
	}
	return &HeldError{PID: rec.PID}
}

// ErrLost is Check's answer once another process took the lease over (it
// judged this holder dead or wedged) or the lease file is gone: this process
// holds nothing any more and must not write what the lease guards.
var ErrLost = errors.New("proclease: the lease was taken over by another process")

// lockRecord is the on-disk JSON shape of a lease file. Epoch says who holds
// it: every take-over writes one above the record it
// replaces, the holder's heartbeats keep it, and a holder whose epoch is no
// longer the file's lost the lease - even when the file names its own PID (a
// second lease instance of the same process, a PID the OS reused). A record
// written before the epoch reads as 0. The PID judges a holder alive.
type lockRecord struct {
	PID         int       `json:"pid"`
	Epoch       uint64    `json:"epoch"`
	StartedAt   time.Time `json:"started_at"`
	LeaseTTL    string    `json:"lease_ttl"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
}

// holds reports whether rec is the record this holder wrote: its PID and its
// epoch.
func (l *Lease) holds(rec *lockRecord) bool {
	return rec != nil && rec.PID == os.Getpid() && rec.Epoch == l.epoch.Load()
}

// Lease is a process lease on one file. The acquisition state machine
// handles six cases:
//
//  1. File missing                              → write ours.
//  2. File exists, holder PID == us             → take over (a restart, or
//     another instance of this process: the epoch fences the one before).
//  3. File exists, holder PID not alive         → stale → take over.
//  4. File exists, heartbeat older than 2× TTL  → wedged → take over.
//  5. File exists, holder alive + fresh         → ErrHeld.
//  6. After write, re-read confirms holder      → success.
//
// A background goroutine refreshes HeartbeatAt at LeaseTTL/3
// cadence; Release stops the goroutine and removes the file.
type Lease struct {
	file          string
	leaseTTL      time.Duration
	mu            sync.Mutex
	acquired      bool
	lost          atomic.Bool   // the heartbeat saw another holder, or no file
	epoch         atomic.Uint64 // the epoch this holder wrote at Acquire
	stopHeartbeat chan struct{}
	heartbeatDone chan struct{}
}

// New constructs a lease on file. The file itself is not created until
// Acquire is called.
func New(file string) *Lease {
	return &Lease{
		file:     file,
		leaseTTL: defaultLeaseTTL,
	}
}

// SetLeaseTTL overrides the default 30s lease window. Useful for
// tests that need fast turn-over. Must be called before Acquire.
func (l *Lease) SetLeaseTTL(d time.Duration) {
	if d > 0 {
		l.leaseTTL = d
	}
}

func (l *Lease) path() string { return l.file }

// Acquire attempts to obtain the lease. Returns nil on success, a *HeldError
// (errors.Is ErrHeld) naming the holder if another live process holds it freshly, or a wrapped error on I/O failure.
//
// Idempotent: a second Acquire by the same instance (without an
// intervening Release) is a no-op success.
func (l *Lease) Acquire(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.acquired {
		return nil
	}

	if dir := filepath.Dir(l.file); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("proclease: mkdir %s: %w", dir, err)
		}
	}

	action, prev, err := l.decideAcquisitionAction()
	switch {
	case err != nil:
		return err
	case action == acquisitionRefuse:
		return heldBy(prev)
	}
	var epoch uint64 = 1
	if prev != nil {
		epoch = prev.Epoch + 1 // a take-over: one above the record it replaces
	}

	rec := lockRecord{
		PID:         os.Getpid(),
		Epoch:       epoch,
		StartedAt:   time.Now().UTC(),
		LeaseTTL:    l.leaseTTL.String(),
		HeartbeatAt: time.Now().UTC(),
	}
	if err := l.writeRecord(rec); err != nil {
		return err
	}

	// Validate-after-write: confirm we still hold it. Defends against
	// a racing third process writing between our decision + write.
	got, err := l.readRecord()
	if err != nil {
		return fmt.Errorf("proclease: validate own lock: %w", err)
	}
	if got == nil || got.PID != os.Getpid() || got.Epoch != epoch {
		return heldBy(got)
	}

	l.epoch.Store(epoch)
	l.acquired = true
	l.lost.Store(false)
	l.stopHeartbeat = make(chan struct{})
	l.heartbeatDone = make(chan struct{})
	go l.heartbeatLoop()
	return nil
}

// Release stops the heartbeat goroutine and removes the lease file.
// Idempotent — Release on a never-Acquired lease is a no-op.
func (l *Lease) Release() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.acquired {
		return nil
	}
	close(l.stopHeartbeat)
	<-l.heartbeatDone

	// Only our own lease is removed: one another holder took over - another
	// process, or another instance of this one (a higher epoch) - is its.
	if rec, err := l.readRecord(); err == nil && l.holds(rec) {
		if err := os.Remove(l.path()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("proclease: remove lease file: %w", err)
		}
	}
	l.acquired = false
	l.stopHeartbeat = nil
	l.heartbeatDone = nil
	return nil
}

// acquisitionAction is the decision output for the state machine.
type acquisitionAction int

const (
	acquisitionWrite  acquisitionAction = iota // create / take over
	acquisitionRefuse                          // another holder is alive + fresh
)

// decideAcquisitionAction implements the 6-branch decision tree
// against the current on-disk state, and answers the record it read (nil
// for none) - the one a take-over replaces. It does NOT itself write or
// refuse — the caller acts on the returned action.
func (l *Lease) decideAcquisitionAction() (acquisitionAction, *lockRecord, error) {
	rec, err := l.readRecord()
	if err != nil {
		return acquisitionWrite, nil, err
	}
	if rec == nil {
		return acquisitionWrite, nil, nil // case 1: missing
	}
	if rec.PID == os.Getpid() {
		return acquisitionWrite, rec, nil // case 2: this process's - taken over
	}
	if !server.IsProcessAlive(rec.PID) {
		return acquisitionWrite, rec, nil // case 3: stale (holder dead)
	}
	ttl, parseErr := time.ParseDuration(rec.LeaseTTL)
	if parseErr != nil || ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	if time.Since(rec.HeartbeatAt) > 2*ttl {
		return acquisitionWrite, rec, nil // case 4: wedged
	}
	return acquisitionRefuse, rec, nil // case 5: alive + fresh
}

// readRecord loads the current .lock file. Returns (nil, nil) when
// the file does not exist (case 1 of the state machine).
func (l *Lease) readRecord() (*lockRecord, error) {
	data, err := atomicio.ReadAtomic(l.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("proclease: read lease file: %w", err)
	}
	var rec lockRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("proclease: parse lease file: %w", err)
	}
	return &rec, nil
}

// writeRecord serializes rec to .lock via the storage atomic helper.
func (l *Lease) writeRecord(rec lockRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("proclease: marshal lease file: %w", err)
	}
	if err := atomicio.WriteAtomic(l.path(), data, 0o644); err != nil {
		return fmt.Errorf("proclease: write lease file: %w", err)
	}
	return nil
}

// heartbeatLoop refreshes HeartbeatAt at leaseTTL/3 cadence until
// stopHeartbeat closes. Errors from the refresh are logged-silently
// (no logger plumbing yet) — recovery is the next heartbeat cycle.
func (l *Lease) heartbeatLoop() {
	defer close(l.heartbeatDone)
	interval := l.leaseTTL / 3
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopHeartbeat:
			return
		case <-ticker.C:
			rec, err := l.readRecord()
			if err != nil {
				continue // unreadable for now: the next beat reads it again
			}
			if !l.holds(rec) {
				// Another holder took the lease over (another process that
				// judged us dead or wedged, or another instance of this
				// one), or the file is gone: we hold nothing now.
				// Check says so to every writer; no heartbeat would win it
				// back.
				l.lost.Store(true)
				return
			}
			rec.HeartbeatAt = time.Now().UTC()
			_ = l.writeRecord(*rec)
		}
	}
}

// Check answers ErrLost when this holder no longer holds the lease it
// acquired - another process or another instance took it over (the file's
// epoch is not the one it wrote), or the file is gone - else nil. It
// reads the file, so a writer learns of a takeover at once, not a heartbeat
// later; a file it cannot read for now is not a loss. A lease never acquired
// answers nil: it guards nothing.
func (l *Lease) Check() error {
	l.mu.Lock()
	acquired := l.acquired
	l.mu.Unlock()
	if !acquired {
		return nil
	}
	if l.lost.Load() {
		return ErrLost
	}
	rec, err := l.readRecord()
	if err != nil {
		return nil
	}
	if !l.holds(rec) {
		l.lost.Store(true)
		return ErrLost
	}
	return nil
}

// IsHeldBy returns true if we (this instance) currently hold the lease.
func (l *Lease) IsHeldBy() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.acquired
}

// Live reports whether a live process holds the lease in file: its holder is
// alive and its heartbeat fresh (within twice its TTL). No file, or a dead or
// silent holder, is not live. A record naming this process's PID is judged by
// its heartbeat too: it may be the lease of a process that died and whose PID
// the OS gave to this one.
func Live(file string) (bool, error) {
	rec, err := (&Lease{file: file}).readRecord()
	if err != nil || rec == nil {
		return false, err
	}
	if !server.IsProcessAlive(rec.PID) {
		return false, nil
	}
	ttl, parseErr := time.ParseDuration(rec.LeaseTTL)
	if parseErr != nil || ttl <= 0 {
		ttl = defaultLeaseTTL
	}
	return time.Since(rec.HeartbeatAt) <= 2*ttl, nil
}

// RecentlyWritten reports whether file was written within twice the default
// lease TTL — what a holder that heartbeats does. A caller that cannot read a
// lease uses it to tell a lease being written (or held by a scanner) from one
// whose holder stopped long ago.
func RecentlyWritten(file string) bool {
	st, err := os.Stat(file)
	return err == nil && time.Since(st.ModTime()) <= 2*defaultLeaseTTL
}
