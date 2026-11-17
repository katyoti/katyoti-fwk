// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"encoding/json"
)

// SessionClient is the agent's COUNTERPART for one run — the single seam for
// the whole relationship, both directions:
//
//   - OUTPUT (agent → counterpart): the agent's content stream + its terminal
//     completion. Delivered by the "client bridge" (the translator) which
//     subscribes to the agent's EventBus and forwards to the run's client.
//   - INPUT (counterpart → agent): prompts + permissions — the embedded
//     HumanGate, unchanged.
//
// One interface, many implementations, picked per-run by a factory from the
// UserComms.Input dimension:
//
//	direct            → terminalClient   (GUI / TUI / CLI over the Room)
//	inbox             → inboxClient      (CWM approval-request + task Suspend)
//	bubble_to_parent  → parentAgentClient (agent-as-human: AskBroker + Suspend)
//	via_agent         → parentAgentClient
//	via_leader        → leaderClient     (team leader mailbox + Suspend)
//	none              → silentClient
//
// Layering: the GATE is an agent concern (this interface), the WAIT is the task
// concern (TaskService.SuspensionManager), the ROUTING is an agent-comms
// concern (AskBroker / Mailbox). The agent loop's output stays on the EventBus
// (decoupled, multi-subscriber); the terminal impl bridges SendContent back to
// the Room fan-out so cowork + the session recorder are untouched.
type SessionClient interface {
	// SendContent delivers one unit of the agent's output stream to the
	// counterpart. Ephemeral events (streaming deltas) may be dropped under
	// back-pressure; authoritative events (persisted messages, lifecycle)
	// must not be.
	SendContent(ctx context.Context, ev ContentEvent) error

	// Complete delivers the agent's terminal result. For a terminal client it
	// drives the Output view; for a parent-agent client it is the child's
	// structured answer; for an inbox client it can write a result item.
	Complete(ctx context.Context, ev CompleteEvent) error

	// HumanGate is the INPUT half — Prompt + PromptPermission + Capabilities.
	HumanGate
}

// ContentEvent is one unit of an agent's output stream. Body is the
// already-shaped wire event (terminal-protocol JSON today) kept opaque so this
// contract carries no dependency on the transport's event package; Type is its
// discriminator; Ephemeral marks droppable streaming chunks (msg.delta) versus
// reliable/persisted events (msg.new / msg.update / stats / lifecycle).
type ContentEvent struct {
	SessionID string          `json:"sessionId"`
	Type      string          `json:"type"`
	Ephemeral bool            `json:"ephemeral,omitempty"`
	Body      json.RawMessage `json:"body"`
}

// CompleteEvent is an agent's terminal result, with the structured Output
// shaped by the task's output schema (when the task declared one).
type CompleteEvent struct {
	SessionID string          `json:"sessionId"`
	Summary   string          `json:"summary,omitempty"`
	Output    json.RawMessage `json:"output,omitempty"`
	Error     string          `json:"error,omitempty"`
}
