// SPDX-License-Identifier: Apache-2.0

package engine

import "time"

// RunTransition is a change of a run's state, as the task service reports it.
type RunTransition string

const (
	// RunQueued: the run waits for a slot of its group.
	RunQueued RunTransition = "queued"
	// RunStarted: the run got its slot and works (QueuedForMs: how long it queued).
	RunStarted RunTransition = "started"
	// RunWaiting: the run waits (Waiting: on what; Deadline: until when) and
	// holds no slot.
	RunWaiting RunTransition = "waiting"
	// RunWoken: what the run waited for came (WokenBy).
	RunWoken RunTransition = "woken"
	// RunTimedOut: the run's wait ended at its deadline.
	RunTimedOut RunTransition = "timed-out"
	// RunResumed: after a restart, the run waits again where it waited.
	RunResumed RunTransition = "resumed"
	// RunRestarted: after a crash, the run works again from its last step.
	RunRestarted RunTransition = "restarted"
	// RunInterrupted: after a restart, the run could not go on (Reason).
	RunInterrupted RunTransition = "interrupted"
	// RunPaused: a clean stop of the serving process paused the run; it stays
	// saved and is taken up at the next start (Reason).
	RunPaused RunTransition = "paused"
	// RunStopping: a person stopped the run; it has not ended yet.
	RunStopping RunTransition = "stopping"
	// RunKilled: the stopped run did not end in time and was abandoned.
	RunKilled RunTransition = "killed"
	// RunCancelled, RunCompleted, RunFailed: how the run ended (Reason for a
	// failure).
	RunCancelled RunTransition = "cancelled"
	RunCompleted RunTransition = "completed"
	RunFailed    RunTransition = "failed"
	// RunCheckpointFailed: the run's state could not be saved — it goes on, but
	// a restart would not find it where it is (Reason).
	RunCheckpointFailed RunTransition = "checkpoint-failed"
	// RunWarning: something went wrong around the run; it goes on (Reason).
	RunWarning RunTransition = "warning"
	// RunStuck: the run works but has taken no step for a long time.
	RunStuck RunTransition = "stuck"
)

// RunLifecycleEvent is one transition of a run, published on
// RunLifecycleTopic(runID) — by the task service only, from the few places a
// run's state changes.
type RunLifecycleEvent struct {
	RunID       string        `json:"runId"`
	ParentRunID string        `json:"parentRunId,omitempty"`
	TaskName    string        `json:"taskName,omitempty"`
	Group       string        `json:"group,omitempty"`
	Transition  RunTransition `json:"transition"`
	// Status is the run's status after the transition.
	Status string `json:"status,omitempty"`
	// Reason says why (a failure, an interruption, a warning).
	Reason string `json:"reason,omitempty"`
	// Waiting names what a waiting run waits for (its conditions' labels or
	// topics); Deadline is when its wait times out.
	Waiting  []string   `json:"waiting,omitempty"`
	Deadline *time.Time `json:"deadline,omitempty"`
	// WokenBy is the event that woke the run (its label, else its type).
	WokenBy string `json:"wokenBy,omitempty"`
	// QueuedForMs is how long a started run queued for its slot.
	QueuedForMs int64     `json:"queuedForMs,omitempty"`
	At          time.Time `json:"at"`
}

// TaskServiceWarningTopic is where the task service tells what goes wrong
// around it (its runs folder, its TaskGroups.yaml, a run manifest it cannot
// read): {"reason", "at"}.
const TaskServiceWarningTopic = "task-service.warning"

// RunLifecycleTopic is where a run's transitions are published.
func RunLifecycleTopic(runID string) string { return "run." + runID + ".lifecycle" }

// IsRunLifecycleTopic reports whether topic is a run's lifecycle topic.
func IsRunLifecycleTopic(topic string) bool {
	return len(topic) > len("run..lifecycle") && topic[:4] == "run." && topic[len(topic)-len(".lifecycle"):] == ".lifecycle"
}
