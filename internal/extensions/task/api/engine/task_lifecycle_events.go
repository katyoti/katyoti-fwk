// SPDX-License-Identifier: Apache-2.0

package engine

import (
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// EventTaskOutput is the bus topic for a top-level task run's terminal Output.
// Emitted by TaskService.Run when executor.Run returns with status Completed.
//
// The event lets post-hoc consumers (L2 test runner, ktw-utils session show,
// future GUI) inspect the run's final Output map without holding a live
// reference to TaskService. The simulator's `__spawn_dimensions` snapshot
// (added by Phase A `tier-1-simulator-introspection`) rides through here when
// `RecordContext: true` is set on the simulator config.
//
// Topic format: `task.output`. Per the subject conventions in
// `internal/extensions/task/api/engine/subjects.go`, this is the
// engine-level event name; production bus routing (NATS subjects with the
// tenant/universe/session prefix) is handled by the bridging layer.
const EventTaskOutput = "task.output"

// EventTaskStarted is the bus topic for a task run's START — the symmetric
// bookend to EventTaskOutput. Both are emitted by the EXECUTION FRAMEWORK
// (TaskService.Run / LocalSpawner.Spawn), NEVER by the caller. This is the
// key invariant: a task cannot start without firing this event, regardless of
// how it was launched (direct run, delegate, workflow child, teammate). The
// older caller-emitted `child_spawn` event is the parent's spawn INTENT;
// task.started is the task's own authoritative lifecycle signal.
//
// Topic format: `task.started`.
const EventTaskStarted = "task.started"

// TaskStartedEvent carries everything needed to identify a task run and the
// shape it started with: the full id set (run / session / parent, via the
// correlation envelope), the task identity (name + type), the delegation
// mode, the resolved spawn DIMENSIONS (the 6-dimension SpawnConfig, marshalled
// to a generic map so this package stays free of a presets import), and the
// validated INPUT map.
type TaskStartedEvent struct {
	agentapi.EventBase
	HasCorrelation
	RunID      string         `json:"run_id"`
	TaskName   string         `json:"task_name,omitempty"`
	TaskType   string         `json:"task_type,omitempty"`
	Mode       string         `json:"mode,omitempty"`
	Dimensions map[string]any `json:"dimensions,omitempty"`
	Input      map[string]any `json:"input,omitempty"`
}

// TaskOutputEvent carries the terminal result of a task run — the DONE
// bookend. Status + Output + Error capture the outcome; the correlation
// envelope carries the same id set as task.started so the two can be paired.
//
// Embeds agentapi.EventBase to satisfy the recorder's AgentEvent interface so
// the standard `Emitter == runID` filter applies and the existing journal
// pipeline can serialize it without special-casing.
type TaskOutputEvent struct {
	agentapi.EventBase
	HasCorrelation
	RunID      string         `json:"run_id"`
	TaskName   string         `json:"task_name,omitempty"`
	Status     string         `json:"status"` // "completed" | "failed" | "cancelled"
	Output     map[string]any `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"durationMs,omitempty"`
}

// init registers the task lifecycle events with the agent event registry so
// JSON deserialization (used by ktw-utils session show, L2 runner, the GUI
// task tree, etc.) can discriminate on the `type` field.
func init() {
	agentapi.EventRegistry.Register(EventTaskStarted, TaskStartedEvent{})
	agentapi.EventRegistry.Register(EventTaskOutput, TaskOutputEvent{})
}
