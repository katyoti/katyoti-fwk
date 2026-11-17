// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A RUN SAYS WHAT HAPPENS TO IT — every change of a run's state is published
// once, on RunLifecycleTopic, from the few places it happens: its group's slots
// (queued, started), its executor (waiting, woken, timed out, its checkpoint),
// launch (resumed, restarted, how it ended), Stop (stopping, killed) and the
// restore (interrupted).

// tell publishes a transition of a run, with what the registry knows of it
// (read after the transition: its status is the new one).
func (s *Service) tell(runID string, t engine.RunTransition, fill func(*engine.RunLifecycleEvent)) {
	if isEnd(t) { // told, its end completes the panels it drove (run_panels.go)
		defer s.endPanels(runID, t)
	}
	if s.channel == nil {
		return
	}
	evt := &engine.RunLifecycleEvent{RunID: runID, Transition: t, At: time.Now()}
	if info, _, ok := s.reg.snapshot(runID); ok {
		evt.ParentRunID, evt.TaskName, evt.Group, evt.Status = info.ParentRunID, info.TaskName, info.Group, string(info.Status)
		if s.runs != nil && isEnd(t) {
			s.runs.saveEnded(info) // Runs still lists it after a restart
			if err := s.runs.markEnded(runID, info.Status); err != nil {
				s.warn(runID, "its end could not be recorded — a restart may take it up again: "+err.Error())
			}
		}
		if isEnd(t) {
			defer s.keepAliveEnded(info, t) // a kept-alive task is started again
		}
	}
	if fill != nil {
		fill(evt)
	}
	s.channel.Bus().Publish(engine.RunLifecycleTopic(runID), evt)
	s.bridge.publish(s.appCtx, evt)
}

// lifecycleHook tells a run's waits: what it waits for, what woke it.
type lifecycleHook struct {
	s     *Service
	runID string
}

func (lifecycleHook) OnPreStep(context.Context, *engine.State, engine.Event) error { return nil }
func (h lifecycleHook) OnPostStep(context.Context, *engine.State, *engine.StepResult, error) {
	h.s.stepped(h.runID)
}

func (h lifecycleHook) OnSuspend(_ context.Context, _ string, w *engine.WaitSpec) {
	h.s.reg.mutate(h.runID, func(e *runEntry) { e.wait = w })
	h.s.tell(h.runID, engine.RunWaiting, func(e *engine.RunLifecycleEvent) {
		if w == nil {
			return
		}
		for _, c := range w.Conditions {
			e.Waiting = append(e.Waiting, conditionName(c))
		}
		if w.Timeout > 0 {
			d := time.Now().Add(w.Timeout)
			e.Deadline = &d
		}
	})
}

func (h lifecycleHook) OnResume(_ context.Context, _ string, ev engine.Event) {
	h.s.reg.mutate(h.runID, func(e *runEntry) { e.wait = nil })
	h.s.stepped(h.runID) // its wait is over: it moves again from now
	if ev.Type == "timeout" {
		h.s.tell(h.runID, engine.RunTimedOut, nil)
		return
	}
	h.s.tell(h.runID, engine.RunWoken, func(e *engine.RunLifecycleEvent) {
		e.WokenBy = ev.Label
		if e.WokenBy == "" {
			e.WokenBy = ev.Type
		}
	})
}

// conditionName is what a wait condition waits for, for people: its label,
// else its external event, else its topic.
func conditionName(c engine.WaitCondition) string {
	switch {
	case c.Label != "":
		return c.Label
	case c.EventURI != "":
		return c.EventURI
	}
	return c.Topic
}

// tellingStore tells a run's checkpoint that could not be saved: the run goes
// on, but a restart would not find it where it is.
type tellingStore struct {
	engine.StateStore
	s *Service
}

// Delete warns of a saved state it could not remove: a restart could resume a
// run that already ended.
func (t tellingStore) Delete(runID string) error {
	err := t.StateStore.Delete(runID)
	if err != nil {
		t.s.warn(runID, "its saved state could not be removed — a restart could take it for unfinished: "+err.Error())
	}
	return err
}

func (t tellingStore) Save(runID string, cp engine.Checkpoint) error {
	err := t.StateStore.Save(runID, cp)
	if err != nil {
		t.s.tell(runID, engine.RunCheckpointFailed, func(e *engine.RunLifecycleEvent) { e.Reason = err.Error() })
	}
	return err
}

// ErrDeadlineExceeded: a run ran past its maximum duration (max_duration_s).
var ErrDeadlineExceeded = errors.New("its deadline passed")

// deadlineOf is when a run must have ended: its start plus its maximum
// duration; zero when it has none.
func deadlineOf(in taskapi.TaskRunRequest, startedAt time.Time) time.Time {
	if in.MaxDurationS <= 0 {
		return time.Time{}
	}
	return startedAt.Add(time.Duration(in.MaxDurationS) * time.Second)
}

// killedReason says why a run was killed, after killAfter.
func killedReason(killAfter time.Duration) string {
	return "killed: it did not stop within " + killAfter.String() + " of being stopped"
}

// endTransition is how a run that ended ended.
func endTransition(st taskapi.RunStatus) engine.RunTransition {
	switch st {
	case taskapi.RunStatusCompleted:
		return engine.RunCompleted
	case taskapi.RunStatusCancelled:
		return engine.RunCancelled
	case taskapi.RunStatusKilled:
		return engine.RunKilled
	}
	return engine.RunFailed
}
