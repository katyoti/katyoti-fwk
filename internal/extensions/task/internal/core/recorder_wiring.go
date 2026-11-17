// SPDX-License-Identifier: Apache-2.0

package core

import (
	"sync"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// subscribeRecorder wires a SessionRecorder to EventBus topics for JSONL journal output.
//
// The recorder only writes events whose Emitter field matches `runID`. This
// prevents cross-agent leakage when multiple agents run on the same channel
// (e.g., parent + delegated children all share the EventBus). Without this
// filter, every recorder would record every event from every agent, polluting
// each session journal with foreign events.
func subscribeRecorder(bus engine.EventBus, recorder *engine.SessionRecorder, runID string) {
	llm := recorder.LLMJournal()
	user := recorder.UserJournal()

	// This recorder captures events for its run AND its whole delegated
	// subtree, so a reopened session journal (read by the GUI's Journal tab +
	// task tree) shows every run's turns, filterable per run. The DESCENDANT
	// SET is the membership test: seeded with this run, it grows as task.started
	// events whose parent is already a known descendant arrive (events come
	// parent-before-child). Events from OTHER top-level runs sharing the bus are
	// still rejected — their emitter is never a descendant of this root.
	descendants := map[string]bool{runID: true}
	var descMu sync.Mutex
	isDescendant := func(id string) bool {
		descMu.Lock()
		defer descMu.Unlock()
		return descendants[id]
	}
	addDescendant := func(id string) {
		descMu.Lock()
		descendants[id] = true
		descMu.Unlock()
	}

	// matches accepts an event whose emitter is this run or any descendant.
	// Empty emitter (published before the executor stamped it) is accepted to
	// avoid silently dropping.
	matches := func(payload any) bool {
		if e, ok := payload.(agentapi.AgentEvent); ok {
			emitter := e.GetEmitter()
			return emitter == "" || isDescendant(emitter)
		}
		// Unknown payload type — accept it (legacy behavior)
		return true
	}

	// sub wraps bus.Subscribe so the returned unsubscribe closure is captured by
	// the recorder; recorder.Close drains them. Without this, every completed
	// session leaks ~13 entries in MemoryEventBus.subs (corr-3 / perf-1).
	sub := func(topic string, h func(string, any)) {
		recorder.AddUnsub(bus.Subscribe(topic, h))
	}

	// LLM journal events
	sub(agentapi.EventSessionStart, func(_ string, payload any) {
		if matches(payload) {
			llm.LogWithTurn(engine.JournalSessionStart, 0, payload)
		}
	})
	sub(agentapi.EventSessionEnd, func(_ string, payload any) {
		if matches(payload) {
			llm.LogWithTurn(engine.JournalSessionEnd, 0, payload)
		}
	})
	sub(agentapi.EventLLMRequest, func(_ string, payload any) {
		if e, ok := payload.(agentapi.LLMRequestEvent); ok && matches(payload) {
			llm.LogWithTurn(engine.JournalLLMRequest, e.Turn, payload)
		}
	})
	sub(agentapi.EventLLMResponse, func(_ string, payload any) {
		if e, ok := payload.(agentapi.LLMResponseEvent); ok && matches(payload) {
			llm.LogWithTurn(engine.JournalLLMResponse, e.Turn, payload)
		}
	})
	sub(agentapi.EventToolCall, func(_ string, payload any) {
		if e, ok := payload.(agentapi.ToolCallEvent); ok && matches(payload) {
			llm.LogWithTurn(engine.JournalToolCall, e.Turn, payload)
		}
	})
	sub(agentapi.EventToolResult, func(_ string, payload any) {
		if e, ok := payload.(agentapi.ToolResultEvent); ok && matches(payload) {
			llm.LogWithTurn(engine.JournalToolResult, e.Turn, payload)
		}
	})
	sub(agentapi.EventToolError, func(_ string, payload any) {
		if e, ok := payload.(agentapi.ToolErrorEvent); ok && matches(payload) {
			llm.LogWithTurn(engine.JournalToolResult, e.Turn, payload)
		}
	})

	// Task-level lifecycle bookends — START and DONE. Framework-emitted by
	// TaskService.Run / LocalSpawner.Spawn (never the caller), so the spawn
	// graph + per-run start dimensions/input + terminal output rebuild from
	// the journal alone. A task.started whose parentRunId is already a known
	// descendant joins the descendant set (and is recorded); the LLM/user
	// events above then match on descendant membership too, so the whole
	// subtree's turns land in this session journal.
	sub(engine.EventTaskStarted, func(_ string, payload any) {
		rid, parent := taskRunAndParent(payload)
		if rid == "" {
			return
		}
		if rid == runID || isDescendant(parent) {
			addDescendant(rid)
			llm.LogWithTurn(engine.JournalTaskStarted, 0, payload)
		}
	})
	// Every transition of the run and its subtree (queued, waiting on what,
	// woken, resumed, interrupted, …).
	recorder.AddUnsub(bus.SubscribeAny(func(topic string, payload any) {
		if e, ok := payload.(*engine.RunLifecycleEvent); ok && engine.IsRunLifecycleTopic(topic) && isDescendant(e.RunID) {
			llm.LogWithTurn(engine.JournalRunLifecycle, 0, e)
		}
	}))
	sub(engine.EventTaskOutput, func(_ string, payload any) {
		rid, parent := taskRunAndParent(payload)
		if rid == "" {
			return
		}
		// task.output follows task.started, so rid is already in the set; the
		// parent fallback covers the (rare) out-of-order delivery.
		if rid == runID || isDescendant(rid) || isDescendant(parent) {
			llm.LogWithTurn(engine.JournalTaskOutput, 0, payload)
		}
	})

	// Team-lens narrative: every
	// TeamManager mutation lands as a team_event journal entry. The standard
	// emitter filter applies: the LEAD's session carries the lifecycle
	// narrative (created / member-joined / state / completed) because the
	// lead's run performs those mutations; a teammate's own messages journal
	// into ITS session. Cross-session aggregation joins on the correlation
	// envelope's team_id — by design, not by duplicating events.
	sub(engine.EventTeamEvent, func(_ string, payload any) {
		if matches(payload) {
			llm.LogWithTurn(engine.JournalTeamEvent, 0, payload)
		}
	})

	// Audit-trail: cross-team mailbox drops (R5+I1 / security T3 follow-up).
	// Captured by every recorder on the channel without emitter filtering —
	// the drop is a global security event, not run-scoped. Operators
	// replaying a session see who tried to message whom across team
	// boundaries. Message text is omitted from the event payload (set by
	// the publisher in messaging.go).
	sub(engine.MailboxCrossTeamDropTopic(), func(_ string, payload any) {
		llm.LogWithTurn(engine.JournalMailboxCrossTeamDrop, 0, payload)
	})

	// Audit-trail: child spawn intent (I2). Emitted by the delegate tool
	// before forking. Captured globally so a replayed session reconstructs
	// the full spawn graph regardless of which run was active when the
	// recorder hooked in. matches() filter via emitter would drop legitimate
	// events; no filtering here for the same reason as cross-team-drop.
	sub(engine.ChildSpawnTopic(), func(_ string, payload any) {
		llm.LogWithTurn(engine.JournalChildSpawn, 0, payload)
	})

	// Audit-trail: child permission denial (I3). Emitted by the spawn /
	// workspace-path / URL-allowlist hooks when they refuse a tool call.
	// Same global-capture semantics as above.
	sub(engine.ChildPermissionDeniedTopic(), func(_ string, payload any) {
		llm.LogWithTurn(engine.JournalChildPermissionDenied, 0, payload)
	})

	// Child lifecycle — the COMPLETE delegation events (childRunId + parentRunId
	// + label + mode + kind on start; output on done; error on failed). These
	// are the flat mirrors the spawner publishes alongside the name-templated
	// child.<name>.* topics (which the recorder can't subscribe to). The cold
	// task-tree projection rebuilds the delegation graph keyed by child run id
	// from these — the unambiguous key a recursive same-named delegate chain
	// preserves. Gated to this session's subtree by the descendant set (same
	// rule as task.started) so a sibling top-level run's children don't leak in.
	sub(engine.ChildStartedAuditTopic(), func(_ string, payload any) {
		rid, parent := childRunAndParent(payload)
		if rid == "" {
			return
		}
		if rid == runID || isDescendant(rid) || isDescendant(parent) {
			addDescendant(rid)
			llm.LogWithTurn(engine.JournalChildStarted, 0, payload)
		}
	})
	sub(engine.ChildDoneAuditTopic(), func(_ string, payload any) {
		rid, parent := childRunAndParent(payload)
		if rid == "" {
			return
		}
		if rid == runID || isDescendant(rid) || isDescendant(parent) {
			llm.LogWithTurn(engine.JournalChildDone, 0, payload)
		}
	})
	sub(engine.ChildFailedAuditTopic(), func(_ string, payload any) {
		rid, parent := childRunAndParent(payload)
		if rid == "" {
			return
		}
		if rid == runID || isDescendant(rid) || isDescendant(parent) {
			llm.LogWithTurn(engine.JournalChildFailed, 0, payload)
		}
	})

	// Inter-agent messages (mailbox + group). Globally captured like the other
	// audit channels: the payload carries agent NAMES, not run ids, so it can't
	// be gated by the descendant set; a per-session recorder on a team channel
	// records that team's traffic. Makes comms survive into the cold Team view.
	sub(engine.AgentMessageAuditTopic(), func(_ string, payload any) {
		llm.LogWithTurn(engine.JournalAgentMessage, 0, payload)
	})

	// Workflow `call` observability + Magentic ledger snapshots/complete — both
	// emitter-stamped (the workflow / ledger run), so the standard descendant
	// filter scopes them to this session's subtree.
	sub(engine.CallCompletedAuditTopic(), func(_ string, payload any) {
		if matches(payload) {
			llm.LogWithTurn(engine.JournalCall, 0, payload)
		}
	})
	sub(engine.LedgerSnapshotAuditTopic(), func(_ string, payload any) {
		if matches(payload) {
			llm.LogWithTurn(engine.JournalLedgerSnapshot, 0, payload)
		}
	})
	sub(engine.LedgerCompleteAuditTopic(), func(_ string, payload any) {
		if matches(payload) {
			llm.LogWithTurn(engine.JournalLedgerComplete, 0, payload)
		}
	})

	// User journal events
	sub(agentapi.EventMessageShown, func(_ string, payload any) {
		if e, ok := payload.(agentapi.MessageShownEvent); ok && matches(payload) {
			user.Log(engine.UserMessageShown, e.Turn, "", payload)
		}
	})
	sub(agentapi.EventPromptShown, func(_ string, payload any) {
		if matches(payload) {
			user.Log(engine.UserPromptShown, 0, "", payload)
		}
	})
	sub(agentapi.EventUserInput, func(_ string, payload any) {
		if matches(payload) {
			user.Log(engine.UserInput, 0, "", payload)
		}
	})
	sub(agentapi.EventError, func(_ string, payload any) {
		if matches(payload) {
			user.Log(engine.UserErrorShown, 0, "", payload)
		}
	})
}

// childRunAndParent extracts (runID, parentRunID) from a child lifecycle
// payload (started / done / failed), tolerating both pointer and value forms.
// Done/Failed carry no ParentRunID field but their RunID is already a known
// descendant by the time they fire, so the recorder gate matches on RunID.
func childRunAndParent(payload any) (string, string) {
	switch e := payload.(type) {
	case *engine.ChildStartedEvent:
		return e.RunID, e.ParentRunID
	case engine.ChildStartedEvent:
		return e.RunID, e.ParentRunID
	case *engine.ChildDoneEvent:
		return e.RunID, e.Correlation.ParentRunID
	case engine.ChildDoneEvent:
		return e.RunID, e.Correlation.ParentRunID
	case *engine.ChildFailedEvent:
		return e.RunID, e.Correlation.ParentRunID
	case engine.ChildFailedEvent:
		return e.RunID, e.Correlation.ParentRunID
	}
	return "", ""
}

// taskRunAndParent extracts (runID, parentRunID) from a task.started /
// task.output payload, tolerating both pointer and value forms. Returns empty
// runID when the payload is not a task lifecycle event.
func taskRunAndParent(payload any) (string, string) {
	switch e := payload.(type) {
	case *engine.TaskStartedEvent:
		return e.RunID, e.Correlation.ParentRunID
	case engine.TaskStartedEvent:
		return e.RunID, e.Correlation.ParentRunID
	case *engine.TaskOutputEvent:
		return e.RunID, e.Correlation.ParentRunID
	case engine.TaskOutputEvent:
		return e.RunID, e.Correlation.ParentRunID
	}
	return "", ""
}
