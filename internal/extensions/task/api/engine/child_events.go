// SPDX-License-Identifier: Apache-2.0

package engine

import "time"

// Child lifecycle events emitted by the Spawner on the shared Channel.
// All implement EventStamper for auto-stamping by the executor.

// ChildStartedEvent is emitted when a child machine starts running.
// ParentRunID identifies the parent workflow run that spawned this child;
// consumers (terminal-protocol translator, GUI Team tab) use it to draw the
// parent→child topology edge. Empty for the root run (no parent).
type ChildStartedEvent struct {
	HasCorrelation
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	Emitter     string    `json:"emitter"`
	ChildName   string    `json:"childName"`
	ChildKind   string    `json:"childKind"`
	RunID       string    `json:"runId"`
	ParentRunID string    `json:"parentRunId,omitempty"`
	// Mode is the child's delegation mode (worker / background / fork / …).
	// Consumers (GUI Team tab edges) render the REAL mode from here instead
	// of guessing a default.
	Mode string `json:"mode,omitempty"`
}

func (e *ChildStartedEvent) StampEvent(eventType string, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ChildDoneEvent is emitted when a child machine completes successfully.
// Output carries the child's final result (StepResult.Output).
type ChildDoneEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ChildName string    `json:"childName"`
	RunID     string    `json:"runId"`
	Output    any       `json:"output,omitempty"`
}

func (e *ChildDoneEvent) StampEvent(eventType string, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ChildSuspendedEvent is emitted when a child enters a user-input wait — e.g.
// an agent_as_human child that bubbled an ask-user question UP to its parent.
// It is the parent's wake signal: an agent_as_human parent blocks on the
// child's `child.<id>.suspended` topic (via await-child) and, on receipt,
// drains + answers the pending question instead of busy-polling task-status.
// QuestionID correlates to the bubbled ChildQuestion (AskBroker).
type ChildSuspendedEvent struct {
	HasCorrelation
	Type        string    `json:"type"`
	Timestamp   time.Time `json:"timestamp"`
	Emitter     string    `json:"emitter"`
	ChildName   string    `json:"childName"`
	RunID       string    `json:"runId"`
	ParentRunID string    `json:"parentRunId,omitempty"`
	QuestionID  string    `json:"questionId,omitempty"`
	// Question is the prompt text, carried so consoles/GUIs can render the
	// conversation line ("child asks: …") without reaching into the broker.
	Question string `json:"question,omitempty"`
}

func (e *ChildSuspendedEvent) StampEvent(eventType string, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}

// ChildFailedEvent is emitted when a child machine fails.
type ChildFailedEvent struct {
	HasCorrelation
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Emitter   string    `json:"emitter"`
	ChildName string    `json:"childName"`
	RunID     string    `json:"runId"`
	Error     string    `json:"error"`
}

func (e *ChildFailedEvent) StampEvent(eventType string, emitter string) {
	e.Type = eventType
	e.Emitter = emitter
	e.Timestamp = time.Now()
}
