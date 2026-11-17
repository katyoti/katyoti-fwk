// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// Test hooks for the external test package core_test: a test that needs a
// package importing this one (the workflow's) runs there, on a durable
// service built here.

// DurableServiceForTest is a task service keeping its runs in dir (a second
// one on the same dir is the process after a restart); it runs "waiter".
func DurableServiceForTest(t *testing.T, dir string) *Service { return durableService(t, dir) }

// AddTaskForTest makes a task runnable by its name.
func (s *Service) AddTaskForTest(task *taskapi.Task) { s.tasks[task.Name] = task }

// OrphanForTest makes a run look left by a process that is gone.
func (s *Service) OrphanForTest(t *testing.T, runID string) { orphan(t, s.runs, runID) }

// WakeForTest ends a waiting "waiter" run.
func (s *Service) WakeForTest(runID string) {
	_ = s.channel.Bus().Publish("wake."+runID, map[string]any{})
}

// AwaitedForTest is what a run waits for now (nil when it does not wait).
func (s *Service) AwaitedForTest(runID string) []engine.WaitCondition {
	var out []engine.WaitCondition
	s.reg.mutate(runID, func(e *runEntry) {
		if e.wait != nil {
			out = append(out, e.wait.Conditions...)
		}
	})
	return out
}
