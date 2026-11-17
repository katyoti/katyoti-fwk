// SPDX-License-Identifier: Apache-2.0

package ask_user

import (
	"context"
	"errors"
	"testing"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// fakeChannel records whether prompt_shown was already on the bus when
// Ask ran — pinning the "prompt reaches subscribers BEFORE the blocking
// ask" ordering contract — and returns a canned answer.
type fakeChannel struct {
	sawPromptBeforeAsk *bool
	promptSeen         *int
	answer             AskOutput
	err                error
}

func (f *fakeChannel) Ask(_ context.Context, _ AskInput) (AskOutput, error) {
	*f.sawPromptBeforeAsk = *f.promptSeen > 0
	return f.answer, f.err
}

func withTestBus(t *testing.T) (*engine.MemoryEventBus, func()) {
	t.Helper()
	bus := engine.NewMemoryEventBus()
	prev := busFromContext
	busFromContext = func(context.Context) engine.EventBus { return bus }
	return bus, func() { busFromContext = prev }
}

// TestAskWithEvents_EmitsPromptShownBeforeAskAndUserInputAfter pins the
// tool-owned conversation events: prompt_shown (with the question text
// from either field alias, the prompt type, options, channel, and the
// loop turn) is published before the channel blocks; user_input (with
// the typed answer) is published after it returns.
func TestAskWithEvents_EmitsPromptShownBeforeAskAndUserInputAfter(t *testing.T) {
	bus, restore := withTestBus(t)
	defer restore()

	var prompts []agentapi.PromptShownEvent
	var inputs []agentapi.UserInputEvent
	bus.Subscribe(agentapi.EventPromptShown, func(_ string, p any) {
		prompts = append(prompts, p.(agentapi.PromptShownEvent))
	})
	bus.Subscribe(agentapi.EventUserInput, func(_ string, p any) {
		inputs = append(inputs, p.(agentapi.UserInputEvent))
	})

	sawPromptBeforeAsk := false
	promptSeen := 0
	bus.Subscribe(agentapi.EventPromptShown, func(_ string, _ any) { promptSeen++ })

	ctx := agentapi.WithCurrentTurn(context.Background(), 7)
	ctx = agentapi.WithUserCommsInput(ctx, "direct")

	ch := &fakeChannel{
		sawPromptBeforeAsk: &sawPromptBeforeAsk,
		promptSeen:         &promptSeen,
		answer:             AskOutput{Answer: "yes please", Type: "confirm"},
	}
	out, err := askWithEvents(ctx, ch, AskInput{
		Prompt:  "proceed?", // the alias field — PromptText must resolve it
		Type:    "select",
		Options: []string{"a", "b"},
	})
	if err != nil {
		t.Fatalf("ask: %v", err)
	}
	if out.Answer != "yes please" {
		t.Fatalf("answer = %q", out.Answer)
	}

	if !sawPromptBeforeAsk {
		t.Error("prompt_shown was not on the bus before the channel's Ask ran")
	}
	if len(prompts) != 1 {
		t.Fatalf("prompt_shown events = %d, want 1", len(prompts))
	}
	p := prompts[0]
	if p.Question != "proceed?" || p.PromptType != "select" || p.Turn != 7 || p.Channel != "direct" {
		t.Errorf("prompt event = %+v", p)
	}
	if len(p.Options) != 2 {
		t.Errorf("options not carried: %+v", p.Options)
	}
	if p.Type != agentapi.EventPromptShown {
		t.Errorf("event base type = %q", p.Type)
	}

	if len(inputs) != 1 {
		t.Fatalf("user_input events = %d, want 1", len(inputs))
	}
	if inputs[0].Answer != "yes please" || inputs[0].Turn != 7 {
		t.Errorf("user_input event = %+v", inputs[0])
	}
}

// TestAskWithEvents_NoUserInputOnError: a failed ask (no channel, gate
// error, timeout) emits the prompt but never a phantom user_input.
func TestAskWithEvents_NoUserInputOnError(t *testing.T) {
	bus, restore := withTestBus(t)
	defer restore()

	var inputs int
	bus.Subscribe(agentapi.EventUserInput, func(_ string, _ any) { inputs++ })

	sawPrompt := false
	promptSeen := 0
	bus.Subscribe(agentapi.EventPromptShown, func(_ string, _ any) { promptSeen++ })

	ch := &fakeChannel{sawPromptBeforeAsk: &sawPrompt, promptSeen: &promptSeen, err: errors.New("gate down")}
	if _, err := askWithEvents(context.Background(), ch, AskInput{Question: "q"}); err == nil {
		t.Fatal("expected error")
	}
	if promptSeen != 1 {
		t.Errorf("prompt_shown = %d, want 1", promptSeen)
	}
	if inputs != 0 {
		t.Errorf("user_input = %d, want 0", inputs)
	}
}

// TestNewAskUserTool_DeclaresInteractive: the loop suppresses its generic
// tool_feedback echo for interactive tools — keyed off this attribute,
// never off the tool name.
func TestNewAskUserTool_DeclaresInteractive(t *testing.T) {
	def := NewAskUserTool().ToolDef()
	if !def.Interactive {
		t.Error("ask-user ToolDef must declare Interactive")
	}
}
