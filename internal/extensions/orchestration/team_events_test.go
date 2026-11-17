// SPDX-License-Identifier: Apache-2.0

package orchestration

import (
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestTeamManager_EmitsLifecycleNarrative: every TeamManager mutation emits exactly one typed
// TeamEvent on the flat team.event topic, with the correlation envelope
// (team_id + acting run) filled. The session log alone must suffice to
// rebuild the team narrative — this pins the emission half.
func TestTeamManager_EmitsLifecycleNarrative(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	var events []*engine.TeamEvent
	bus.Subscribe(engine.EventTeamEvent, func(_ string, payload any) {
		if evt, ok := payload.(*engine.TeamEvent); ok {
			events = append(events, evt)
		}
	})

	m := NewTeamManager(t.TempDir())
	m.SetEventBus(bus)
	m.SetActor("run-lead-1", "")

	team, err := m.CreateTeam("research-crew", "find things", "lead")
	if err != nil {
		t.Fatalf("CreateTeam: %v", err)
	}
	if team.ID == "" {
		t.Fatal("team must carry a stable ID")
	}
	if team.State != engine.TeamStateForming {
		t.Errorf("new team state = %q, want forming", team.State)
	}

	if err := m.AddMember(team.Name, TeamMember{AgentID: "task-1", Name: "alpha", Mode: "worker"}); err != nil {
		t.Fatalf("AddMember: %v", err)
	}
	m.EmitMessage(team.Name, "alpha", "lead", "starting research")
	if err := m.SetMemberActive(team.Name, "task-1", false); err != nil {
		t.Fatalf("SetMemberActive: %v", err)
	}
	if err := m.CompleteTeam(team.Name, false, "all goals met"); err != nil {
		t.Fatalf("CompleteTeam: %v", err)
	}
	// Idempotent: a second complete emits nothing.
	if err := m.CompleteTeam(team.Name, false, "again"); err != nil {
		t.Fatalf("CompleteTeam (2nd): %v", err)
	}

	wantActions := []string{
		engine.TeamActionCreated,
		engine.TeamActionMemberJoined,
		engine.TeamActionStateChange, // forming → active on first member
		engine.TeamActionMessage,
		engine.TeamActionMemberStateChange,
		engine.TeamActionCompleted,
	}
	if len(events) != len(wantActions) {
		got := make([]string, len(events))
		for i, e := range events {
			got[i] = e.Action
		}
		t.Fatalf("got %d events %v, want %v", len(events), got, wantActions)
	}
	for i, want := range wantActions {
		e := events[i]
		if e.Action != want {
			t.Errorf("event[%d].Action = %q, want %q", i, e.Action, want)
		}
		if e.TeamID != team.ID || e.Correlation.TeamID != team.ID {
			t.Errorf("event[%d] team correlation missing: TeamID=%q corr=%q", i, e.TeamID, e.Correlation.TeamID)
		}
		if e.Correlation.RunID != "run-lead-1" || e.GetEmitter() != "run-lead-1" {
			t.Errorf("event[%d] acting-run correlation missing: corr.run=%q emitter=%q", i, e.Correlation.RunID, e.GetEmitter())
		}
	}

	// Spot-check the narrative payloads.
	if events[1].MemberID != "alpha" || events[1].State != engine.TeamStateActive {
		t.Errorf("member-joined event = %+v, want member alpha on active team", events[1])
	}
	if events[3].Detail["to"] != "lead" || events[3].Detail["summary"] != "starting research" {
		t.Errorf("message event detail = %#v", events[3].Detail)
	}
	if events[5].State != engine.TeamStateDone || events[5].Detail["summary"] != "all goals met" {
		t.Errorf("completed event = %+v", events[5])
	}
}
