// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"log/slog"
	"time"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// WHAT GOES WRONG AROUND A RUN IS SHOWN — a run that goes on although
// something failed around it (its manifest, its journal, its saved state, its
// group) gets a warning: an event, a journal line, and its last warnings in
// Runs. What fails around the task service itself (its runs folder, its
// TaskGroups.yaml, a manifest it cannot read) is a service warning, kept and
// listed by Groups.

const maxRunWarnings = 5

// warn records a warning on a run that goes on.
func (s *Service) warn(runID, reason string) {
	s.reg.mutate(runID, func(e *runEntry) {
		e.info.Warnings = append(e.info.Warnings, reason)
		if n := len(e.info.Warnings); n > maxRunWarnings {
			e.info.Warnings = e.info.Warnings[n-maxRunWarnings:]
		}
	})
	slog.Warn("tasks: a run goes on with a warning", "run_id", runID, "warning", reason)
	s.tell(runID, engine.RunWarning, func(e *engine.RunLifecycleEvent) { e.Reason = reason })
}

// serviceWarn records a warning on the task service itself, once.
func (s *Service) serviceWarn(reason string) {
	s.warnMu.Lock()
	for _, w := range s.warnings {
		if w == reason {
			s.warnMu.Unlock()
			return
		}
	}
	s.warnings = append(s.warnings, reason)
	s.warnMu.Unlock()
	slog.Warn("tasks: " + reason)
	if s.channel != nil {
		s.channel.Bus().Publish(engine.TaskServiceWarningTopic, map[string]any{"reason": reason, "at": time.Now()})
	}
}

// useRuns installs the run store: a manifest it cannot read is a service
// warning.
func (s *Service) useRuns(st *runStore) {
	st.onBad = func(name string, err error) {
		s.serviceWarn(fmt.Sprintf("a run manifest cannot be read — set aside in runs/bad: %s: %v", name, err))
	}
	s.runs = st
}

// serviceWarnings are the task service's warnings.
func (s *Service) serviceWarnings() []string {
	s.warnMu.Lock()
	defer s.warnMu.Unlock()
	return append([]string(nil), s.warnings...)
}
