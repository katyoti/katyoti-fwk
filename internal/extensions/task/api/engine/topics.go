// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"fmt"
	"strings"
)

// Topic Conventions — the IN-PROCESS address space.
//
// ── Two address spaces (the boundary) ─────────────────────────────────
//
// The runtime has TWO distinct event-naming systems. They are NOT a
// half-finished rename — they address two different transports:
//
//   topics.go  (this file) — the IN-PROCESS EventBus / engine.Channel.
//                FLAT names (`child.<name>.done`, `to.<name>.message`,
//                `audit.*`, `metric.*`, …). No tenant/universe/session
//                prefix because the process IS the scope: one Channel per
//                TaskService, so every topic is implicitly local. This is
//                the LIVE runtime vocabulary — the spawner, every workflow
//                node, the ledger, the orchestration mailbox, and the
//                terminal protocol all publish/subscribe here.
//
//   subjects.go — the CROSS-PROCESS NATS MessageBus. FULLY-SCOPED subjects
//                (`ktw.<tenant>.universe.<u>.session.<sid>.task.<tid>.…`).
//                The transport subjects (slot req/rep, registration,
//                descriptor, connstate, contributions) are live; the
//                session/task EVENT subjects are RESERVED targets for the
//                deferred in-process→NATS event bridge (each carries a
//                `// Replaces today's <topic>` mapping comment).
//
// When the bridge lands, an in-process topic here projects onto its NATS
// subject in subjects.go per that mapping. Until then, these two address
// spaces coexist by design. The full mapping + the EventURI relationship
// live in modules/bus/nats_subjects_list.md and the topic-conventions skill.
//
// Topics are how agents, workflows, and machines find each other within a
// single process at runtime.
//
// Namespace layout:
//
//   child.<name>.started        — child lifecycle: spawner → workflow
//   child.<name>.done           —
//   child.<name>.failed         —
//   child.<name>.suspended      —
//   child.<name>.progress       — optional periodic progress (opt-in)
//
//   to.<name>.cancel            — cooperative cancellation signal
//   to.<name>.message           — direct agent messaging
//   to.<name>.task              — task assignment (coordinated team pattern)
//   to.<name>.result            — result reply (coordinated team pattern)
//
//   group.<team>.message        — broadcast to team members
//   group.<team>.announcement   — team-wide announcement
//
//   workflow.<id>.started       — workflow lifecycle: external observers
//   workflow.<id>.done          —
//   workflow.<id>.failed        —
//
//   protocol.<id>.plan-approval — plan approval request/response (correlation ID)
//   protocol.<id>.permission    — tool permission request/response
//   protocol.<id>.shutdown      — shutdown handshake
//
// Three rules:
//   1. Emitters own their name. child.* events come from the Spawner, not the child.
//   2. Subscribers use the exact topic (no wildcards at subscription level).
//   3. Payloads are typed structs (see topic_events.go), not raw maps.

// ── Child lifecycle topics (spawner emits, workflow subscribes) ───────

// ChildStartedTopic returns the event topic for a child starting.
func ChildStartedTopic(name string) string { return fmt.Sprintf("child.%s.started", name) }

// ChildDoneTopic returns the event topic for a child completing successfully.
func ChildDoneTopic(name string) string { return fmt.Sprintf("child.%s.done", name) }

// ChildFailedTopic returns the event topic for a child failing.
func ChildFailedTopic(name string) string { return fmt.Sprintf("child.%s.failed", name) }

// PlanChangedTopic is published when an object's plan changes (planification):
// the planner wakes to tick.
func PlanChangedTopic() string { return "plan.changed" }

// PlannerTickTopic carries what one planner tick decided (its report), into
// the planner run's journal.
func PlannerTickTopic() string { return "planner.tick" }

// ExternalEventTopic is the in-process topic an external event (an EventURI of
// another process) is forwarded onto while a run waits on it
// (WaitCondition.EventURI).
func ExternalEventTopic(eventURI string) string { return "external." + eventURI }

// RunEndedTopic is where the TaskService publishes a run's end (its
// task.output event) for whoever waits on that run — a delegating parent
// waiting as a machine.
func RunEndedTopic(runID string) string { return "run." + runID + ".ended" }

// ChildEventsTopic is where a parent run is told one of its children asked it
// a question or ended — for a parent awaiting its children as a machine
// (await-child). No payload is read: the parent checks what happened.
func ChildEventsTopic(parentRunID string) string { return "parent." + parentRunID + ".child-event" }

// QuestionAnsweredTopic is where a child's question to its parent is told
// answered — for the child awaiting the answer as a machine.
func QuestionAnsweredTopic(questionID string) string { return "question." + questionID + ".answered" }

// HumanTaskAnsweredTopic is where a human task's to-do is told answered - for
// the human task's run awaiting it as a machine. No payload is read: the run
// reads the to-do.
func HumanTaskAnsweredTopic(todoID string) string { return "human-task." + todoID + ".answered" }

// HumanTaskDueTopic is where a human task's to-do is told reminded or
// escalated (what: "reminded" or "escalated") - for the channels that
// announce it (the phone's push).
func HumanTaskDueTopic(todoID, what string) string { return "human-task." + todoID + "." + what }

// IsChildTopic reports whether a topic is one of a child's lifecycle topics
// (`child.<name>.<action>`): an event only an in-process child emits.
func IsChildTopic(topic string) bool { return strings.HasPrefix(topic, "child.") }

// ParseChildTopic splits a child topic into the child's name and the action
// (child.<name>.<action>); ok false for any other topic.
func ParseChildTopic(topic string) (name, action string, ok bool) {
	rest, found := strings.CutPrefix(topic, "child.")
	if !found {
		return "", "", false
	}
	i := strings.LastIndex(rest, ".")
	if i <= 0 || i == len(rest)-1 {
		return "", "", false
	}
	return rest[:i], rest[i+1:], true
}

// ChildCancelTopic returns the event topic for sending a cancel signal to a child.
func ChildCancelTopic(name string) string { return fmt.Sprintf("to.%s.cancel", name) }

// ChildSuspendedTopic signals a child entered HIL/user-input wait.
func ChildSuspendedTopic(name string) string { return fmt.Sprintf("child.%s.suspended", name) }

// ChildProgressTopic delivers periodic progress reports (opt-in per child).
func ChildProgressTopic(name string) string { return fmt.Sprintf("child.%s.progress", name) }

// ── Call observability topics (workflow `kind: call` emits) ──────────

// CallCompletedTopic returns the per-attempt observability topic for a
// workflow `call` node. A call is NOT a child machine (A-hybrid model):
// this single event per attempt is its journal/session-view
// trail — uri, duration, attempt, outcome — correlated to the workflow's
// runID via the executor's event stamping.
func CallCompletedTopic(name string) string { return fmt.Sprintf("call.%s.completed", name) }

// ── Agent-to-agent direct topics ──────────────────────────────────────

// AgentMessageTopic is the direct messaging topic for a named agent.
func AgentMessageTopic(name string) string { return fmt.Sprintf("to.%s.message", name) }

// AgentTaskTopic is where task assignments are delivered (coordinator pattern).
func AgentTaskTopic(name string) string { return fmt.Sprintf("to.%s.task", name) }

// AgentResultTopic is where named agents publish results (coordinator pattern).
func AgentResultTopic(name string) string { return fmt.Sprintf("to.%s.result", name) }

// MailboxCrossTeamDropTopic is the audit-trail topic emitted whenever a
// cross-team mailbox message is dropped at receive (R5+I1 / security T3).
// A single global topic so operators can subscribe to "all cross-team
// drops" without per-agent fanout. The event payload carries the sender,
// receiver, and both teams.
func MailboxCrossTeamDropTopic() string { return "audit.mailbox.cross_team_drop" }

// ChildSpawnTopic is the structured spawn-intent topic emitted by the
// `delegate` tool just before forking a child (I2). Single global topic;
// recorder captures every event so a replayed session reconstructs the
// full spawn graph.
func ChildSpawnTopic() string { return "audit.child.spawn" }

// ChildPermissionDeniedTopic is emitted by SpawnPermissionHook /
// WorkspacePathHook when a child's tool call is rejected (I3). Single
// global topic; payload carries the rule + tool + denied value (bounded;
// no full tool inputs).
func ChildPermissionDeniedTopic() string { return "audit.child.permission_denied" }

// ChildStartedAuditTopic / ChildDoneAuditTopic / ChildFailedAuditTopic are the
// FLAT, journal-able mirrors of the name-templated child.<name>.* lifecycle
// topics. The spawner publishes child lifecycle on BOTH: the name topic (live
// workflow + terminal-translator subscription) AND these flat topics, because
// the SessionRecorder subscribes to FIXED topic strings — it cannot subscribe
// to `child.<name>.started` whose name is unknown ahead of time. Journaling
// these captures the COMPLETE delegation event (childName + childRunId +
// parentRunId + mode + kind) so the cold session/task-tree projection rebuilds
// the tree keyed by child RUN ID. Run-id keying is mandatory: the framework
// `task_started` carries the generic task-TYPE name (e.g. "prompt-worker"
// reused across an entire recursive delegate chain), so name-keyed nodes
// collapse and the edge graph is lost.
func ChildStartedAuditTopic() string { return "audit.child.started" }
func ChildDoneAuditTopic() string    { return "audit.child.done" }
func ChildFailedAuditTopic() string  { return "audit.child.failed" }

// Flat journal-able mirrors of the other name/id-templated runtime topics, so
// the SessionRecorder (fixed-topic subscriber) can journal them and the cold
// session view reconstructs comms / calls / ledger — not just the spawn tree.
// Publishers emit on BOTH the templated topic (live routing) and these.
//
//	audit.message          <- to.<name>.message + group.<team>.message (AgentMessageEvent)
//	audit.call             <- call.<name>.completed                    (CallCompletedEvent)
//	audit.ledger.snapshot  <- ledger.<id>.snapshot                     (LedgerSnapshotEvent)
//	audit.ledger.complete  <- ledger.<id>.complete                     (LedgerCompleteEvent)
func AgentMessageAuditTopic() string   { return "audit.message" }
func CallCompletedAuditTopic() string  { return "audit.call" }
func LedgerSnapshotAuditTopic() string { return "audit.ledger.snapshot" }
func LedgerCompleteAuditTopic() string { return "audit.ledger.complete" }

// RunQueueDepthTopic is the periodic ops-quality metric emitted by
// TaskService so dashboards / Grafana / oncall can subscribe to a steady
// pulse of run-state map sizes. Single global topic; payload counts the
// three run-state maps (Service.runs, bgManager.entries,
// LocalSpawner.children — last is a per-workflow sample). Tier-2 ops
// hook per `decisions/run-state-lifecycle.md`.
func RunQueueDepthTopic() string { return "metric.run.queue_depth" }

// ── Group/team broadcast topics ───────────────────────────────────────

// GroupMessageTopic broadcasts to all members of a team.
func GroupMessageTopic(team string) string { return fmt.Sprintf("group.%s.message", team) }

// GroupAnnouncementTopic broadcasts a team-wide announcement (higher-priority).
func GroupAnnouncementTopic(team string) string { return fmt.Sprintf("group.%s.announcement", team) }

// ── Workflow lifecycle topics (workflow emits, external observers subscribe) ──

// WorkflowStartedTopic signals a workflow instance began.
func WorkflowStartedTopic(id string) string { return fmt.Sprintf("workflow.%s.started", id) }

// WorkflowDoneTopic signals a workflow instance completed successfully.
func WorkflowDoneTopic(id string) string { return fmt.Sprintf("workflow.%s.done", id) }

// WorkflowFailedTopic signals a workflow instance failed.
func WorkflowFailedTopic(id string) string { return fmt.Sprintf("workflow.%s.failed", id) }

// ── Structured protocol topics (request/response with correlation ID) ──

// PlanApprovalTopic is where plan approval requests and responses flow.
func PlanApprovalTopic(id string) string { return fmt.Sprintf("protocol.%s.plan-approval", id) }

// PermissionTopic is where tool permission requests and responses flow.
func PermissionTopic(id string) string { return fmt.Sprintf("protocol.%s.permission", id) }

// ShutdownTopic is where shutdown handshake messages flow.
func ShutdownTopic(id string) string { return fmt.Sprintf("protocol.%s.shutdown", id) }

// ── Ledger topics (Magentic shared-state pattern) ──────────────────────

// LedgerSnapshotTopic is where the ledger publishes its current state on every change.
// Subscribers (specialists) get the latest plan/state snapshot.
func LedgerSnapshotTopic(id string) string { return fmt.Sprintf("ledger.%s.snapshot", id) }

// LedgerUpdateTopic is where peers send merge-patch updates to the ledger.
// Anyone who knows the ledger ID can publish updates.
func LedgerUpdateTopic(id string) string { return fmt.Sprintf("ledger.%s.update", id) }

// LedgerCompleteTopic terminates the ledger machine. Optional payload becomes
// the final output.
func LedgerCompleteTopic(id string) string { return fmt.Sprintf("ledger.%s.complete", id) }

// ── Cancel request (external → spawner) ────────────────────────────────

// CancelRequestTopic is the per-runID topic anyone can publish to in order
// to request cancellation of a specific spawned child. The Spawner
// subscribes to this topic for each child it spawns; on receipt it calls
// its internal Cancel(runID) which both fires `to.<name>.cancel` and
// terminates the child's context. This decouples external callers (GUI,
// CLI, sidecars) from the Spawner's internal state — they only need the
// runID, which `child.<name>.started` events expose.
//
// Payload: a `string` reason (e.g. "gui-cancel"). Empty reason is allowed.
func CancelRequestTopic(runID string) string { return fmt.Sprintf("cancel.request.%s", runID) }

// ── Liveness ───────────────────────────────────────────────────────────
