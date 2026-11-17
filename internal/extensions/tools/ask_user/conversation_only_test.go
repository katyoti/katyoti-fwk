// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"errors"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
)

// TestAskUserIsAConversationOnceIN039IsOn (decision
// ): with IN-039's switch on, ask-user takes a text in a run that
// has a conversation and refuses a select, a confirm or a form - a validation,
// asked with request-validation - and any ask in a run with no conversation;
// request-validation is offered beside it. With the switch off nothing changes.
func TestAskUserIsAConversationOnceIN039IsOn(t *testing.T) {
	direct := agentapi.WithUserCommsInput(context.Background(), "direct")
	if err := conversationOnly(direct, AskInput{Question: "Which colour?"}); err != nil {
		t.Errorf("a text in the chat: %v; want it asked", err)
	}
	for _, typ := range []string{"select", "confirm", "form"} {
		if err := conversationOnly(direct, AskInput{Question: "?", Type: typ}); !errors.Is(err, ErrNotAConversation) {
			t.Errorf("a %s: %v; want it refused for request-validation", typ, err)
		}
	}
	for _, ch := range []string{"inbox", "none"} {
		if err := conversationOnly(agentapi.WithUserCommsInput(context.Background(), ch), AskInput{Question: "?"}); !errors.Is(err, ErrNotAConversation) {
			t.Errorf("a text in a run whose channel is %s: %v; want it refused", ch, err)
		}
	}
	if err := conversationOnly(agentapi.WithUserCommsInput(context.Background(), "bubble_to_parent"), AskInput{Question: "?"}); err != nil {
		t.Errorf("a child asking its parent: %v; want it asked", err)
	}
	again := agentapi.WithAwaitResume(agentapi.WithUserCommsInput(context.Background(), "inbox"), map[string]any{"request": "x"})
	if err := conversationOnly(again, AskInput{Question: "?", Type: "confirm"}); err != nil {
		t.Errorf("an ask called again for its answer: %v; want it never refused", err)
	}
	if ps := askPlugins(NewAskUserTool()); len(ps) != 2 {
		t.Errorf("plugins = %d; want ask-user and request-validation", len(ps))
	}
}
