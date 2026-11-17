// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A RUN THAT NO LONGER MOVES IS REPORTED — a run working (not waiting) that
// has taken no step for stuckAfter is told stuck (an event, its journal, Runs),
// once per stall; its next step clears it. Nothing is killed: a person decides.

var (
	stuckAfter    = 10 * time.Minute
	watchdogEvery = time.Minute
)

// stepped notes a run's progress (its lifecycle hook, after every step).
func (s *Service) stepped(runID string) {
	s.reg.mutate(runID, func(e *runEntry) {
		e.lastStepAt = time.Now()
		e.info.StuckSince = nil
	})
}

// runWatchdog looks at the working runs every watchdogEvery.
func (s *Service) runWatchdog(ctx context.Context) {
	t := time.NewTicker(watchdogEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.metricStop:
			return
		case <-t.C:
			s.watchStuck()
		}
	}
}

// watchStuck tells the runs that have stalled since the last look.
func (s *Service) watchStuck() {
	now := time.Now()
	for _, id := range s.reg.ids() {
		var stalled time.Duration
		s.reg.mutate(id, func(e *runEntry) {
			if e.info.Status != taskapi.RunStatusRunning || !e.terminalAt.IsZero() || e.info.StuckSince != nil {
				return
			}
			since := e.lastStepAt
			if since.IsZero() {
				since = e.info.StartedAt
			}
			if d := now.Sub(since); d > stuckAfter {
				at := since
				e.info.StuckSince = &at
				stalled = d
			}
		})
		if stalled > 0 {
			s.tell(id, engine.RunStuck, func(e *engine.RunLifecycleEvent) {
				e.Reason = fmt.Sprintf("working, but no step for %s", stalled.Round(time.Second))
			})
		}
	}
}
