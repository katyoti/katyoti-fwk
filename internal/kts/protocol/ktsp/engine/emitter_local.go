// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// LocalEmitter implements agentapi.Observer by writing to the existing
// SessionRecorder (dual JSONL journals). Used when the engine runs
// standalone (not as a KTSP container).
type LocalEmitter struct {
	recorder *machine.SessionRecorder
}

// NewLocalEmitter creates an emitter that writes to the SessionRecorder.
// If recorder is nil or disabled, events are silently discarded.
func NewLocalEmitter(recorder *machine.SessionRecorder) *LocalEmitter {
	return &LocalEmitter{recorder: recorder}
}

// OnEvent implements agentapi.Observer. Events are dispatched to the
// appropriate journal based on event type.
func (e *LocalEmitter) OnEvent(_ context.Context, eventType string, turn int, data any) {
	if e.recorder == nil {
		return
	}

	switch eventType {
	// LLM journal events
	case agentapi.EventSessionStart:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalSessionStart, turn, data)
	case agentapi.EventSessionEnd:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalSessionEnd, turn, data)
	case agentapi.EventLLMRequest:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalLLMRequest, turn, data)
	case agentapi.EventLLMResponse:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalLLMResponse, turn, data)
	case agentapi.EventToolCall:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalToolCall, turn, data)
	case agentapi.EventToolResult:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalToolResult, turn, data)
	case agentapi.EventToolError:
		e.recorder.LLMJournal().LogWithTurn(machine.JournalToolResult, turn, data)

	// User journal events
	case agentapi.EventMessageShown:
		e.recorder.UserJournal().Log(machine.UserMessageShown, turn, "", data)
	case agentapi.EventPromptShown:
		e.recorder.UserJournal().Log(machine.UserPromptShown, turn, "", data)
	case agentapi.EventUserInput:
		e.recorder.UserJournal().Log(machine.UserInput, turn, "", data)
	case agentapi.EventToolFeedback:
		e.recorder.UserJournal().Log(machine.UserToolFeedback, turn, "", data)
	case agentapi.EventError:
		e.recorder.UserJournal().Log(machine.UserErrorShown, turn, "", data)

		// EventTurnStart, EventTextDelta, EventComplete: no journal entry needed
	}
}

// Close closes the underlying recorder.
func (e *LocalEmitter) Close() error {
	if e.recorder == nil {
		return nil
	}
	return e.recorder.Close()
}
