// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// « Running now » never shows a step whose claim lapsed as running: it reads
// lapsed, with its holder and when it lapsed, and offers to release it; a live
// claim still reads running.
func TestALapsedClaimIsNotRunning(t *testing.T) {
	now := time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC)
	was := deckNow
	deckNow = func() time.Time { return now }
	t.Cleanup(func() { deckNow = was })

	pulled := func(id, holder string, until time.Time, state string) stand {
		var todo api.WorkTodo
		todo.ID, todo.Request, todo.State, todo.Position = id+".TD-1", id, state, 1
		todo.Claim = &schedapi.Claim{By: cwmapi.Actor{ID: holder, Kind: "agent"}, At: until.Add(-15 * time.Minute).Format(time.RFC3339Nano), Until: until.Format(time.RFC3339Nano)}
		todo.Packet.Step = "implement"
		return standOf(api.PlannedObject{ID: id, Title: id, Module: strings.Split(id, ".")[0]}, []api.WorkTodo{todo}, false, nil)
	}
	live := pulled("ui.RQ-1", "claude-a", now.Add(10*time.Minute), schedapi.StateClaimed)
	lapsed := pulled("mail.F-6", "claude-in020", now.Add(-24*time.Hour), schedapi.StateClaimed)
	reported := pulled("shop.RQ-11", "claude-b", now.Add(-time.Hour), schedapi.StateReported)

	if live.state != stateLabels[standRunning] || reported.state != stateLabels[standRunning] {
		t.Fatalf("a live claim reads %q, a reported step %q; want both running", live.state, reported.state)
	}
	if lapsed.state != stateLabels[standLapsed] || !strings.Contains(lapsed.why, "claude-in020") {
		t.Fatalf("a claim past its until reads %q (%q); want lapsed, naming its holder", lapsed.state, lapsed.why)
	}

	raw, _ := json.Marshal(runningNow("", []stand{live, lapsed, reported}))
	var section struct {
		Children []struct {
			Data    []map[string]any `json:"data"`
			Actions []map[string]any `json:"actions"`
		} `json:"children"`
	}
	_ = json.Unmarshal(raw, &section)
	var list []map[string]any
	var actions []map[string]any
	for _, c := range section.Children {
		if c.Data != nil {
			list, actions = c.Data, c.Actions
		}
	}
	card := map[string]map[string]any{}
	for _, c := range list {
		card[c["id"].(string)] = c
	}
	if c := card["mail.F-6"]; c == nil || c["state"] != "lapsed" || c["lapsed"] != "mail.F-6.TD-1" ||
		!strings.Contains(c["meta"].(string), "claude-in020") || !strings.Contains(c["meta"].(string), "silent since") {
		t.Errorf("the lapsed card: %v; want state lapsed, its to-do to release, its holder and since when", c)
	}
	if c := card["ui.RQ-1"]; c == nil || c["state"] != "running" || c["lapsed"] != nil {
		t.Errorf("the live card: %v; want running, nothing to release", c)
	}
	released := false
	for _, a := range actions {
		if a["uri"] == todoReleaseURI && a["when"] == "lapsed" {
			released = true
		}
	}
	if !released {
		t.Errorf("Running now offers no release of a lapsed step: actions %v", actions)
	}

	// The board by state has a lane for it: a lapsed card never vanishes.
	if board, _ := json.Marshal(byStateTab([]stand{lapsed}, nil, "week")); !strings.Contains(string(board), `"id":"lapsed"`) {
		t.Errorf("the board by state has no lapsed lane: %s", board)
	}
}
