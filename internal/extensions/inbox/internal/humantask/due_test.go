// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestAQuestionRemindsThenEscalates: a question that says by
// when wakes at its follow-up date and reminds once, then at its due date
// escalates once - handed back to its list, its priority raised - each
// recorded on its to-do and announced; it shows overdue, and an answered one
// never does.
func TestAQuestionRemindsThenEscalates(t *testing.T) {
	ctx, todos, bus := setup(t)
	start := time.Date(2026, 10, 23, 9, 0, 0, 0, time.UTC)
	at := start
	was := clock
	clock = func() time.Time { return at }
	t.Cleanup(func() { clock = was })
	heard := map[string]int{}
	m := machineFor(t, map[string]any{"kind": "approve", "title": "Validate the release", "follow_up": "2h", "due": "2026-10-23T15:00:00Z", "priority": "P2"}, nil)
	st := m.InitialState(engine.Event{})

	res := step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	id := dataOf(st.Data).Todo
	for _, what := range []string{"reminded", "escalated"} {
		what := what
		bus.Subscribe(engine.HumanTaskDueTopic(id, what), func(string, any) { heard[what]++ })
	}
	td := todos.todos[id]
	if td.Work[api.WorkFollowUp] != "2026-10-23T11:00:00Z" || td.Work[api.WorkDue] != "2026-10-23T15:00:00Z" {
		t.Fatalf("the dates on the to-do = %v / %v; want 11:00 and 15:00", td.Work[api.WorkFollowUp], td.Work[api.WorkDue])
	}
	if res.Wait.Timeout != time.Hour {
		t.Errorf("first wait %v; want the hourly recheck before a follow-up two hours away", res.Wait.Timeout)
	}
	todos.todos[id].Assignee = nil

	at = start.Add(90 * time.Minute)
	res = step(t, ctx, m, &st, engine.Event{Type: "recheck"})
	if res.Wait.Timeout != 30*time.Minute {
		t.Errorf("wait %v; want to wake at the follow-up, 30 minutes away", res.Wait.Timeout)
	}
	if _, done := todos.todos[id].Work[api.WorkReminded]; done || heard["reminded"] != 0 {
		t.Fatal("reminded before its follow-up date")
	}

	at = start.Add(2*time.Hour + time.Minute)
	step(t, ctx, m, &st, engine.Event{Type: "recheck"})
	step(t, ctx, m, &st, engine.Event{Type: "recheck"})
	if todos.todos[id].Work[api.WorkReminded] == nil || heard["reminded"] != 1 {
		t.Errorf("past its follow-up: reminded %v, announced %d; want once", todos.todos[id].Work[api.WorkReminded], heard["reminded"])
	}
	if api.Overdue(*todos.todos[id], at) {
		t.Error("overdue before its due date")
	}

	at = start.Add(6*time.Hour + time.Minute)
	step(t, ctx, m, &st, engine.Event{Type: "recheck"})
	step(t, ctx, m, &st, engine.Event{Type: "recheck"})
	esc := todos.todos[id]
	if esc.Work[api.WorkEscalated] == nil || heard["escalated"] != 1 || esc.Priority != "P1" || esc.Assignee != nil {
		t.Errorf("past its due date: escalated %v announced %d priority %s assignee %v; want once, P1, back to its list", esc.Work[api.WorkEscalated], heard["escalated"], esc.Priority, esc.Assignee)
	}
	if !api.Overdue(*esc, at) {
		t.Error("not overdue past its due date")
	}
	esc.Result = &schedapi.Result{Status: schedapi.StatusSuccess}
	if api.Overdue(*esc, at) {
		t.Error("an answered question shows overdue")
	}
}

// TestADateThatDoesNotReadIsIgnored: a question asked with a date in no
// form it reads names none, never a wrong one.
func TestADateThatDoesNotReadIsIgnored(t *testing.T) {
	td := TodoOf(api.HumanTask{Kind: "approve", Title: "x", FollowUp: "tomorrow", Due: "-1h"}, "run-1", time.Now())
	if td.Work[api.WorkFollowUp] != nil || td.Work[api.WorkDue] != nil {
		t.Errorf("dates %v / %v; want none", td.Work[api.WorkFollowUp], td.Work[api.WorkDue])
	}
}
