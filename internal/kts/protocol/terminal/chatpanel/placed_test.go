// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestAPanelPlacedInTheConversationIsABareNodeWhereItCame -: a
// question asked from a chat places its card's panel in the conversation;
// the chat panel shows it as a bare `panel` node after what came before it,
// once however often it is placed; a placement naming no panel is refused.
func TestAPanelPlacedInTheConversationIsABareNodeWhereItCame(t *testing.T) {
	b := New("", "sess", "run-1")
	if _, err := b.Message(RoleAssistant, "m1", "I need your validation."); err != nil {
		t.Fatal(err)
	}
	card := panel.NewID()
	ev, err := b.Panel(agentapi.PanelShownEvent{PanelID: card, Title: "Validate the plan"})
	if err != nil {
		t.Fatal(err)
	}
	op := onlyReplace(t, wire(t, ev), ArtifactsNode)
	kids, _ := op.Node["children"].([]any)
	if len(kids) != 2 {
		t.Fatalf("conversation = %v; want the message, then the card", kids)
	}
	n, _ := kids[1].(map[string]any)
	if n["kind"] != "panel" || n["panelId"] != card || n["id"] != PanelNodeID(card) || n["bare"] != true || n["title"] != "Validate the plan" {
		t.Errorf("the placed node = %v; want a bare panel node of the card's panel", n)
	}

	if ev, err := b.Panel(agentapi.PanelShownEvent{PanelID: card}); err != nil || len(ev) != 0 {
		t.Errorf("placed again: %v %v; want nothing new", ev, err)
	}
	if _, err := b.Panel(agentapi.PanelShownEvent{PanelID: "run-7"}); err == nil {
		t.Error("a placement naming no panel was taken")
	}
}
