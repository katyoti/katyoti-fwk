// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TASKS KEPT RUNNING — the universe declares tasks it keeps running
// (TaskGroups.yaml keep_alive). The serving process starts each one that has no
// active run, after the restore decided the runs a previous process left, and
// again whenever it ends — after a backoff, doubling, so a task that keeps
// failing does not spin. Ending too often stops it, with a service warning; a
// person stopping its run stops it too. Running it by hand starts it again.
// One run at a time: any run of the task is its run, whoever started it. With a
// runs store, its state is kept in runs/keep-alive.json: a restart neither
// forgets a stop nor resets the count of a task that keeps ending.

var (
	keepAliveFirstBackoff = 30 * time.Second
	keepAliveMaxBackoff   = 30 * time.Minute
	keepAliveWindow       = time.Hour // ends counted within this window
	keepAliveMaxEnds      = 5         // ends within the window that stop it
)

type keepAliveState struct {
	ends     []time.Time // its runs' ends within the window
	backoff  time.Duration
	next     time.Time // not started again before
	stopped  string    // why it is stopped ("" = kept running)
	restarts int
	runID    string // its last run
}

type keepAlive struct {
	// first, max: its backoff — the package's, read once when it is made.
	first, max time.Duration
	mu         sync.Mutex
	tasks      []taskapi.KeepAliveTask
	state      map[string]*keepAliveState
	// serving is the serving process's context: set by the first sweep, so a
	// run that ends is started again on time (only a serving process does).
	serving context.Context
	timer   *time.Timer
	path    string // where its state is kept ("" = not kept)
	// sweeping: one sweep at a time — the periodic one and a backoff's timer
	// would otherwise both see no active run and start the task twice.
	sweeping sync.Mutex
}

// keepAliveSaved is a kept-alive task's state as kept on disk.
type keepAliveSaved struct {
	Ends     []time.Time   `json:"ends,omitempty"`
	Backoff  time.Duration `json:"backoff,omitempty"`
	Next     time.Time     `json:"next,omitempty"`
	Stopped  string        `json:"stopped,omitempty"`
	Restarts int           `json:"restarts,omitempty"`
	RunID    string        `json:"run_id,omitempty"`
}

// useKeepAlive declares the kept-alive tasks, their state read back from the
// runs store.
func (s *Service) useKeepAlive(tasks []taskapi.KeepAliveTask) {
	k := newKeepAlive(tasks)
	if s.runs != nil && len(k.tasks) > 0 {
		k.path = filepath.Join(s.runs.dir, "keep-alive.json")
		k.load(s)
	}
	s.keepAlive = k
}

// load reads the kept state back; a task no longer declared is forgotten.
func (k *keepAlive) load(s *Service) {
	b, err := atomicio.ReadAtomic(k.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	var saved map[string]keepAliveSaved
	if err == nil {
		err = json.Unmarshal(b, &saved)
	}
	if err != nil {
		s.serviceWarn("the kept-alive tasks' state cannot be read — they start afresh: " + err.Error())
		return
	}
	for task, v := range saved {
		if st, ok := k.state[task]; ok {
			*st = keepAliveState{ends: v.Ends, backoff: v.Backoff, next: v.Next, stopped: v.Stopped, restarts: v.Restarts, runID: v.RunID}
		}
	}
}

// save keeps the state. Called with k.mu held.
func (k *keepAlive) save() {
	if k.path == "" {
		return
	}
	out := map[string]keepAliveSaved{}
	for task, st := range k.state {
		out[task] = keepAliveSaved{Ends: st.ends, Backoff: st.backoff, Next: st.next, Stopped: st.stopped, Restarts: st.restarts, RunID: st.runID}
	}
	if err := writeJSONAtomic(k.path, out); err != nil {
		slog.Warn("tasks: the kept-alive tasks' state not kept", "err", err)
	}
}

func newKeepAlive(tasks []taskapi.KeepAliveTask) *keepAlive {
	k := &keepAlive{state: map[string]*keepAliveState{}, first: keepAliveFirstBackoff, max: keepAliveMaxBackoff}
	for _, t := range tasks {
		if t.Task == "" {
			continue
		}
		k.tasks = append(k.tasks, t)
		k.state[t.Task] = &keepAliveState{}
	}
	return k
}

// declared is the declaration of a kept-alive task (ok false: not one).
func (k *keepAlive) declared(task string) (taskapi.KeepAliveTask, bool) {
	for _, t := range k.tasks {
		if t.Task == task {
			return t, true
		}
	}
	return taskapi.KeepAliveTask{}, false
}

// keepAliveSweep starts each kept-alive task that is due and has no active
// run. Called by the serving process: after its restore sweep, and when a
// backoff ends.
func (s *Service) keepAliveSweep(ctx context.Context) {
	k := s.keepAlive
	if k == nil || len(k.tasks) == 0 {
		return
	}
	k.sweeping.Lock()
	defer k.sweeping.Unlock()
	k.mu.Lock()
	if k.serving == nil {
		k.serving = ctx
	}
	k.mu.Unlock()
	for _, t := range k.tasks {
		s.keepAliveStart(ctx, t)
	}
}

// keepAliveStart starts a kept-alive task when it is due and has no active
// run. A panic starting it is its failure, not the sweep's.
func (s *Service) keepAliveStart(ctx context.Context, t taskapi.KeepAliveTask) {
	k := s.keepAlive
	defer func() {
		if p := recover(); p != nil {
			slog.Error("tasks: starting a kept-alive task panicked", "task", t.Task, "panic", p, "stack", string(debug.Stack()))
			s.serviceWarn(fmt.Sprintf("the kept-alive task %s could not be started: it panicked: %v", t.Task, p))
		}
	}()
	if s.activeRunOf(t.Task) != "" {
		return
	}
	k.mu.Lock()
	st := k.state[t.Task]
	due := st.stopped == "" && !time.Now().Before(st.next)
	k.mu.Unlock()
	if !due {
		return
	}
	r, err := s.Run(ctx, taskapi.TaskRunRequest{
		TaskName: t.Task, Group: t.Group, Input: t.Input,
		Origin: &taskapi.RunOrigin{Kind: taskapi.OriginKeepAlive, ID: t.Task},
	})
	k.mu.Lock()
	switch {
	case errors.Is(err, taskapi.ErrGroupFull):
		// Its group is full for now: not a fault of the task — tried again
		// at the next sweep, its count of ends untouched.
		slog.Info("tasks: a kept-alive task waits for room in its group", "task", t.Task, "why", err)
	case err != nil:
		st.ends = append(st.ends, time.Now()) // could not start: counted as an end
		k.backOff(st, fmt.Sprintf("it could not be started: %v", err), s)
	default:
		st.runID = r.RunID
		slog.Info("tasks: a kept-alive task is started", "task", t.Task, "run_id", r.RunID)
	}
	k.save()
	k.mu.Unlock()
}

// activeRunOf is the id of an active run of task in this process, "" if none.
func (s *Service) activeRunOf(task string) string {
	for _, r := range s.reg.list("") {
		if r.TaskName != task || r.ParentRunID != "" {
			continue
		}
		switch r.Status {
		case taskapi.RunStatusQueued, taskapi.RunStatusRunning, taskapi.RunStatusWaiting, taskapi.RunStatusStopping, taskapi.RunStatusPending:
			return r.RunID
		}
	}
	return ""
}

// keepAliveEnded is told a run ended: a kept-alive task's run — whoever
// started it — is started again after its backoff, or stopped when it ended
// too often.
func (s *Service) keepAliveEnded(info taskapi.RunInfo, t engine.RunTransition) {
	k := s.keepAlive
	if k == nil || info.ParentRunID != "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	st, ok := k.state[info.TaskName]
	if !ok || st.stopped != "" {
		return
	}
	defer k.save()
	if t == engine.RunInterrupted {
		// The process went down: started again at once, no backoff. Still an
		// end — a task that keeps taking its process down is stopped too.
		if st.ends = append(st.ends, time.Now()); len(st.ends) >= keepAliveMaxEnds {
			k.backOff(st, fmt.Sprintf("its run %s was interrupted by a restart", info.RunID), s)
			return
		}
		st.next = time.Time{}
		k.wake(s, 0)
		return
	}
	// A run that lived long ended after real work: its backoff starts over.
	if !info.StartedAt.IsZero() && time.Since(info.StartedAt) > keepAliveWindow {
		st.backoff = 0
	}
	st.ends = append(st.ends, time.Now())
	k.backOff(st, fmt.Sprintf("its run %s ended %s", info.RunID, t), s)
}

// backOff schedules the next start, or stops the task when it ended too often.
// Called with k.mu held.
func (k *keepAlive) backOff(st *keepAliveState, why string, s *Service) {
	cut := time.Now().Add(-keepAliveWindow)
	kept := st.ends[:0]
	for _, e := range st.ends {
		if e.After(cut) {
			kept = append(kept, e)
		}
	}
	st.ends = kept
	task := ""
	for name, x := range k.state {
		if x == st {
			task = name
		}
	}
	if len(st.ends) >= keepAliveMaxEnds {
		st.stopped = fmt.Sprintf("it ended %d times within %s (last: %s) — run it by hand to start it again", len(st.ends), keepAliveWindow, why)
		go s.serviceWarn(fmt.Sprintf("the kept-alive task %s is stopped: %s", task, st.stopped))
		return
	}
	if st.backoff == 0 {
		st.backoff = k.first
	} else if st.backoff *= 2; st.backoff > k.max {
		st.backoff = k.max
	}
	st.next = time.Now().Add(st.backoff)
	st.restarts++
	slog.Info("tasks: a kept-alive task is started again after its backoff", "task", task, "why", why, "in", st.backoff)
	k.wake(s, st.backoff)
}

// wake sweeps again after d, when serving. Called with k.mu held.
func (k *keepAlive) wake(s *Service, d time.Duration) {
	ctx := k.serving
	if ctx == nil {
		return // not serving: the serving process's sweep starts it
	}
	if k.timer != nil {
		k.timer.Stop()
	}
	k.timer = time.AfterFunc(d, func() {
		if ctx.Err() == nil {
			s.keepAliveSweep(ctx)
		}
	})
}

// keepAliveStopped: a person stopped a kept-alive run — the task stays stopped
// until a person runs it again.
func (s *Service) keepAliveStopped(info taskapi.RunInfo) {
	k := s.keepAlive
	if k == nil || info.ParentRunID != "" {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if st, ok := k.state[info.TaskName]; ok {
		st.stopped = "stopped by a person — run it by hand to start it again"
		st.runID = info.RunID
		slog.Info("tasks: a kept-alive task is stopped by a person", "task", info.TaskName, "run_id", info.RunID)
		k.save()
	}
}

// keepAliveRunByHand: a person ran a kept-alive task — it is kept running
// again, its count of ends cleared.
func (s *Service) keepAliveRunByHand(task string) {
	k := s.keepAlive
	if k == nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if st, ok := k.state[task]; ok && st.stopped != "" {
		*st = keepAliveState{}
		k.save()
	}
}

// keepAliveInfo is the kept-alive tasks' state, for Groups.
func (s *Service) keepAliveInfo() []taskapi.KeepAliveInfo {
	k := s.keepAlive
	if k == nil || len(k.tasks) == 0 {
		return nil
	}
	var out []taskapi.KeepAliveInfo
	for _, t := range k.tasks {
		active := s.activeRunOf(t.Task)
		k.mu.Lock()
		st := k.state[t.Task]
		info := taskapi.KeepAliveInfo{Task: t.Task, Restarts: st.restarts, RunID: st.runID}
		switch {
		case st.stopped != "":
			info.State, info.Reason = "stopped", st.stopped
		case active != "":
			info.State, info.RunID = "running", active
		case !st.next.IsZero() && time.Now().Before(st.next):
			at := st.next
			info.State, info.NextAt = "backing-off", &at
		default:
			info.State = "waiting-to-serve"
		}
		k.mu.Unlock()
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Task < out[j].Task })
	return out
}
