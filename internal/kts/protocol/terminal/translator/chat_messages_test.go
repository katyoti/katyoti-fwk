// SPDX-License-Identifier: Apache-2.0

package translator_test

import (
	"sync"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestTranslator_TheRunsMessagesAreNodesOfItsChatPanel -: the
// agent's text and the person's answer become nodes of the session's chat
// panel, in order, handed to the panel sink beside the room; a message
// input on the panel is read back as the run's next user turn.
func TestTranslator_TheRunsMessagesAreNodesOfItsChatPanel(t *testing.T) {
	h := newHarness(t)
	var mu sync.Mutex
	var sent []panel.Event
	id := h.trans.UseChatPanel("", func(e panel.Event) { mu.Lock(); sent = append(sent, e); mu.Unlock() })
	if id == "" || h.trans.ChatPanelID() != id {
		t.Fatalf("UseChatPanel = %q, ChatPanelID = %q", id, h.trans.ChatPanelID())
	}

	h.bus.Publish(agentapi.EventTextDelta, agentapi.TextDeltaEvent{Turn: 1, Text: "What should "})
	h.bus.Publish(agentapi.EventTextDelta, agentapi.TextDeltaEvent{Turn: 1, Text: "I draw?"})
	h.bus.Publish(agentapi.EventLLMResponse, agentapi.LLMResponseEvent{Turn: 1})
	h.bus.Publish(agentapi.EventUserInput, agentapi.UserInputEvent{Turn: 1, Answer: "A board"})

	deadline := time.Now().Add(2 * time.Second)
	var last panel.Event
	for {
		mu.Lock()
		n := len(sent)
		if n > 0 {
			last = sent[n-1]
		}
		mu.Unlock()
		if n >= 3 || time.Now().After(deadline) {
			if n < 3 {
				t.Fatalf("chat panel events %d of 3: %+v", n, sent)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	first := sent[0]
	mu.Unlock()
	if first.Kind != panel.KindOpen || first.Panel != id {
		t.Fatalf("first event = %+v, want the panel's open", first)
	}
	kids, _ := last.Ops[0].Node["children"].([]any)
	if len(kids) != 2 {
		t.Fatalf("conversation = %v, want the agent's message then the person's", kids)
	}
	a, _ := kids[0].(render.Node)
	u, _ := kids[1].(render.Node)
	if a["content"] != "What should I draw?" || a["role"] != "assistant" || u["content"] != "A board" || u["role"] != "user" {
		t.Fatalf("conversation = %v", kids)
	}

	if text, ok := h.trans.ChatMessage(panel.Input{Panel: id, Kind: panel.InputMessage, Text: "Add a column"}); !ok || text != "Add a column" {
		t.Errorf("a message input on the chat panel = %q, %v; want the run's next turn", text, ok)
	}
}
