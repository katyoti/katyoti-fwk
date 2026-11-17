// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// TestTheConversationsMessagesAreNodesOfTheChatPanelInOrder -:
// the agent's and the person's text messages go out as nodes of the chat
// panel, in the order they came, beside the UI artifacts; the panel opens
// on the first of them.
func TestTheConversationsMessagesAreNodesOfTheChatPanelInOrder(t *testing.T) {
	b := New("", "sess", "run-1")
	first, err := b.Message(RoleAssistant, "m1", "Here is the board.")
	if err != nil {
		t.Fatal(err)
	}
	got := wire(t, first)
	if len(got) != 2 || got[0].Kind != panel.KindOpen || got[0].Open.Owner != "run-1" {
		t.Fatalf("a first message opens the panel, then shows: %+v", got)
	}
	ev, err := b.Mount(page("a1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Message(RoleUser, "u1", "Add a column"); err != nil {
		t.Fatal(err)
	}
	ev, err = b.Message(RoleAssistant, "m2", "Done.")
	if err != nil {
		t.Fatal(err)
	}
	op := onlyReplace(t, wire(t, ev), ArtifactsNode)
	kids, _ := op.Node["children"].([]any)
	var ids, roles []string
	for _, k := range kids {
		n, _ := k.(map[string]any)
		ids = append(ids, n["id"].(string))
		if r, ok := n["role"].(string); ok {
			roles = append(roles, r)
		}
	}
	want := []string{"message:m1", "artifact:a1", "message:u1", "message:m2"}
	if len(ids) != len(want) {
		t.Fatalf("conversation = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("conversation = %v, want %v (in the order they came)", ids, want)
		}
	}
	if len(roles) != 3 || roles[0] != RoleAssistant || roles[1] != RoleUser || roles[2] != RoleAssistant {
		t.Errorf("roles = %v, want who said each", roles)
	}

	// The same message again shows nothing new; a new text replaces its node.
	if ev, _ := b.Message(RoleAssistant, "m2", "Done."); len(ev) != 0 {
		t.Errorf("a message shown again answered %+v", ev)
	}
	ev, _ = b.Message(RoleAssistant, "m2", "Done - two columns.")
	if op := onlyReplace(t, wire(t, ev), MessageNodeID("m2")); op.Node["content"] != "Done - two columns." {
		t.Errorf("a changed message's node = %v", op.Node)
	}
}

// TestAMessageInputIsTheRunsNextUserTurn - the person's message on the chat
// panel's input is the run's next user turn; any other input, or another
// panel's, is not.
func TestAMessageInputIsTheRunsNextUserTurn(t *testing.T) {
	b := New("", "sess", "run-1")
	if text, ok := b.MessageOf(panel.Input{Panel: b.ID(), Kind: panel.InputMessage, Text: "go on"}); !ok || text != "go on" {
		t.Errorf("message input = %q, %v; want its text", text, ok)
	}
	for _, in := range []panel.Input{
		{Panel: b.ID(), Kind: panel.InputCancel},
		{Panel: b.ID(), Kind: panel.InputAction, Name: "x"},
		{Panel: panel.NewID(), Kind: panel.InputMessage, Text: "elsewhere"},
	} {
		if _, ok := b.MessageOf(in); ok {
			t.Errorf("%+v taken as a user turn", in)
		}
	}
}
