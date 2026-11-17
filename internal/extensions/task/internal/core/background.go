// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// defaultRetentionTTL bounds how long terminal entries linger in m.entries
// after completion. Long enough for in-flight readers (notifications drain,
// status polling) to observe the result; short enough to keep RSS bounded.
const defaultRetentionTTL = 5 * time.Minute

// defaultLRUCap is the hard count ceiling for each of the three run-state
// maps (bgManager.entries, Service.runs, LocalSpawner.children) AFTER TTL
// eviction has run. TTL bounds growth in time; LRU bounds growth in count
// for burst workloads where lots of runs complete inside the TTL window.
//
// 1000 is the tier-2 perf budget's reference scenario ceiling (see
// `decisions/tasks-performance-budget.md`). Operators can override per-map
// via the Set*LRUCap methods.
const defaultLRUCap = 1000

// bgEntry tracks a single background task.
type bgEntry struct {
	record        taskapi.TaskRecord
	cancel        context.CancelFunc
	done          chan struct{}
	result        *engine.RunResult
	err           error
	fireAndForget bool      // skip notification queue on completion/failure
	terminalAt    time.Time // zero until status becomes Done/Failed/Killed; used for TTL pruning
}

// bgManager implements BackgroundTaskManager.
type bgManager struct {
	mu            sync.RWMutex
	entries       map[string]*bgEntry
	notifications map[string][]taskapi.TaskNotification // parentRunID → pending notifications
	channel       *engine.Channel
	taskSvc       taskapi.TaskService // set after construction
	appCtx        context.Context     // application context for service resolution
	retentionTTL  time.Duration       // how long terminal entries linger before pruning
	lruCap        int                 // LRU count ceiling — see defaultLRUCap
}

// NewBackgroundTaskManager creates a background task manager. The ask-user
// suspension inbox is a separate concern (suspensionManager) — see
// TaskService.SuspensionManager().
func NewBackgroundTaskManager(channel *engine.Channel) taskapi.BackgroundTaskManager {
	return &bgManager{
		entries:       make(map[string]*bgEntry),
		notifications: make(map[string][]taskapi.TaskNotification),
		channel:       channel,
		retentionTTL:  defaultRetentionTTL,
		lruCap:        defaultLRUCap,
	}
}

// SetRetentionTTL overrides the terminal-entry retention window. Used by
// tests to drive fast pruning (e.g. 1ms) without waiting 5 minutes.
func (m *bgManager) SetRetentionTTL(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retentionTTL = d
}

// SetLRUCap overrides the LRU count ceiling. Tests use small caps (e.g.
// 3) to drive eviction without populating 1000+ entries.
func (m *bgManager) SetLRUCap(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lruCap = n
}

// QueueDepth returns the current entry count. Lock-protected snapshot,
// safe to call without holding m.mu. Used by the periodic metric emitter.
func (m *bgManager) QueueDepth() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// pruneStale removes terminal entries older than retentionTTL, then —
// if the map still exceeds lruCap — evicts the oldest-terminalAt entries
// until under cap. Caller must hold m.mu (write lock).
//
// Every eviction emits `slog.Info("run-state evicted", run_id=…, reason=…, map="bg")`
// so operators have a paper trail.
func (m *bgManager) pruneStaleLocked(now time.Time) {
	pruneAndCap(m.entries, m.retentionTTL, m.lruCap, now, "bg",
		func(e *bgEntry) time.Time {
			if e == nil {
				return time.Time{}
			}
			return e.terminalAt
		})
}

// pruneAndCap is the shared TTL+LRU eviction helper used by all three run-
// state maps (bg, runs, spawner). Generic over the entry type so each
// caller passes its own terminalAt accessor.
//
// Two phases:
//  1. TTL: drop any terminal entry older than ttl.
//  2. LRU: if still over cap, sort terminal entries by terminalAt and drop
//     the oldest until the map fits.
//
// Non-terminal entries (terminalAt zero) are never evicted — they're in-
// flight and the run-state map is their source of truth.
func pruneAndCap[E any](entries map[string]E, ttl time.Duration, cap int, now time.Time, mapName string, terminalAt func(E) time.Time) {
	if ttl > 0 {
		for id, e := range entries {
			t := terminalAt(e)
			if t.IsZero() {
				continue
			}
			if now.Sub(t) > ttl {
				slog.Info("run-state evicted", "run_id", id, "reason", "ttl", "map", mapName)
				delete(entries, id)
			}
		}
	}
	if cap <= 0 || len(entries) <= cap {
		return
	}
	// LRU: collect terminal entries with their timestamps, sort ascending,
	// drop the oldest until we fit. In-flight entries (terminalAt zero) are
	// never evicted — they're load-bearing.
	type aged struct {
		id string
		t  time.Time
	}
	terminal := make([]aged, 0, len(entries))
	for id, e := range entries {
		t := terminalAt(e)
		if t.IsZero() {
			continue
		}
		terminal = append(terminal, aged{id: id, t: t})
	}
	sort.Slice(terminal, func(i, j int) bool { return terminal[i].t.Before(terminal[j].t) })
	want := len(entries) - cap
	for i := 0; i < len(terminal) && want > 0; i++ {
		slog.Info("run-state evicted", "run_id", terminal[i].id, "reason", "lru", "map", mapName)
		delete(entries, terminal[i].id)
		want--
	}
}

func (m *bgManager) Register(parentRunID string, request taskapi.TaskRunRequest) (string, error) {
	// Background spawns must declare a wall-clock budget. Without one, a
	// run can hold a goroutine + bus subscriptions indefinitely; the audit
	// (security.md high-finding-2 / R6) flagged this as resource-exhaustion
	// risk. Foreground runs inherit the caller's context and are unaffected.
	if request.MaxDurationS <= 0 {
		return "", fmt.Errorf("background task %q: MaxDurationS must be > 0 (no unbounded background spawns)", request.TaskName)
	}

	taskID := engine.GenerateRunID()

	entry := &bgEntry{
		record: taskapi.TaskRecord{
			ID:          taskID,
			TaskName:    request.TaskName,
			Status:      taskapi.TaskPending,
			ParentRunID: parentRunID,
			StartedAt:   time.Now(),
		},
		done:          make(chan struct{}),
		fireAndForget: request.FireAndForget,
	}

	m.mu.Lock()
	m.pruneStaleLocked(time.Now())
	m.entries[taskID] = entry
	m.mu.Unlock()

	// Run lifecycle in background goroutine
	go m.runLifecycle(taskID, request)

	return taskID, nil
}

// killChildren kills the background tasks a parent run started that have not
// ended: its run ended without completing (failed, stopped, killed).
func (m *bgManager) killChildren(parentRunID string) {
	if parentRunID == "" {
		return
	}
	m.mu.RLock()
	var ids []string
	for id, e := range m.entries {
		if e != nil && e.record.ParentRunID == parentRunID && !isTerminalLifecycle(e.record.Status) {
			ids = append(ids, id)
		}
	}
	m.mu.RUnlock()
	for _, id := range ids {
		m.killTask(id)
	}
}

// waitChildren blocks until all background tasks whose ParentRunID matches
// the provided parent run have finished (entry.done closed), or until ctx is done.
//
// This is intentionally internal (not part of the BackgroundTaskManager interface)
// and is used to keep the process alive until a parent's in-process background
// children have had a chance to start and complete.
//
// The wait re-snapshots after each pass: a teammate spawned while we were
// already blocked on a sibling will be picked up on the next iteration.
func (m *bgManager) waitChildren(ctx context.Context, parentRunID string) {
	if parentRunID == "" {
		return
	}

	seen := make(map[string]struct{})
	for {
		// Snapshot child done channels under lock to avoid holding the mutex while waiting.
		m.mu.RLock()
		var pending []chan struct{}
		for id, e := range m.entries {
			if e == nil || e.done == nil {
				continue
			}
			if e.record.ParentRunID != parentRunID {
				continue
			}
			if _, alreadyWaited := seen[id]; alreadyWaited {
				continue
			}
			seen[id] = struct{}{}
			pending = append(pending, e.done)
		}
		m.mu.RUnlock()

		if len(pending) == 0 {
			return
		}

		for _, ch := range pending {
			select {
			case <-ch:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (m *bgManager) runLifecycle(taskID string, request taskapi.TaskRunRequest) {
	m.mu.RLock()
	entry, ok := m.entries[taskID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	defer close(entry.done)
	// A panicking machine/factory must not strand the record in TaskRunning
	// forever — pollers and the parent notification queue would never fire.
	// (Defer order: cancel → this recovery → close(done), so pollers waking
	// on done observe the Failed status.)
	defer func() {
		if p := recover(); p != nil {
			m.failTask(taskID, fmt.Sprintf("background task panicked: %v", p))
		}
	}()

	// Create cancellable context from application context.
	// If MaxDurationS > 0, wrap with a wall-clock timeout (Dimension 1
	// resource limit). Cancellation propagates to the agent loop via ctx.Done.
	ctx := m.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	var cancel context.CancelFunc
	if request.MaxDurationS > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(request.MaxDurationS)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	// Publish Running + the cancel func in ONE critical section. A Kill that
	// already marked the record terminal wins — the run must not start (the
	// old unlocked `entry.cancel = cancel` let a racing Kill mark the record
	// Killed while the agent kept executing with nobody holding its cancel).
	m.mu.Lock()
	if isTerminalLifecycle(entry.record.Status) {
		m.mu.Unlock()
		return
	}
	entry.record.Status = taskapi.TaskRunning
	entry.cancel = cancel
	m.mu.Unlock()

	taskSvc := m.taskSvc
	if taskSvc == nil {
		m.failTask(taskID, "TaskService not available")
		return
	}

	// Use TaskService.Run which already handles machine creation + executor
	request.ParentRunID = entry.record.ParentRunID
	request.BackgroundID = taskID
	runResp, err := taskSvc.Run(ctx, request)
	if err != nil {
		m.failTask(taskID, fmt.Sprintf("start task: %v", err))
		return
	}

	// Record the child's journal session id now that the run has started, so
	// task-status / await-child can hand it to the parent — the handle to read
	// THIS child's run with session-read (the delegate task id is not one).
	if runResp.SessionID != "" {
		m.mu.Lock()
		entry.record.SessionID = runResp.SessionID
		m.mu.Unlock()
	}

	// Wait for result
	result, err := taskSvc.Result(ctx, taskapi.ResultRequest{RunID: runResp.RunID})
	if err != nil {
		if ctx.Err() != nil {
			m.killTask(taskID)
			return
		}
		m.failTask(taskID, fmt.Sprintf("wait for result: %v", err))
		return
	}
	// The run ended otherwise than completed (failed, cancelled, killed, its
	// deadline passed): so did the task.
	if result.Status != string(taskapi.RunStatusCompleted) {
		why := result.Error
		if why == "" {
			why = "its run ended " + result.Status
		}
		m.failTask(taskID, why)
		return
	}

	// Complete
	now := time.Now()
	m.mu.Lock()
	if isTerminalLifecycle(entry.record.Status) {
		// A Kill landed between the result arriving and this commit — the
		// terminal verdict (and its notification) already went out.
		m.mu.Unlock()
		return
	}
	entry.record.Status = taskapi.TaskCompleted
	entry.record.EndedAt = &now
	entry.terminalAt = now
	entry.record.Result = &taskapi.AgentResult{
		Summary:       result.Status,
		TotalDuration: now.Sub(entry.record.StartedAt),
	}
	if output, ok := result.Output.(map[string]any); ok {
		if s, ok := output["summary"].(string); ok {
			entry.record.Result.Summary = s
		}
	}
	parentID := entry.record.ParentRunID
	m.mu.Unlock()

	// Enqueue notification for parent — unless fire-and-forget mode is set.
	if !request.FireAndForget {
		m.enqueueNotification(parentID, taskapi.TaskNotification{
			TaskID:   taskID,
			TaskName: request.TaskName,
			Status:   taskapi.TaskCompleted,
			Summary:  entry.record.Result.Summary,
		})
	}
	// Wake a parent blocked on this child's terminal event (await-child).
	m.emitChildTerminal(taskID, request.TaskName, parentID, "")
}

func (m *bgManager) Kill(taskID string) error {
	m.mu.RLock()
	_, ok := m.entries[taskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("background task %q not found", taskID)
	}
	// killTask captures entry.cancel under the write lock and invokes it —
	// reading it here after RUnlock raced the lifecycle goroutine's write.
	m.killTask(taskID)
	return nil
}

func (m *bgManager) Progress(taskID string) (*taskapi.AgentProgress, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[taskID]
	if !ok {
		return nil, fmt.Errorf("background task %q not found", taskID)
	}
	progress := entry.record.Progress
	return &progress, nil
}

func (m *bgManager) TaskStatus(taskID string) (*taskapi.TaskRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[taskID]
	if !ok {
		return nil, fmt.Errorf("background task %q not found", taskID)
	}
	record := entry.record
	return &record, nil
}

func (m *bgManager) Notifications(parentRunID string) []taskapi.TaskNotification {
	m.mu.Lock()
	defer m.mu.Unlock()
	notifs := m.notifications[parentRunID]
	delete(m.notifications, parentRunID)
	return notifs
}

// dropNotifications forgets what was queued for a parent that has ended.
func (m *bgManager) dropNotifications(parentRunID string) {
	m.mu.Lock()
	delete(m.notifications, parentRunID)
	m.mu.Unlock()
}

// isTerminalLifecycle reports whether a task lifecycle status is final.
// Terminal transitions happen exactly once — failTask / killTask / the
// completion commit all guard on it so a verdict (and its notification)
// can't be emitted twice for the same task.
func isTerminalLifecycle(s taskapi.TaskLifecycle) bool {
	return s == taskapi.TaskCompleted || s == taskapi.TaskFailed || s == taskapi.TaskKilled
}

// RecordEnded records a background task that ended in another process — a
// child interrupted by a restart: its parent, restored, finds it ended (its
// task-status, await-child, its notifications) instead of "not found".
func (m *bgManager) RecordEnded(taskID, parentRunID, taskName, errMsg string) {
	m.mu.Lock()
	if _, ok := m.entries[taskID]; !ok {
		m.entries[taskID] = &bgEntry{
			record: taskapi.TaskRecord{ID: taskID, ParentRunID: parentRunID, TaskName: taskName, Status: taskapi.TaskRunning, StartedAt: time.Now()},
			done:   make(chan struct{}),
		}
	}
	m.mu.Unlock()
	m.failTask(taskID, errMsg)
}

func (m *bgManager) failTask(taskID, errMsg string) {
	now := time.Now()
	m.mu.Lock()
	entry, ok := m.entries[taskID]
	if !ok || isTerminalLifecycle(entry.record.Status) {
		m.mu.Unlock()
		return
	}
	entry.record.Status = taskapi.TaskFailed
	entry.record.EndedAt = &now
	entry.terminalAt = now
	entry.record.Error = errMsg
	parentID := entry.record.ParentRunID
	taskName := entry.record.TaskName
	fireAndForget := entry.fireAndForget
	m.mu.Unlock()
	if !fireAndForget {
		m.enqueueNotification(parentID, taskapi.TaskNotification{
			TaskID:   taskID,
			TaskName: taskName,
			Status:   taskapi.TaskFailed,
			Error:    errMsg,
		})
	}
	m.emitChildTerminal(taskID, taskName, parentID, errMsg)
}

func (m *bgManager) killTask(taskID string) {
	now := time.Now()
	m.mu.Lock()
	entry, ok := m.entries[taskID]
	if !ok || isTerminalLifecycle(entry.record.Status) {
		m.mu.Unlock()
		return
	}
	// Capture the cancel func under the lock — the lifecycle goroutine
	// publishes it under the same lock, so this can't race the write.
	cancelFn := entry.cancel
	entry.record.Status = taskapi.TaskKilled
	entry.record.EndedAt = &now
	entry.terminalAt = now
	parentID := entry.record.ParentRunID
	taskName := entry.record.TaskName
	fireAndForget := entry.fireAndForget
	m.mu.Unlock()
	if cancelFn != nil {
		cancelFn()
	}
	if !fireAndForget {
		m.enqueueNotification(parentID, taskapi.TaskNotification{
			TaskID:   taskID,
			TaskName: taskName,
			Status:   taskapi.TaskKilled,
		})
	}
	// A killed child is terminal too — wake any parent awaiting it.
	m.emitChildTerminal(taskID, taskName, parentID, "killed")
}

func (m *bgManager) enqueueNotification(parentRunID string, notif taskapi.TaskNotification) {
	if parentRunID == "" {
		return
	}
	// A parent that has ended will never read it. (One not known yet — a
	// child recorded at a restore before its parent is restored — will.)
	if svc, ok := m.taskSvc.(*Service); ok && svc != nil && svc.endedHere(parentRunID) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications[parentRunID] = append(m.notifications[parentRunID], notif)
}

// emitChildTerminal publishes a child lifecycle event (`child.<taskID>.done` /
// `.failed`) on the shared Channel so an agent_as_human PARENT blocked on that
// topic (via the await-child tool) wakes immediately instead of busy-polling
// task-status. The polled notification queue still carries the result CONTENT
// the parent drains at turn start; this event is purely the WAKE signal.
//
// Keyed by taskID — the parent's handle from `delegate` — which is a DISTINCT
// keyspace from the Spawner's logical child-name topics for workflow children.
// Best-effort: a nil Channel (unit-test contexts) is a no-op. Must be called
// OUTSIDE m.mu (Publish may run subscribers synchronously).
func (m *bgManager) emitChildTerminal(taskID, taskName, parentRunID, failErr string) {
	if m.channel == nil {
		return
	}
	corr := engine.Correlation{RunID: taskID, ParentRunID: parentRunID}
	if parentRunID != "" {
		// A parent awaiting its children as a machine checks them when told.
		defer m.channel.Bus().Publish(engine.ChildEventsTopic(parentRunID), corr)
	}
	if failErr == "" {
		evt := &engine.ChildDoneEvent{ChildName: taskName, RunID: taskID}
		evt.Correlation = corr
		m.channel.Bus().Publish(engine.ChildDoneTopic(taskID), evt)
		return
	}
	evt := &engine.ChildFailedEvent{ChildName: taskName, RunID: taskID, Error: failErr}
	evt.Correlation = corr
	m.channel.Bus().Publish(engine.ChildFailedTopic(taskID), evt)
}

// Children lists the background tasks a parent run started.
func (m *bgManager) Children(parentRunID string) []taskapi.TaskRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []taskapi.TaskRecord
	for _, e := range m.entries {
		if e.record.ParentRunID == parentRunID {
			out = append(out, e.record)
		}
	}
	return out
}
