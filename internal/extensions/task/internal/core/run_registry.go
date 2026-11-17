// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sync"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// runEntry tracks a single task run.
type runEntry struct {
	info       taskapi.RunInfo
	request    taskapi.TaskRunRequest // what it was started with (Repeat)
	cancel     context.CancelFunc
	pause      func() // cancels it for a clean stop: it stays on disk (shutdown.go)
	done       chan struct{}
	result     *engine.RunResult
	err        error
	terminalAt time.Time // zero until the run has ended (or was killed); used for TTL pruning
	// stopped: a person stopped it — its status (cancelled, killed) stays,
	// whatever the run's goroutine or its group's slots report after.
	stopped bool
	// killed is closed when the run is killed: its callers get their answer
	// although its goroutine has not ended (and may never).
	killed     chan struct{}
	killedOnce sync.Once
	// queuedAt: when it first queued for its slot; started: it got its first
	// slot (its lifecycle's queued / started).
	queuedAt time.Time
	started  bool
	// killedWhy: why it was killed, written when it was.
	killedWhy string
	// lastStepAt: when it last took a step (the stuck-run watchdog).
	lastStepAt time.Time
	// wait: what it waits for now (nil when it does not) — rechecked when the
	// bus reconnects (RecheckWaits).
	wait *engine.WaitSpec
}

// runRegistry owns the live + recently-terminal run entries for a
// TaskService: the keyed map, its mutex, and the retention bounds. The
// TTL/LRU enforcement itself is the shared pruneAndCap helper (also used
// by bgManager and LocalSpawner). The Service composes one of these so
// the lifecycle methods (Run/Status/Stop/Result/Runs) operate through a
// single concern instead of reaching into a bare map.
//
// Locking is encapsulated and mirrors the original Service sites
// exactly: add prunes-then-inserts under the write lock; get returns the
// entry pointer (callers reading immutable-after-create fields, or
// reading mutable fields after the entry's done channel has closed,
// rely on that as the original code did); mutate runs a closure under
// the write lock for the few in-place status writes.
type runRegistry struct {
	mu           sync.RWMutex
	runs         map[string]*runEntry
	retentionTTL time.Duration // bounds terminal-entry lifetime in `runs`
	lruCap       int           // LRU count ceiling — see defaultLRUCap
}

func newRunRegistry() *runRegistry {
	return &runRegistry{
		runs:         make(map[string]*runEntry),
		retentionTTL: defaultRetentionTTL,
		lruCap:       defaultLRUCap,
	}
}

// add prunes stale/over-cap entries then inserts e, keyed by its RunID.
func (r *runRegistry) add(e *runEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now())
	r.runs[e.info.RunID] = e
}

// get returns the entry pointer for runID. The caller reads
// immutable-after-create fields (cancel, done, StartedAt) directly, and
// mutable fields only after the done barrier or via mutate — same
// contract as the pre-decomposition Service.
func (r *runRegistry) get(runID string) (*runEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.runs[runID]
	return e, ok
}

// mutate runs fn against the entry under the write lock, if present.
// Used for the in-place status/result writes (Run's completion
// goroutine, Stop).
func (r *runRegistry) mutate(runID string, fn func(*runEntry)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.runs[runID]; ok {
		fn(e)
	}
}

// sessionIDOf returns the SessionID of runID, or "" if absent. Used by
// the parent-session inheritance lookup in Run.
func (r *runRegistry) sessionIDOf(runID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.runs[runID]; ok {
		return e.info.SessionID
	}
	return ""
}

// view is the run as callers see it: its info, when it ended, and — failed or
// killed — why.
func (e *runEntry) view() taskapi.RunInfo {
	info := e.info
	if !e.terminalAt.IsZero() {
		at := e.terminalAt
		info.EndedAt = &at
	}
	switch {
	case info.Status == taskapi.RunStatusKilled:
		info.Error = e.killedWhy
	case info.Status == taskapi.RunStatusFailed && e.err != nil:
		info.Error = e.err.Error()
	}
	return info
}

// ids is every run's id.
func (r *runRegistry) ids() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.runs))
	for id := range r.runs {
		out = append(out, id)
	}
	return out
}

// list returns a snapshot of every entry's RunInfo, optionally filtered
// to a session.
func (r *runRegistry) list(sessionID string) []taskapi.RunInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []taskapi.RunInfo
	for _, e := range r.runs {
		if sessionID != "" && e.info.SessionID != sessionID {
			continue
		}
		out = append(out, e.view())
	}
	return out
}

// snapshot is a copy of a run's info and machine state, read under the lock:
// the run's goroutine (and the group scheduler) write them while it runs —
// read them only through here or under mutate.
func (r *runRegistry) snapshot(runID string) (info taskapi.RunInfo, state string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.runs[runID]
	if !ok {
		return taskapi.RunInfo{}, "", false
	}
	info = e.view()
	if e.result != nil {
		state = e.result.State.Name
	}
	return info, state, true
}

// len returns the current entry count (the queue-depth gauge sample).
func (r *runRegistry) len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.runs)
}

func (r *runRegistry) setRetentionTTL(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.retentionTTL = d
}

func (r *runRegistry) setLRUCap(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lruCap = n
}

// pruneLocked removes terminal entries older than retentionTTL then
// enforces the LRU cap. Caller holds the write lock. Eviction logs to
// slog.Info with reason="ttl"|"lru" + map="runs".
func (r *runRegistry) pruneLocked(now time.Time) {
	pruneAndCap(r.runs, r.retentionTTL, r.lruCap, now, "runs",
		func(e *runEntry) time.Time {
			if e == nil {
				return time.Time{}
			}
			return e.terminalAt
		})
}
