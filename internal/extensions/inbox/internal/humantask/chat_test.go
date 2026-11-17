// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// withCard sets the card a chat shows - the to-do's title, and once answered
// who answered - for the test, putting the inbox's back after.
func withCard(t *testing.T) {
	t.Helper()
	was := CardOf
	CardOf = func(_ context.Context, td schedapi.Todo) render.Node {
		text := td.Title
		if a, ok := api.AnswerOf(td); ok {
			text += " - answered by " + a.By.ID
		}
		return render.Node{"kind": "card", "id": "inbox:" + td.ID, "children": []any{render.Node{"kind": "text", "content": text}}}
	}
	t.Cleanup(func() { CardOf = was })
}

// TestAQuestionAskedFromAChatShowsItsCardThereOnce: a human task
// asked from a conversation puts one to-do - the one « À faire » lists -,
// opens a panel holding its card, owned by its run under the chat's, and
// places it in that conversation once, however often it wakes; answered there
// or in « À faire » (the same to-do), the card turns into its answered form
// and the panel completes with the answer, which is the run's result.
func TestAQuestionAskedFromAChatShowsItsCardThereOnce(t *testing.T) {
	withCard(t)
	ctx, todos, bus := setup(t)
	var mu sync.Mutex
	var placed []agentapi.PanelShownEvent
	bus.Subscribe(agentapi.EventPanelShown, func(_ string, p any) {
		mu.Lock()
		defer mu.Unlock()
		placed = append(placed, p.(agentapi.PanelShownEvent))
	})
	m := machineFor(t, map[string]any{"kind": "approve", "about": "ui.F-061", "chat": "run-chat"},
		map[string]any{"title": "Validate the plan"})
	st := m.InitialState(engine.Event{})

	res := step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	if res.NextState != stateShow || res.Wait != nil || len(res.Panels) != 1 || res.Panels[0].Kind != panel.KindOpen {
		t.Fatalf("first step: next %q wait %v panels %+v; want its card's panel opened", res.NextState, res.Wait, res.Panels)
	}
	open := res.Panels[0].Open
	if open.Owner != "run-1" || open.Parent != "run-chat" || open.Object != "ui.F-061" || open.Answer.Tree == nil {
		t.Errorf("the open = %+v; want owned by the human task's run, under the chat's, on what it is about, with its card", open)
	}
	if todos.puts != 1 {
		t.Fatalf("puts = %d; want one to-do - the one « À faire » lists", todos.puts)
	}
	panelID := res.Panels[0].Panel

	res = step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	if res.Wait == nil || len(res.Panels) != 0 {
		t.Fatalf("second step: wait %v panels %+v; want it placed and waiting", res.Wait, res.Panels)
	}
	step(t, ctx, m, &st, engine.Event{Type: "recheck", Label: "recheck"})
	mu.Lock()
	if len(placed) != 1 || placed[0].PanelID != panelID || placed[0].Emitter != "run-chat" || placed[0].Title != "Validate the plan" {
		t.Fatalf("placed = %+v; want its panel placed once in the chat's conversation", placed)
	}
	mu.Unlock()
	if todos.puts != 1 {
		t.Errorf("puts = %d after waking; want it asked once", todos.puts)
	}

	id := dataOf(st.Data).Todo
	todos.answer(id, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "approved", Summary: "ok",
		By: cwmapi.Actor{ID: "jdoe", Kind: "person"}})
	res = step(t, ctx, m, &st, engine.Event{Type: engine.HumanTaskAnsweredTopic(id), Label: "answered"})
	if a, ok := res.Output.(api.HumanAnswer); !ok || a.Verdict != "approved" || res.NextState != "done" {
		t.Fatalf("answered: %#v next %q; want the answer as its result", res.Output, res.NextState)
	}
	if len(res.Panels) != 2 || res.Panels[0].Kind != panel.KindOps || res.Panels[1].Kind != panel.KindComplete {
		t.Fatalf("answered: panels %+v; want the card replaced, then the panel complete", res.Panels)
	}
	op := res.Panels[0].Ops[0]
	if op.Op != render.OpReplace || op.Target != CardNode || !containsText(op.Node, "answered by jdoe") {
		t.Errorf("the answered card = %+v; want the card's box replaced by its answered form", op)
	}
	if c := res.Panels[1].Complete; c.Status != "ok" {
		t.Errorf("complete = %+v; want ok", c)
	}
}

// TestAQuestionAskedOutsideAChatOpensNoPanel: with no chat, the human task
// asks in « À faire » alone - no panel, nothing placed.
func TestAQuestionAskedOutsideAChatOpensNoPanel(t *testing.T) {
	withCard(t)
	ctx, todos, bus := setup(t)
	n := 0
	bus.Subscribe(agentapi.EventPanelShown, func(string, any) { n++ })
	m := machineFor(t, map[string]any{"kind": "approve", "about": "ui.F-061"}, map[string]any{"title": "Validate"})
	st := m.InitialState(engine.Event{})
	res := step(t, ctx, m, &st, engine.Event{Type: engine.TickEvent})
	if res.Wait == nil || len(res.Panels) != 0 {
		t.Fatalf("first step: wait %v panels %+v; want a wait and no panel", res.Wait, res.Panels)
	}
	id := dataOf(st.Data).Todo
	todos.answer(id, schedapi.Result{Status: schedapi.StatusSuccess, Verdict: "approved", By: cwmapi.Actor{ID: "jdoe", Kind: "person"}})
	res = step(t, ctx, m, &st, engine.Event{Type: engine.HumanTaskAnsweredTopic(id)})
	if len(res.Panels) != 0 || n != 0 {
		t.Errorf("answered: panels %+v, placed %d; want none", res.Panels, n)
	}
}

func containsText(n render.Node, s string) bool {
	if c, ok := n["content"].(string); ok && strings.Contains(c, s) {
		return true
	}
	kids, _ := n["children"].([]any)
	for _, k := range kids {
		if m, ok := k.(render.Node); ok && containsText(m, s) {
			return true
		}
	}
	return false
}
