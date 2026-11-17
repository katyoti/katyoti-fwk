// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"sort"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ErrInterruptedByRestart is the error of a run a restart ended: it was
// running or queued, or it waited on something that no longer exists.
var ErrInterruptedByRestart = errors.New("interrupted by a restart")

// ErrTaskChangedSinceSaved is why a waiting run is not resumed: its task's
// definition is not the one its saved state belongs to.
var ErrTaskChangedSinceSaved = errors.New("its task changed since the run was saved")

// ErrNotRestartable is why a run working at a crash is not restarted: its
// kind's steps may not be done again (a script with side effects).
var ErrNotRestartable = errors.New("this kind of task is not restarted after a crash")

// ErrRestartedTooManyTimes is why a run that keeps crashing at the same step
// is not restarted again.
var ErrRestartedTooManyTimes = errors.New("it was restarted too many times without progress")

// maxRestartsWithoutProgress is how many times a run crashing at the same
// step is restarted before it is failed.
const maxRestartsWithoutProgress = 3

// ErrStateLayoutChanged is why a waiting run is not resumed: its machine's
// saved-state layout is not the one its checkpoint was saved under (a new
// build changed its shape).
var ErrStateLayoutChanged = errors.New("its saved state's layout changed since it was saved")

// RestoreReport says what RestoreRuns did with each run it found.
type RestoreReport struct {
	Resumed     []string // waiting runs, waiting again
	Restarted   []string // runs working or queued at the crash, started again
	Interrupted []string
	Cancelled   []string // runs a person had stopped before the restart
}

// RestoreRuns picks up the runs a previous process left in the DataDir. Called
// when the process starts serving its universe (and again while it serves):
//   - a top-level run that was WAITING (an event, an answer) is rebuilt from
//     its request and waits again where it waited — when its event comes, it
//     continues;
//   - a top-level run working or queued at the crash starts again — from its
//     last saved step — when its kind's steps may be done again;
//   - every other run — a child, a kind that is not restarted, one that
//     crashed too often — is reported interrupted by the restart, with why, so
//     a person, the planner and its parent see it ended. Children are decided
//     first: a parent resumed after them finds them ended.
//
// A run whose process is still alive (its owner's lease is live) is that
// process's, and left alone; RestoreRuns is called again while serving
// (restoreSweepEvery) to pick up what a process that died since left.
func (s *Service) RestoreRuns(ctx context.Context) RestoreReport {
	var rep RestoreReport
	if s.runs == nil {
		return rep
	}
	ms, err := s.runs.manifests()
	if err != nil {
		s.serviceWarn("the runs folder cannot be read — the runs a previous process left are neither restored nor reported: " + err.Error())
		return rep
	}
	one := func(m runManifest, top bool) {
		if s.runs.ownedByALiveProcess(m) {
			return
		}
		// One process restores it, however many sweep at once.
		release, ok := s.runs.claim(m)
		if !ok {
			return
		}
		s.restoreOne(ctx, m, &rep, top)
		release()
	}
	var workflowChildren []runManifest
	for _, m := range childrenFirst(ms) {
		if m.Spawn != nil && m.Spawn.By == "workflow" {
			workflowChildren = append(workflowChildren, m) // with their parent, below
			continue
		}
		one(m, m.Request.ParentRunID == "")
	}
	// A workflow's children are restored with it — after it, at any depth: a
	// child whose parent goes on (restored now, or live) is restored as a
	// run of its own; one whose parent is gone is reported interrupted.
	for progress := true; progress && len(workflowChildren) > 0; {
		progress = false
		rest := workflowChildren[:0]
		for _, m := range workflowChildren {
			if s.liveParent(m.Request.ParentRunID) {
				one(m, true)
				progress = true
			} else {
				rest = append(rest, m)
			}
		}
		workflowChildren = rest
	}
	for _, m := range workflowChildren {
		one(m, false)
	}
	s.runs.dropDeadOwners()
	if len(rep.Resumed)+len(rep.Restarted)+len(rep.Interrupted) > 0 {
		slog.Info("tasks: restored runs after a restart", "resumed", len(rep.Resumed), "restarted", len(rep.Restarted), "interrupted", len(rep.Interrupted))
	}
	return rep
}

// childrenFirst orders a sweep: children before the others — a parent resumed
// after its children finds them ended.
func childrenFirst(ms []runManifest) []runManifest {
	sort.SliceStable(ms, func(i, j int) bool {
		return ms[i].Request.ParentRunID != "" && ms[j].Request.ParentRunID == ""
	})
	return ms
}

// restoreOne restores a run this process claimed — a top-level run, or a
// workflow's child whose parent goes on (top): a waiting run waits again
// where it waited; a run working or queued at the crash starts again — from
// its last saved step — when its kind's steps may be done again. Any other
// child (an agent's: its parent, restored, is told its call was interrupted
// and decides) and any other run is reported interrupted.
func (s *Service) restoreOne(ctx context.Context, m runManifest, rep *RestoreReport, top bool) {
	// A run whose restore panics (its factory, its machine) is reported
	// interrupted with the panic — the sweep goes on with the others.
	defer func() {
		if p := recover(); p != nil {
			slog.Error("tasks: restore: restoring a run panicked", "run_id", m.RunID, "task", m.Request.TaskName, "panic", p, "stack", string(debug.Stack()))
			s.interrupt(ctx, m, fmt.Errorf("%w: restoring it panicked: %v", ErrInterruptedByRestart, p))
			rep.Interrupted = append(rep.Interrupted, m.RunID)
		}
	}()
	switch {
	case m.Ended != "": // it ended before the restart (its manifest outlived its end)
		s.forgetEnded(m)
		return
	case m.Stopped != "": // a person stopped it before the restart
		s.endLeft(ctx, m, taskapi.RunStatusCancelled, engine.RunCancelled, fmt.Errorf("%s before the restart", m.Stopped))
		rep.Cancelled = append(rep.Cancelled, m.RunID)
		return
	}
	why := ErrInterruptedByRestart
	if d := deadlineOf(m.Request, m.StartedAt); !d.IsZero() && time.Now().After(d) {
		why = fmt.Errorf("%w: %w (max_duration_s %d) while the process was down", ErrInterruptedByRestart, ErrDeadlineExceeded, m.Request.MaxDurationS)
	} else if top { // a top-level run, or a workflow's child restored with it
		cp, cpErr := s.runs.Load(m.RunID)
		var err error
		if cpErr != nil && !errors.Is(cpErr, os.ErrNotExist) {
			// Unreadable is not "none": restarted from its beginning, a run
			// would do again all it had done. It is reported, its file set aside.
			s.runs.setAside(m.RunID+".wait.json", cpErr)
			s.interrupt(ctx, m, fmt.Errorf("%w: its saved state cannot be read: %w", ErrInterruptedByRestart, cpErr))
			rep.Interrupted = append(rep.Interrupted, m.RunID)
			return
		}
		if cpErr == nil && cp.Wait != nil {
			if err = s.resume(ctx, m); err == nil {
				rep.Resumed = append(rep.Resumed, m.RunID)
				return
			}
		} else {
			var saved *engine.Checkpoint
			if cpErr == nil {
				saved = &cp
			}
			if err = s.restart(ctx, m, saved); err == nil {
				rep.Restarted = append(rep.Restarted, m.RunID)
				return
			}
		}
		slog.Warn("tasks: restore: a run could not be restored", "run_id", m.RunID, "task", m.Request.TaskName, "err", err)
		if err != nil { // its cause, whatever it is, says why
			why = fmt.Errorf("%w: %w", ErrInterruptedByRestart, err)
		}
	}
	s.interrupt(ctx, m, why)
	rep.Interrupted = append(rep.Interrupted, m.RunID)
}

// restart starts again a run that was working or queued when its process
// died: from its last saved step (cp), or from its beginning (no checkpoint:
// it had not finished a step).
func (s *Service) restart(ctx context.Context, m runManifest, cp *engine.Checkpoint) error {
	p, err := s.prepare(ctx, m.Request)
	if err != nil {
		return err
	}
	if m.TaskHash != "" && taskHash(p.t) != m.TaskHash {
		return ErrTaskChangedSinceSaved
	}
	if cp != nil && cp.Layout != "" && cp.Layout != engine.StateLayoutOf(p.machine) {
		return fmt.Errorf("%w (saved %s, now %s)", ErrStateLayoutChanged, cp.Layout, engine.StateLayoutOf(p.machine))
	}
	// A run still queued (at a crash or a clean stop) did nothing: taken up
	// from its beginning whatever its kind.
	if !engine.RestartsAfterCrash(p.machine) && (cp != nil || !m.Queued) {
		return ErrNotRestartable
	}
	version := 0
	if cp != nil {
		version = cp.State.Version
	}
	if m.Restarts > 0 && version == m.RestartVersion {
		m.Restarts++
	} else {
		m.Restarts, m.RestartVersion = 1, version
	}
	if m.Restarts > maxRestartsWithoutProgress {
		return fmt.Errorf("%w (%d times at step %d)", ErrRestartedTooManyTimes, m.Restarts-1, version)
	}
	m.Owner, m.PID = s.runs.owner, os.Getpid()
	if err := s.runs.saveManifest(m); err != nil {
		return err
	}
	how := launchRestartFresh
	if cp != nil {
		how = launchRestartWorking
	}
	_, err = s.launch(p, m.RunID, how, m.StartedAt)
	return err
}

// restoreSweepEvery is how often a serving process looks again for runs a
// process that died since left (a CLI run killed while the sidecar serves; a
// holder judged live at start whose heartbeat then stopped).
var restoreSweepEvery = 30 * time.Second

// ServeRestores restores at once what a previous process left, then again
// every restoreSweepEvery until ctx ends. Called once, by the serving process.
func (s *Service) ServeRestores(ctx context.Context, firstSwept func()) {
	s.servingProc.on.Store(true) // a clean stop pauses its runs (shutdown.go)
	s.RestoreRuns(ctx)
	s.sweepPanels(ctx)    // what no run drives any more ends
	s.keepAliveSweep(ctx) // what the universe keeps running, once the left runs are decided
	if firstSwept != nil {
		firstSwept()
	}
	t := time.NewTicker(restoreSweepEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.RestoreRuns(ctx)
			s.sweepPanels(ctx)
			s.keepAliveSweep(ctx)
		}
	}
}

// resume rebuilds a waiting run under its own id and lets it wait again.
func (s *Service) resume(ctx context.Context, m runManifest) error {
	p, err := s.prepare(ctx, m.Request)
	if err != nil {
		return err
	}
	if m.TaskHash != "" && taskHash(p.t) != m.TaskHash {
		return ErrTaskChangedSinceSaved
	}
	if cp, err := s.runs.Load(m.RunID); err == nil && cp.Layout != "" && cp.Layout != engine.StateLayoutOf(p.machine) {
		return fmt.Errorf("%w (saved %s, now %s)", ErrStateLayoutChanged, cp.Layout, engine.StateLayoutOf(p.machine))
	}
	m.Owner, m.PID = s.runs.owner, os.Getpid()
	if err := s.runs.saveManifest(m); err != nil {
		return err
	}
	_, err = s.launch(p, m.RunID, launchResumeWaiting, m.StartedAt)
	return err
}

// forgetEnded forgets on disk a run that ended before the restart; Runs keeps
// listing it (its end is recorded if the crash came before it was).
func (s *Service) forgetEnded(m runManifest) {
	known := false
	for _, info := range s.runs.endedRuns() {
		if info.RunID == m.RunID {
			known = true
		}
	}
	if !known {
		s.runs.saveEnded(taskapi.RunInfo{
			RunID: m.RunID, TaskName: m.Request.TaskName, Status: m.Ended, ParentRunID: m.Request.ParentRunID,
			SessionID: m.Request.SessionID, StartedAt: m.StartedAt, Group: m.Group, Priority: m.Request.Priority,
			Origin: originOf(m.Request), Target: m.Request.Target,
		})
	}
	s.runs.forget(m.RunID)
}

// interrupt records a run the restart ended (why), and forgets it on disk.
func (s *Service) interrupt(ctx context.Context, m runManifest, why error) {
	s.endLeft(ctx, m, taskapi.RunStatusFailed, engine.RunInterrupted, why)
}

// endLeft records how a run a previous process left ended — status, told as
// transition t, why — and forgets it on disk.
func (s *Service) endLeft(ctx context.Context, m runManifest, status taskapi.RunStatus, t engine.RunTransition, why error) {
	done := make(chan struct{})
	close(done)
	s.reg.add(&runEntry{
		info: taskapi.RunInfo{
			RunID:       m.RunID,
			TaskName:    m.Request.TaskName,
			Status:      status,
			ParentRunID: m.Request.ParentRunID,
			SessionID:   m.Request.SessionID,
			StartedAt:   m.StartedAt,
			Group:       m.Group,
			Priority:    m.Request.Priority,
			Origin:      originOf(m.Request),
			Target:      m.Request.Target,
		},
		cancel:     func() {},
		done:       done,
		err:        why,
		terminalAt: time.Now(),
	})
	defer s.runs.forget(m.RunID) // once its end is told (and recorded)
	res := savedResult{ParentRunID: m.Request.ParentRunID, EndedAt: time.Now(), Request: &m.Request}
	res.RunID, res.Status, res.Error = m.RunID, string(status), why.Error()
	s.runs.saveResult(res) // before its end is told: a parent woken by it reads it
	// Its session journal says how it ended, as any run's does.
	if s.channel != nil && m.Request.SessionID != "" {
		if rec, _ := s.openRecorder(ctx, m.Request.TaskName, m.Request.SessionID, m.RunID); rec != nil { // none: its interruption is still told
			defer rec.Close()
		}
	}
	// Told, its end completes what it left open on a screen.
	s.tell(m.RunID, t, func(e *engine.RunLifecycleEvent) { e.Reason = why.Error() })
	// A background child: its restored parent knows it by its background id.
	if bg, ok := s.bgMgr.(*bgManager); ok && m.Request.BackgroundID != "" {
		bg.RecordEnded(m.Request.BackgroundID, m.Request.ParentRunID, m.Request.TaskName, why.Error())
	}
	if s.channel != nil {
		evt := &engine.TaskOutputEvent{RunID: m.RunID, TaskName: m.Request.TaskName, Status: string(status), Error: why.Error()}
		evt.Correlation = engine.Correlation{SessionID: m.Request.SessionID, RunID: m.RunID, ParentRunID: m.Request.ParentRunID}
		evt.StampEvent(engine.EventTaskOutput, m.RunID)
		s.channel.Bus().Publish(engine.EventTaskOutput, evt)
		// A restored parent waiting on it learns it ended.
		s.channel.Bus().Publish(engine.RunEndedTopic(m.RunID), evt)
	}
}
