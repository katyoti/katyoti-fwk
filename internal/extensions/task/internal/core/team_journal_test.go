// SPDX-License-Identifier: Apache-2.0

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestRecorder_JournalsTeamNarrative — the journaling half of the
// team-lens narrative: team.event payloads emitted on
// the bus land as team_event entries in the session's llm.jsonl (standard
// emitter filter), so the SESSION LOG ALONE rebuilds the team narrative.
func TestRecorder_JournalsTeamNarrative(t *testing.T) {
	bus := engine.NewMemoryEventBus()
	rec, err := engine.NewSessionRecorder(t.TempDir(), "team-narrative", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	subscribeRecorder(bus, rec, "run-lead-9")

	emit := func(action, teamID, member string) {
		evt := &engine.TeamEvent{Action: action, TeamID: teamID, TeamName: "crew", MemberID: member}
		evt.Correlation = engine.Correlation{RunID: "run-lead-9", TeamID: teamID, MemberID: member}
		evt.StampEvent(engine.EventTeamEvent, "run-lead-9")
		bus.Publish(engine.EventTeamEvent, evt)
	}
	emit(engine.TeamActionCreated, "team-xyz", "")
	emit(engine.TeamActionMemberJoined, "team-xyz", "alpha")
	emit(engine.TeamActionCompleted, "team-xyz", "")

	// An event from ANOTHER run must NOT land in this session's journal.
	foreign := &engine.TeamEvent{Action: engine.TeamActionCreated, TeamID: "team-foreign"}
	foreign.StampEvent(engine.EventTeamEvent, "run-other")
	bus.Publish(engine.EventTeamEvent, foreign)

	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(rec.SessionDir(), "llm.jsonl"))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	content := string(raw)

	if got := strings.Count(content, `"type":"team_event"`); got != 3 {
		t.Errorf("team_event entries = %d, want 3 (emitter filter must exclude run-other):\n%s", got, content)
	}
	for _, want := range []string{
		engine.TeamActionCreated, engine.TeamActionMemberJoined, engine.TeamActionCompleted,
		`"teamId":"team-xyz"`, `"runId":"run-lead-9"`, `"member_id":"alpha"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("journal missing %q", want)
		}
	}
	if strings.Contains(content, "team-foreign") {
		t.Error("foreign run's team event leaked into this session's journal")
	}
}
