// SPDX-License-Identifier: Apache-2.0

package translator_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// : a run whose model call failed says why in its chat - its
// error, with its message key for the GUI to say it in the person's language
// - and its end says it failed: an error line, its session "error", never
// "done"; the session's journal keeps the error.
func TestAFailedModelCallIsSaidInTheChatAndEndsTheSessionFailed(t *testing.T) {
	h := newHarness(t)
	why := "anthropic: key ANTHROPIC_FOUNDRY_API_KEY is not set"
	key := &usermsg.Msg{Key: "agent.modelKeyMissing", Params: map[string]any{"model": "foundry-haiku", "reason": why}}
	h.bus.Publish(agentapi.EventError, agentapi.ErrorEvent{EventBase: agentapi.EventBase{Type: agentapi.EventError}, Turn: 1, Error: why, Msg: key})
	h.bus.Publish(agentapi.EventComplete, agentapi.CompleteEvent{
		EventBase: agentapi.EventBase{Type: agentapi.EventComplete},
		Status:    "error", Turns: 1, Duration: time.Second,
		Error: "LLM error on turn 1: " + why, Msg: key,
	})

	content := func() terminal.SystemContent {
		t.Helper()
		var ev terminal.MsgNewEvent
		if err := json.Unmarshal(recvOne(t, h.ch, time.Second), &ev); err != nil {
			t.Fatal(err)
		}
		var sc terminal.SystemContent
		ev.Msg.Content.As(&sc)
		return sc
	}
	said := content()
	if said.Level != "error" || said.Message != why || said.Msg == nil || said.Msg.Key != key.Key || said.Msg.Params["model"] != "foundry-haiku" {
		t.Fatalf("the error is said as %+v; want the error and its key", said)
	}
	end := content()
	if end.Level != "error" || strings.HasPrefix(end.Message, " (") {
		t.Fatalf("the end is said as %+v; want an error line that names the failure", end)
	}
	if sess, _ := h.room.GetSession("s1"); sess.Status != "error" {
		t.Fatalf("the session ends %q; want error", sess.Status)
	}
	history, err := h.room.GetHistory("s1")
	if err != nil {
		t.Fatal(err)
	}
	kept := false
	for _, m := range history {
		var sc terminal.SystemContent
		m.Content.As(&sc)
		kept = kept || sc.Message == why
	}
	if !kept {
		t.Fatal("the session's journal does not keep the error")
	}
}
