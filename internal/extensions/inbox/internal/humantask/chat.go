// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// THE CARD IN THE CHAT - a question asked from a conversation
// (its Chat names the run) shows inline in that chat as its human-task card,
// the very card « À faire » draws: the human task opens a display panel
// holding it, once its to-do is known, and places the panel in the
// conversation (agent.panel_shown, the chat panel bridge). Its buttons report
// the same to-do as « À faire »'s, so answered in either place it is answered
// once: the to-do's answer wakes the human task, which turns the card into
// its answered form - who answered, what, a relayed answer saying so - and
// completes the panel. A typed reply in the chat answers nothing: only the
// card, « À faire », or the person's report relayed with its proof does. The
// panel's events go out with the run's steps, after their checkpoint
// , so a restart neither loses nor repeats them.

// CardOf is a human task's card as « À faire » draws it, its answer forms
// resolved; answered, the card says by whom and what, with no answer area.
// The inbox's waiting list owns the card and sets it; nil, no card shows in a
// chat.
var CardOf func(ctx context.Context, t schedapi.Todo) render.Node

// CardNode is the id of the card's box in the panel: what the answered card
// replaces.
const CardNode = "human-task"

// inChat says whether the human task shows its card in a chat.
func (m *Machine) inChat() bool { return m.Spec.Chat != "" && CardOf != nil }

// cardBox is the panel's tree: the card in its box.
func cardBox(card render.Node) render.Node {
	return render.Node{"kind": "vbox", "id": CardNode, "children": []any{card}}
}

// openCard is the open of the card's panel: owned by the human task's run,
// its parent the chat's run, on the object the question is about.
func (m *Machine) openCard(ctx context.Context, d *data, t schedapi.Todo) ([]panel.Event, error) {
	tr := panel.NewTrack("")
	tree := render.InlineSpec(cardBox(CardOf(ctx, t)))
	ev, err := tr.Next(panel.Event{Kind: panel.KindOpen, Open: &panel.Open{
		Title: m.Spec.Title, Owner: engine.RunIDFromContext(ctx), Parent: m.Spec.Chat, Object: m.Spec.About,
		Answer: render.Answer{Tree: &tree},
	}})
	if err != nil {
		return nil, err
	}
	d.Panel = &tr
	return []panel.Event{ev}, nil
}

// placeCard places the card's panel in the chat's conversation, once.
func (m *Machine) placeCard(ctx context.Context, d *data) {
	if d.Panel == nil || d.Placed {
		return
	}
	d.Placed = true
	bus, err := application.UseService[engine.EventBus](ctx)
	if err != nil || bus == nil {
		return
	}
	_ = bus.Publish(agentapi.EventPanelShown, agentapi.PanelShownEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventPanelShown, Time: time.Now(), Emitter: m.Spec.Chat},
		PanelID:   d.Panel.ID, Title: m.Spec.Title,
	})
}

// answeredCard turns the card into its answered form and completes the
// panel; no panel, nothing.
func (m *Machine) answeredCard(ctx context.Context, d *data, t schedapi.Todo, a api.HumanAnswer) []panel.Event {
	if d.Panel == nil || CardOf == nil {
		return nil
	}
	var out []panel.Event
	if ev, err := d.Panel.Next(panel.Event{Kind: panel.KindOps, Ops: []render.Op{{
		Op: render.OpReplace, Target: CardNode, Node: cardBox(CardOf(ctx, t)),
	}}}); err == nil {
		out = append(out, ev)
	}
	status := "ok"
	if a.Status != "" && a.Status != schedapi.StatusSuccess {
		status = "failed"
	}
	if ev, err := d.Panel.Next(panel.Event{Kind: panel.KindComplete, Complete: &panel.Complete{Status: status, Result: a}}); err == nil {
		out = append(out, ev)
	}
	return out
}
