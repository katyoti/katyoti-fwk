// SPDX-License-Identifier: Apache-2.0

package server

import (
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
)

// TestAChatPanelMessageReachesTheRun -: a message the person
// sends on the chat panel's input is delivered to the run as its next user
// turn (the session's gate answer); a cancel, an action or a payload that
// does not decode delivers nothing.
func TestAChatPanelMessageReachesTheRun(t *testing.T) {
	id := panel.NewID()
	message := func(in panel.Input) (string, bool) {
		if in.Panel != id || in.Kind != panel.InputMessage {
			return "", false
		}
		return in.Text, true
	}
	var got []string
	handle := chatInput(message, func(s string) { got = append(got, s) })
	for _, in := range []panel.Input{
		{Panel: id, Kind: panel.InputMessage, Text: "go on"},
		{Panel: id, Kind: panel.InputCancel},
		{Panel: id, Kind: panel.InputAction, Name: "x"},
	} {
		raw, _ := json.Marshal(in)
		handle(raw)
	}
	handle([]byte(`{"panel":"nope"}`))
	if len(got) != 1 || got[0] != "go on" {
		t.Fatalf("delivered %v, want the person's message alone", got)
	}
}
