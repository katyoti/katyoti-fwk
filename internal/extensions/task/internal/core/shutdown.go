// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A CLEAN STOP PAUSES, A QUIT STOPS — when its process closes the task service
// (the application closes it on its way out), what happens to the runs still
// going depends on the process:
//   - the serving process PAUSES them: each is cancelled, keeps its manifest
//     and checkpoint, and is taken up at the next start as after a crash — a
//     waiting run waits again, a working one restarts from its last step;
//   - any other process (a CLI or TUI run, a core started for a window) STOPS
//     them, as a person would: they end cancelled and are never restarted in
//     the background by the serving process.
// Either way new runs are refused from then on, and the runs have
// shutdownGrace to end.

// ErrShuttingDown: the task service is closing — no new run.
var ErrShuttingDown = errors.New("the task service is shutting down")

// ErrPausedByShutdown is the cause a paused run is cancelled with: its end path
// keeps it on disk for the next start.
var ErrPausedByShutdown = errors.New("paused by a clean shutdown — it is taken up at the next start")

// shutdownGrace is how long Close waits for the runs to end.
var shutdownGrace = 10 * time.Second

// serving marks the serving process's task service (ServeRestores).
type serving struct{ on atomic.Bool }

// Close ends the runs still going — paused in the serving process, stopped in
// any other — then gives up this process's runs (its owner lock). Closing
// twice is a no-op.
func (s *Service) Close() error {
	if !s.closing.CompareAndSwap(false, true) {
		return nil
	}
	s.endRunsForShutdown(shutdownGrace)
	if s.runs != nil {
		s.runs.close()
	}
	if s.metricStop != nil {
		close(s.metricStop)
	}
	return nil
}

// endRunsForShutdown pauses (serving) or stops (otherwise) every run still
// going, and waits up to grace for them to end.
func (s *Service) endRunsForShutdown(grace time.Duration) {
	pause := s.servingProc.on.Load()
	var waits []<-chan struct{}
	for _, id := range s.reg.ids() {
		e, ok := s.reg.get(id)
		if !ok {
			continue
		}
		select {
		case <-e.done:
			continue
		default:
		}
		waits = append(waits, e.done)
		if pause && e.pause != nil {
			e.pause()
			continue
		}
		_, _ = s.Stop(context.Background(), taskapi.StopRequest{RunID: id})
	}
	if len(waits) == 0 {
		return
	}
	how := map[bool]string{true: "paused", false: "stopped"}[pause]
	slog.Info("tasks: the task service closes", "runs", len(waits), "how", how)
	deadline := time.After(grace)
	for _, w := range waits {
		select {
		case <-w:
		case <-deadline:
			slog.Warn("tasks: runs did not end within the shutdown grace", "grace", grace, "how", how)
			return
		}
	}
}
