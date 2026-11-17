// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// RunStatus represents the outcome of an agent run.
type RunStatus int

const (
	RunCompleted RunStatus = iota
	RunAborted
	RunMaxTurns
	RunError
	RunBudgetExceeded
)

func (s RunStatus) String() string {
	switch s {
	case RunCompleted:
		return "completed"
	case RunAborted:
		return "aborted"
	case RunMaxTurns:
		return "max_turns_exceeded"
	case RunError:
		return "error"
	case RunBudgetExceeded:
		return "budget_exceeded"
	default:
		return "unknown"
	}
}

// RunResult is the outcome of an agent run.
type RunResult struct {
	Status   RunStatus
	Summary  string
	Files    []string
	Turns    int
	Usage    Usage
	Duration time.Duration
	Error    error
}

// runResultJSON is a RunResult as it is written: its error as its message
// (a Go error value marshals to {}, which says nothing).
type runResultJSON struct {
	Status   RunStatus
	Summary  string
	Files    []string
	Turns    int
	Usage    Usage
	Duration time.Duration
	Error    string `json:",omitempty"`
}

// MarshalJSON writes the result with its error's message.
func (r RunResult) MarshalJSON() ([]byte, error) {
	j := runResultJSON{Status: r.Status, Summary: r.Summary, Files: r.Files, Turns: r.Turns, Usage: r.Usage, Duration: r.Duration}
	if r.Error != nil {
		j.Error = r.Error.Error()
	}
	return json.Marshal(j)
}

// UnmarshalJSON reads a result written by MarshalJSON back.
func (r *RunResult) UnmarshalJSON(b []byte) error {
	var j runResultJSON
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*r = RunResult{Status: j.Status, Summary: j.Summary, Files: j.Files, Turns: j.Turns, Usage: j.Usage, Duration: j.Duration}
	if j.Error != "" {
		r.Error = errors.New(j.Error)
	}
	return nil
}

// Event type constants are in agent_events.go as typed structs.

// ErrRunComplete is a sentinel error returned by tool handlers to signal
// that the agent should complete its run. Used by the complete tool.
type ErrRunComplete struct {
	Summary string
	Files   []string
	// Output is the task's structured output (the complete tool's full input
	// when the task declared an output-schema). When set, it becomes the run's
	// Output map so a programmatic consumer reading task.output gets the declared
	// envelope — not just the run Summary. nil for schema-less tasks.
	Output map[string]any
}

func (e *ErrRunComplete) Error() string {
	return fmt.Sprintf("run complete: %s", e.Summary)
}

// --- KTSP Engine Integration Types ---
// These interfaces are NOT used by the agent loop itself.
// They are consumed by ktsp/engine/ and tui/ for KTSP container integration.

// Observer receives events emitted by the agent loop via KTSP engine.
type Observer interface {
	OnEvent(ctx context.Context, eventType string, turn int, data any)
	Close() error
}

// ToolDispatcher routes tool calls via KTSP engine container.
type ToolDispatcher interface {
	Dispatch(ctx context.Context, name string, input json.RawMessage) (string, error)
	ToolDefs() []ToolDef
}

// StateTracker is notified of agent loop state changes via KTSP engine.
type StateTracker interface {
	SetRunning(turn, maxTurns int)
	SetIdle(result *RunResult)
}
