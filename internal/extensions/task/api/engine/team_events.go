// SPDX-License-Identifier: Apache-2.0

package engine

import (
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// EventTeamEvent is the single flat bus topic for ALL team lifecycle
// events. One topic — not
// per-team `team.<id>.*` topics — follows the `task.output` precedent:
// the session recorder subscribes a fixed topic set per run and filters
// by `Emitter == runID`, which cannot enumerate team-scoped topics that
// don't exist yet at subscribe time. The TeamID discriminates inside the
// payload (and in the correlation envelope).
const EventTeamEvent = "team.event"

// Team lifecycle actions carried by TeamEvent.Action.
const (
	TeamActionCreated           = "created"
	TeamActionMemberJoined      = "member-joined"
	TeamActionMemberLeft        = "member-left"
	TeamActionMemberStateChange = "member-state-changed"
	TeamActionMessage           = "message"
	TeamActionItemShared        = "item-shared"
	TeamActionStateChange       = "state-changed"
	TeamActionCompleted         = "completed"
	TeamActionFailed            = "failed"
)

// Team FSM states (explicit lifecycle):
// forming → active → done | failed. Member states: working → done | left.
const (
	TeamStateForming = "forming"
	TeamStateActive  = "active"
	TeamStateDone    = "done"
	TeamStateFailed  = "failed"

	MemberStateWorking = "working"
	MemberStateDone    = "done"
	MemberStateLeft    = "left"
)

// TeamEvent is the typed payload for EventTeamEvent — the team-lens
// narrative of a session. Every mutation of a team (creation, membership,
// messages, shared items, state transitions, completion) emits exactly one
// of these, journaled as a `team_event` entry, so the session log alone
// suffices to rebuild the full team view.
//
// Embeds agentapi.EventBase (recorder Emitter filter + registry
// deserialization) and HasCorrelation (the universal envelope — TeamID and
// MemberID are duplicated as direct fields for consumer convenience; the
// envelope adds run/parent/session linkage to the task-tree lens).
type TeamEvent struct {
	agentapi.EventBase
	HasCorrelation
	Action   string         `json:"action"`
	TeamID   string         `json:"team_id"`
	TeamName string         `json:"team_name,omitempty"`
	State    string         `json:"state,omitempty"`     // team FSM state AFTER the transition
	MemberID string         `json:"member_id,omitempty"` // persona, for member-scoped actions
	Detail   map[string]any `json:"detail,omitempty"`    // action-specific extras (message summary, shared-item ref, …)
}

// init registers TeamEvent with the agent event registry so JSON
// deserialization (ktw-utils session tooling, GUI translator) can
// discriminate on the `type` field.
func init() {
	agentapi.EventRegistry.Register(EventTeamEvent, TeamEvent{})
}
