// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A request's state is rolled up from its to-dos, open and archived, beside
// its plan's end.
func TestARequestRollsUpFromItsTodos(t *testing.T) {
	td := func(state, waits string) api.WorkTodo {
		return api.WorkTodo{Todo: schedapi.Todo{ID: "b.TD-" + state, State: state, Waits: waits, For: schedapi.ForAgent}}
	}
	// A stopped to-do keeps its report.
	failed := td(schedapi.StateWaiting, "fail: the build broke")
	failed.Result = &schedapi.Result{Status: schedapi.StatusFail, Summary: "the build broke"}
	ask := func(state string) api.WorkTodo {
		return api.WorkTodo{Todo: schedapi.Todo{ID: "b.TD-ask", State: state, For: schedapi.ForPerson}, Packet: api.WorkPacket{Interaction: api.InteractionValidate}}
	}
	ended := func(status string) api.WorkTodo {
		return api.WorkTodo{Todo: schedapi.Todo{ID: "b.TD-done", Result: &schedapi.Result{Status: status}}}
	}
	for _, c := range []struct {
		name, plan string
		open, done []api.WorkTodo
		want       string
		ended      int
	}{
		{"none", api.PlanReady, nil, nil, api.RollupNew, 0},
		{"nothing started", api.PlanReady, []api.WorkTodo{td(schedapi.StateReady, ""), td(schedapi.StateWaiting, "after x")}, nil, api.RollupPlanned, 0},
		{"one pulled", api.PlanReady, []api.WorkTodo{td(schedapi.StateClaimed, "")}, nil, api.RollupInProgress, 0},
		{"one done", api.PlanReady, []api.WorkTodo{td(schedapi.StateWaiting, "after x")}, []api.WorkTodo{ended(schedapi.StatusSuccess)}, api.RollupInProgress, 1},
		{"a person's turn", api.PlanReady, []api.WorkTodo{ask(schedapi.StateReady)}, []api.WorkTodo{ended(schedapi.StatusSuccess)}, api.RollupWaiting, 1},
		{"a failure", api.PlanReady, []api.WorkTodo{failed, ask(schedapi.StateReady)}, nil, api.RollupBlocked, 0},
		{"its end refused", api.PlanBlocked, nil, []api.WorkTodo{ended(schedapi.StatusSuccess)}, api.RollupBlocked, 1},
		{"all removed", api.PlanReady, nil, []api.WorkTodo{{Todo: schedapi.Todo{ID: "b.TD-x"}}}, api.RollupDropped, 1},
		{"done", api.PlanDone, nil, []api.WorkTodo{ended(schedapi.StatusSuccess)}, api.RollupDone, 1},
		{"dropped", api.PlanDropped, []api.WorkTodo{td(schedapi.StateReady, "")}, nil, api.RollupDropped, 0},
	} {
		got := rollupOf(c.plan, c.open, c.done)
		if got.State != c.want || got.Done != c.ended || got.Total != len(c.open)+len(c.done) {
			t.Errorf("%s: %s %d/%d, want %s %d/%d", c.name, got.State, got.Done, got.Total, c.want, c.ended, len(c.open)+len(c.done))
		}
		if (got.State == api.RollupBlocked || got.State == api.RollupWaiting) && got.Note == "" {
			t.Errorf("%s: %s without saying why", c.name, got.State)
		}
	}
}
