// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

func toolCall(id, name string) llm.ToolCall {
	return llm.ToolCall{ID: id, Type: "function", FunctionCall: &llm.FunctionCall{Name: name, Arguments: "{}"}}
}

// A tool whose answer comes later awaits instead of blocking: the batch stops
// on it — the calls before it done, the ones after not run — and the machine
// waits on the tool's event, ready to check again after a restart. Called
// again (the event, or the restart), the tool checks: still pending → the
// machine waits again; answered → the batch goes on from that call, and every
// result lands in the conversation in call order.
func TestABatchAwaitsAndGoesOnWhereItStopped(t *testing.T) {
	var calls []string
	answered := false
	plugin := &nonWrapperPlugin{
		name: "p",
		defs: []api.ToolDef{{Name: "first"}, {Name: "ask"}, {Name: "last"}},
		handler: func(ctx context.Context, name string, _ json.RawMessage) (*api.ToolCallResult, error) {
			calls = append(calls, name)
			if name != "ask" {
				return &api.ToolCallResult{Data: name + "-done"}, nil
			}
			if !api.MachineWaitsFrom(ctx) {
				t.Error("the loop does not tell its tools they may await")
			}
			resume, again := api.AwaitResumeFrom(ctx)
			if !again {
				return nil, &api.ErrAwait{EventURI: "cwm:!workspace/object/q-1/updated", Resume: map[string]any{"request": "q-1"}}
			}
			if resume["request"] != "q-1" {
				t.Errorf("called again with %v, want its own resume", resume)
			}
			if !answered {
				return nil, &api.ErrAwait{EventURI: "cwm:!workspace/object/q-1/updated", Resume: resume}
			}
			return &api.ToolCallResult{Data: "the-answer"}, nil
		},
	}
	set := NewPluginSet()
	set.Add(plugin)
	m := &AgentLoopMachine{plugins: set, hooks: &HookChain{}, loop: NewLoopContext()}
	as := &agentState{StartedAt: time.Now(), LastResponse: &llm.ContentResponse{Choices: []*llm.ContentChoice{{
		ToolCalls: []llm.ToolCall{toolCall("1", "first"), toolCall("2", "ask"), toolCall("3", "last")},
	}}}}
	ctx := context.Background()
	step := func() *machine.StepResult {
		t.Helper()
		action, err := m.stepToolDispatch(ctx, as, false)
		if err != nil {
			t.Fatal(err)
		}
		res, err := action(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	res := step()
	if res.NextState != "awaiting" || res.Wait == nil || !res.Wait.RecheckOnResume || len(res.Wait.Conditions) != 1 {
		t.Fatalf("after the ask: %+v, want awaiting on one condition, rechecked on resume", res)
	}
	if c := res.Wait.Conditions[0]; c.EventURI != "cwm:!workspace/object/q-1/updated" || c.Topic != machine.ExternalEventTopic(c.EventURI) {
		t.Errorf("condition = %+v, want the tool's event and its in-process topic", c)
	}
	if strings.Join(calls, ",") != "first,ask" {
		t.Fatalf("calls = %v, want first then ask; last not yet", calls)
	}

	// Saved and read back, as across a restart.
	b, err := json.Marshal(as)
	if err != nil {
		t.Fatal(err)
	}
	var restored any
	if err := json.Unmarshal(b, &restored); err != nil {
		t.Fatal(err)
	}
	if as, err = m.ensureTyped(&machine.State{Data: restored}); err != nil {
		t.Fatal(err)
	}

	if res = step(); res.NextState != "awaiting" {
		t.Fatalf("still pending: %s, want awaiting again", res.NextState)
	}
	answered = true
	if res = step(); res.NextState != "thinking" || res.Wait != nil {
		t.Fatalf("answered: %s, want the next turn", res.NextState)
	}
	if strings.Join(calls, ",") != "first,ask,ask,ask,last" {
		t.Errorf("calls = %v, want first once, ask until answered, then last once", calls)
	}
	if as.Batch != nil {
		t.Error("the batch is done: no batch kept")
	}
	conv, _ := json.Marshal(as.Messages)
	s := string(conv)
	i, j, k := strings.Index(s, "first-done"), strings.Index(s, "the-answer"), strings.Index(s, "last-done")
	if i < 0 || j < 0 || k < 0 || !(i < j && j < k) {
		t.Errorf("tool results not all in call order in the conversation: %s", s)
	}
}

// Restarted after a crash with the event that woke it (Replayed), an awaiting
// agent checks its awaited call again — and answers the calls after it
// interrupted, never runs them again: the woken step may have run them before
// the process died.
func TestAReplayedWakeDoesNotRunTheRestOfTheBatchAgain(t *testing.T) {
	var calls []string
	plugin := &nonWrapperPlugin{
		name: "p",
		defs: []api.ToolDef{{Name: "ask"}, {Name: "send"}},
		handler: func(ctx context.Context, name string, _ json.RawMessage) (*api.ToolCallResult, error) {
			calls = append(calls, name)
			if _, again := api.AwaitResumeFrom(ctx); name == "ask" && !again {
				return nil, &api.ErrAwait{EventURI: "cwm:!workspace/object/q-1/updated", Resume: map[string]any{"request": "q-1"}}
			}
			return &api.ToolCallResult{Data: name + "-done"}, nil
		},
	}
	set := NewPluginSet()
	set.Add(plugin)
	m := &AgentLoopMachine{plugins: set, hooks: &HookChain{}, loop: NewLoopContext()}
	as := &agentState{StartedAt: time.Now(), LastResponse: &llm.ContentResponse{Choices: []*llm.ContentChoice{{
		ToolCalls: []llm.ToolCall{toolCall("1", "ask"), toolCall("2", "send")},
	}}}}
	ctx := context.Background()
	action, err := m.stepToolDispatch(ctx, as, false)
	if err != nil {
		t.Fatal(err)
	}
	if res, err := action(ctx); err != nil || res.NextState != "awaiting" {
		t.Fatalf("after the ask: %+v %v, want awaiting", res, err)
	}

	// The process died during the woken step; the restart steps with its event, replayed.
	woken := machine.Event{Type: "external", Label: "answer", Replayed: true}
	action, err = m.Step(ctx, &machine.State{Name: "awaiting", Data: as}, woken)
	if err != nil {
		t.Fatal(err)
	}
	res, err := action(ctx)
	if err != nil || res.NextState != "thinking" {
		t.Fatalf("replayed: %+v %v, want the next turn", res, err)
	}
	if strings.Join(calls, ",") != "ask,ask" {
		t.Errorf("calls = %v, want ask checked again and send not run again", calls)
	}
	conv, _ := json.Marshal(as.Messages)
	if !strings.Contains(string(conv), "interrupted by a restart") {
		t.Errorf("send not answered interrupted: %s", conv)
	}
}
