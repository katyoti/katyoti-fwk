// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A RELEASE'S WORK, SORTED ONCE - two person steps
// come oldest first; a failed to-do puts its request under Blocked, once even
// when its plan is blocked too (the plan's block says why); a work queue that
// cannot be read makes every empty list say it is unknown.
func TestAReleasesWorkIsSortedOnce(t *testing.T) {
	todo := func(for_, state, step, since, waits string) api.WorkTodo {
		var w api.WorkTodo
		w.For, w.State, w.Since, w.Waits, w.Packet.Step = for_, state, since, waits, step
		return w
	}
	failed := todo(schedapi.ForAgent, schedapi.StateWaiting, "implement", "", schedapi.StatusFail+": the build broke")
	failed.Result = &schedapi.Result{Status: schedapi.StatusFail, Summary: "the build broke"}
	work := []requestWork{
		{id: "b.RQ-2", title: "Newer ask", todos: []api.WorkTodo{todo(schedapi.ForPerson, schedapi.StateReady, "review", "2026-10-14T10:00:00Z", "")}},
		{id: "b.RQ-1", title: "Older ask", todos: []api.WorkTodo{todo(schedapi.ForPerson, schedapi.StateReady, "review", "2026-10-13T09:00:00Z", "")}},
		{id: "b.RQ-3", title: "Failed", todos: []api.WorkTodo{failed, failed}},
		{id: "b.RQ-4", title: "Blocked and failed", plan: api.PlanFields{State: api.PlanBlocked, Note: "the engine is down"}, todos: []api.WorkTodo{failed}},
	}
	claimed := todo(schedapi.ForAgent, schedapi.StateClaimed, "analyse", "", "")
	claimed.Claim = &schedapi.Claim{By: cwmapi.Actor{ID: "claude-x", Kind: "agent"}, At: "2026-10-14T11:00:00Z"}
	work = append(work, requestWork{id: "b.RQ-5", title: "Running", todos: []api.WorkTodo{claimed}})

	needs, running, blocked := workRows(work)
	if len(needs) != 2 || needs[0]["id"] != "b.RQ-1" || needs[1]["id"] != "b.RQ-2" {
		t.Errorf("needs %v; want b.RQ-1 (older) then b.RQ-2", needs)
	}
	if len(running) != 1 || running[0]["meta"] != "analyse · claude-x · since 2026-10-14T11:00:00Z" {
		t.Errorf("running %v; want b.RQ-5's step, who and since when", running)
	}
	var rows []string
	for _, b := range blocked {
		rows = append(rows, b["id"].(string)+"="+b["state"].(string)+":"+b["meta"].(string))
	}
	want := []string{"b.RQ-3=failed:implement - " + schedapi.StatusFail + ": the build broke", "b.RQ-4=blocked:the engine is down"}
	if strings.Join(rows, "|") != strings.Join(want, "|") {
		t.Errorf("blocked %v; want %v - each request once", rows, want)
	}

	if got := unknownOr(errors.New("no scheduler"), "Nothing runs."); got != "Unknown - the work queue could not be read: no scheduler" {
		t.Errorf("an unread queue says %q", got)
	}
	if got := unknownOr(nil, "Nothing runs."); got != "Nothing runs." {
		t.Errorf("a read queue says %q", got)
	}
}
