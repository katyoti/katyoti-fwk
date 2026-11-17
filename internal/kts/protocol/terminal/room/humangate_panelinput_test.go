// SPDX-License-Identifier: Apache-2.0

package room

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// sentInputs gives the gate a panel wire that records each input sent.
func sentInputs(g *HumanGate) *[]panel.Input {
	var got []panel.Input
	g.SetPanelInput(func(_ context.Context, in panel.Input) error {
		got = append(got, in)
		return nil
	})
	return &got
}

// A form submitted on the chat card for an artifact that is a panel (the
// question ask-user-data opened) goes to that panel's input, its value the
// submitted object.
func TestDeliverPanelInput_Submit(t *testing.T) {
	g := NewHumanGate()
	got := sentInputs(g)
	id := panel.NewID()
	if !g.DeliverPanelInput(context.Background(), id, "submit", json.RawMessage(`{"name":"x"}`)) {
		t.Fatal("a submit for a panel artifact must go to the panel's input")
	}
	if len(*got) != 1 {
		t.Fatalf("sent %d inputs, want 1", len(*got))
	}
	in := (*got)[0]
	v, _ := in.Value.(map[string]any)
	if in.Panel != id || in.Kind != panel.InputSubmit || v["name"] != "x" {
		t.Fatalf("sent %+v, want the submit of %s carrying the form's object", in, id)
	}
}

func TestDeliverPanelInput_Cancel(t *testing.T) {
	g := NewHumanGate()
	got := sentInputs(g)
	id := panel.NewID()
	if !g.DeliverPanelInput(context.Background(), id, "cancel", nil) {
		t.Fatal("a cancel for a panel artifact must go to the panel's input")
	}
	if len(*got) != 1 || (*got)[0].Kind != panel.InputCancel || (*got)[0].Panel != id {
		t.Fatalf("sent %+v, want one cancel of %s", *got, id)
	}
}

// Non-terminal events, artifacts that are no panel, and a gate with no
// panel wire send nothing: the transport falls back to the envelope path.
func TestDeliverPanelInput_NotDelivered(t *testing.T) {
	g := NewHumanGate()
	got := sentInputs(g)
	id := panel.NewID()
	if g.DeliverPanelInput(context.Background(), id, "change", json.RawMessage(`{}`)) {
		t.Fatal("a change event must not go to the panel (not terminal)")
	}
	if g.DeliverPanelInput(context.Background(), "ask-user-data:run-1", "submit", json.RawMessage(`{}`)) {
		t.Fatal("a submit for an artifact that is no panel must not go to a panel")
	}
	if len(*got) != 0 {
		t.Fatalf("sent %+v, want nothing", *got)
	}
	bare := NewHumanGate()
	if bare.DeliverPanelInput(context.Background(), id, "submit", json.RawMessage(`{}`)) {
		t.Fatal("a gate with no panel wire must leave the event to the envelope path")
	}
}
