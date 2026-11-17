// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
	"time"
)

// BackgroundTaskManager manages async background agent tasks.
// Background tasks run in their own goroutine while the parent agent continues.
// The parent receives completion notifications at tool-round boundaries.
type BackgroundTaskManager interface {
	// Register creates a background task from a TaskRunRequest.
	// Returns the task ID immediately — the task runs asynchronously.
	Register(parentRunID string, request TaskRunRequest) (string, error)

	// Kill aborts a running background task.
	Kill(taskID string) error

	// Progress returns the current progress of a background task.
	Progress(taskID string) (*AgentProgress, error)

	// Status returns the current status of a background task.
	TaskStatus(taskID string) (*TaskRecord, error)

	// Notifications drains pending notifications for a parent run.
	// Returns and clears all queued notifications.
	Notifications(parentRunID string) []TaskNotification

	// Children lists the background tasks a parent run started, in no order.
	Children(parentRunID string) []TaskRecord
}

// SuspensionManager owns the ask-user suspension inbox: per-task wakeup
// channels so a tool (the inbox ask-user path, or any future async-wait
// tool) can block until an EventService handler delivers a Resume. This
// is a user-comms concern (dimension 5), separate from the background
// scheduler — resolve it via TaskService.SuspensionManager().
//
// v1 in-memory only: process restart drops every suspension. Cross-restart
// persistence + suspended-task item writeback are future follow-ups.
type SuspensionManager interface {
	// Suspend registers a task as waiting on the given URI. Returns
	// the receive-only resume channel — callers block on it until
	// Resume delivers a payload OR the context cancels. Repeated
	// Suspend calls for the same taskID return the SAME channel (so
	// concurrent callers within one task share the wakeup; in
	// practice the ask-user in-flight guard makes this one-caller
	// anyway).
	Suspend(taskID string, waitURI string) (<-chan json.RawMessage, error)

	// WhileWaiting runs wait — a run waiting inside a tool call for something
	// outside its machine, a person answering at a live UI — with the run's
	// group slot given back (status waiting) and taken again after it
	// (queueing). It returns ctx's error if the slot cannot be taken again.
	WhileWaiting(ctx context.Context, taskID string, wait func()) error

	// Resume unblocks the receiver of a previously-Suspended task by
	// delivering payload on its resume channel. Errors if taskID is
	// not currently suspended. Idempotent on the "already resumed"
	// case — second Resume for a delivered payload is a no-op.
	Resume(taskID string, payload json.RawMessage) error

	// Suspensions returns a snapshot of the currently-suspended
	// tasks. Used by the timeout sweeper + by GUI consumers that
	// want to render the inbox panel without polling each task
	// individually.
	Suspensions() []SuspendedRecord
}

// SuspendedRecord describes one in-flight suspension. The opaque
// payload remains pending until Resume is called.
type SuspendedRecord struct {
	TaskID    string    `json:"taskId"`
	WaitURI   string    `json:"waitUri"`
	StartedAt time.Time `json:"startedAt"`
}

// TaskRecord is the full state of a background task.
type TaskRecord struct {
	ID       string `json:"id"`
	TaskName string `json:"taskName"`
	// SessionID is the journal session of the child's run — set once the run
	// starts. It is the handle a caller passes to session-read to inspect THIS
	// child's run (the delegate task ID / `id` is NOT a session id). Empty until
	// the background run has started.
	SessionID   string        `json:"sessionId,omitempty"`
	Status      TaskLifecycle `json:"status"`
	ParentRunID string        `json:"parentRunId"`
	StartedAt   time.Time     `json:"startedAt"`
	EndedAt     *time.Time    `json:"endedAt,omitempty"`
	Progress    AgentProgress `json:"progress"`
	Result      *AgentResult  `json:"result,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// TaskLifecycle represents the lifecycle state of a background task.
type TaskLifecycle string

const (
	TaskPending   TaskLifecycle = "pending"
	TaskRunning   TaskLifecycle = "running"
	TaskCompleted TaskLifecycle = "completed"
	TaskFailed    TaskLifecycle = "failed"
	TaskKilled    TaskLifecycle = "killed"
)

// AgentProgress tracks live metrics for a running background agent.
type AgentProgress struct {
	ToolUseCount     int            `json:"toolUseCount"`
	TokenCount       int            `json:"tokenCount"`
	RecentActivities []ToolActivity `json:"recentActivities,omitempty"` // capped at 5
}

// ToolActivity describes a single tool invocation for progress tracking.
type ToolActivity struct {
	ToolName    string `json:"toolName"`
	Description string `json:"description,omitempty"`
}

// AgentResult is the terminal output of a completed background agent.
type AgentResult struct {
	Summary       string        `json:"summary"`
	Files         []string      `json:"files,omitempty"`
	TotalTokens   int           `json:"totalTokens"`
	TotalDuration time.Duration `json:"totalDuration"`
	Turns         int           `json:"turns"`
}

// TaskNotification is delivered to the parent agent when a background task completes.
type TaskNotification struct {
	TaskID   string        `json:"taskId"`
	TaskName string        `json:"taskName"`
	Status   TaskLifecycle `json:"status"`
	Summary  string        `json:"summary,omitempty"`
	Error    string        `json:"error,omitempty"`
}
