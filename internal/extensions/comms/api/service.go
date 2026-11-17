// SPDX-License-Identifier: Apache-2.0

// Package commsapi defines the user-communication (Dimension 5) channel as a
// task-type-agnostic SERVICE. It is the parent↔child conversation seam: a child
// Asks its parent and blocks; the parent Awaits the child's next event, Answers
// a pending question, or Stops a driven child.
//
// The agent reaches this surface through thin tool wrappers (ask-user /
// await-child / answer-child / stop-child); non-agent tasks (the simulator,
// workflows) call the same service directly. One implementation, one set of
// events on the engine Channel — so a simulator conversation is exercised and
// rendered exactly like an agent one, with zero LLM.
//
// Scope is Dimension 5 (user comms) only. Peer-to-peer teammate messaging
// (Dimension 4 — the MailboxManager) is a separate seam and is NOT part of this
// service.
package commsapi

import (
	"context"
	"errors"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ErrNoParentChannel surfaces when a task Asks but has no parent to route to
// (a root run) or the comms substrate is unreachable.
var ErrNoParentChannel = errors.New("comms: no parent channel (Ask requires a parent run + comms substrate)")

// Service is the parent↔child user-communication channel. Every method resolves
// the caller's identity from ctx (agent path) unless the request carries an
// explicit override (non-agent tasks pass identity from their RuntimeContext).
type Service interface {
	// Ask routes a child's question UP to its parent and BLOCKS until the
	// parent answers (or ctx is cancelled). Suspends the caller via the task
	// SuspensionManager and wakes a parent blocked in Await via the engine
	// Channel. The asking task is the "child".
	Ask(ctx context.Context, req AskRequest) (Answer, error)

	// Await BLOCKS until the calling parent's next child event — a child Asked,
	// completed, or failed — or the timeout elapses. Returns the event Kind.
	Await(ctx context.Context, req AwaitRequest) (Event, error)

	// Answer delivers an answer to a pending child question, resuming the
	// suspended child.
	Answer(ctx context.Context, req AnswerRequest) (Answered, error)

	// Stop hard-stops a driven child (one that never completes itself) so a
	// parent's Await returns.
	Stop(ctx context.Context, req StopRequest) error

	// Notify emits a one-way message/progress event on the conversation channel
	// (visible to the console + the parent's drain). No answer expected.
	Notify(ctx context.Context, req NotifyRequest) error

	// PendingQuestions drains the questions a parent's children have asked but
	// not yet been answered — for surfacing into the parent agent's
	// conversation (the OnTurnStart drain). Parent identity from ctx unless
	// parentRunID is non-empty.
	PendingQuestions(ctx context.Context, parentRunID string) []agentapi.ChildQuestion
}

// AskRequest is a child's question to its parent.
type AskRequest struct {
	Question string
	Options  []string

	// Identity overrides. Zero values are resolved from ctx (agent tool path):
	// CallerRunID ← RunIDFromContext, ParentRunID ← plugin-context
	// `_parent_run_id`, CallerName ← `_child_name`. Non-agent tasks (the
	// simulator) pass these explicitly from their RuntimeContext.
	CallerRunID string
	ParentRunID string
	CallerName  string
}

// Answer is the parent's reply delivered back to a blocked child.
type Answer struct {
	Text string
	Type string
}

// AwaitRequest parks a parent until its next child event.
type AwaitRequest struct {
	// ChildTaskID, when set, enables a terminal pre-check (the child may have
	// already completed/failed before Await was called).
	ChildTaskID string
	TimeoutS    int

	// ParentRunID override; zero → resolved from ctx (the caller IS the parent).
	ParentRunID string
}

// EventKind enumerates the result of Await.
const (
	EventAsked     = "asked"
	EventCompleted = "completed"
	EventFailed    = "failed"
	EventTimeout   = "timeout"
	EventCancelled = "cancelled"
)

// Event is the result of Await.
type Event struct {
	Kind string
}

// AnswerRequest delivers a parent's answer to a pending child question.
type AnswerRequest struct {
	QuestionID string
	Answer     string
}

// Answered reports the outcome of Answer for live display.
type Answered struct {
	// Delivered is false when the question was not found (already answered or
	// expired) — Detail carries the human-readable note in that case.
	Delivered bool
	Detail    string
}

// StopRequest hard-stops a driven child.
type StopRequest struct {
	ChildTaskID string
	Reason      string
}

// NotifyRequest is a one-way message/progress emission.
type NotifyRequest struct {
	Text string

	// Identity overrides; zero → resolved from ctx.
	CallerRunID string
	ParentRunID string
	CallerName  string
}

// THE PARENT'S HOLDING LIST: a child's or a teammate's question
// is a human task waiting on a holding list whose one member is
// its parent's run - the parent or the team leader that spawned it - an agent
// in a person's seat. The parent answers it through the human
// task's input (its to-do's report); the child's ask is that task's run, and
// its answer the task's result. The list is no CWM object: the comms
// extension answers its member to the scheduler (ParentListMembers).

// ParentListPrefix starts the scheduler group of a parent run's holding list.
const ParentListPrefix = "agent-run:"

// ParentList is the holding list of the questions asked to the run parentRunID.
func ParentList(parentRunID string) string { return ParentListPrefix + parentRunID }

// ParentOfList is the parent run a holding list is for, and whether it is one.
func ParentOfList(group string) (string, bool) {
	if len(group) <= len(ParentListPrefix) || group[:len(ParentListPrefix)] != ParentListPrefix {
		return "", false
	}
	return group[len(ParentListPrefix):], true
}

// ParentActor is the actor a parent run answers its children's questions as.
func ParentActor(parentRunID string) cwmapi.Actor {
	return cwmapi.Actor{ID: parentRunID, Kind: "agent"}
}
