// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"sync"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// EngineState tracks the agent loop's operational state.
// It implements agentapi.StateTracker and is used by slot handlers
// to expose state via KTSP (agent/status, agent/stop).
type EngineState struct {
	mu         sync.RWMutex
	state      ContainerState
	turn       int
	maxTurns   int
	cancelFunc context.CancelFunc
	lastResult *agentapi.RunResult
}

// NewEngineState creates an idle engine state.
func NewEngineState() *EngineState {
	return &EngineState{state: StateIdle}
}

// State returns the current state.
func (s *EngineState) State() ContainerState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Turn returns the current turn number.
func (s *EngineState) Turn() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.turn
}

// MaxTurns returns the max turns setting.
func (s *EngineState) MaxTurns() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxTurns
}

// LastResult returns the last task result.
func (s *EngineState) LastResult() *agentapi.RunResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastResult
}

// SetRunning marks the state as running with the given turn info.
// Implements agentapi.StateTracker.
func (s *EngineState) SetRunning(turn, maxTurns int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateRunning
	s.turn = turn
	s.maxTurns = maxTurns
}

// SetIdle marks the state as idle with the given result.
// Implements agentapi.StateTracker.
func (s *EngineState) SetIdle(result *agentapi.RunResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = StateIdle
	s.lastResult = result
	s.cancelFunc = nil
}

// SetCancelFunc stores a cancel function for the running agent loop.
func (s *EngineState) SetCancelFunc(cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancelFunc = cancel
}

// Cancel invokes the stored cancel function if the engine is running.
// Returns true if cancellation was triggered.
func (s *EngineState) Cancel() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateRunning {
		return false
	}
	if s.cancelFunc != nil {
		s.cancelFunc()
		return true
	}
	return false
}
