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

// awaitingMachine is a loop whose one tool call, `ask`, is handled by h.
func awaitingMachine(h func(ctx context.Context, in json.RawMessage) (*api.ToolCallResult, error), args string, hooks *HookChain) (*AgentLoopMachine, *agentState) {
	plugin := &nonWrapperPlugin{
		name: "p",
		defs: []api.ToolDef{{Name: "ask"}},
		handler: func(ctx context.Context, _ string, in json.RawMessage) (*api.ToolCallResult, error) {
			return h(ctx, in)
		},
	}
	set := NewPluginSet()
	set.Add(plugin)
	m := &AgentLoopMachine{plugins: set, hooks: hooks, loop: NewLoopContext()}
	as := &agentState{StartedAt: time.Now(), LastResponse: &llm.ContentResponse{Choices: []*llm.ContentChoice{{
		ToolCalls: []llm.ToolCall{{ID: "1", Type: "function", FunctionCall: &llm.FunctionCall{Name: "ask", Arguments: args}}},
	}}}}
	return m, as
}

func dispatchOnce(t *testing.T, m *AgentLoopMachine, as *agentState) *machine.StepResult {
	t.Helper()
	action, err := m.stepToolDispatch(context.Background(), as, false)
	if err != nil {
		t.Fatal(err)
	}
	res, err := action(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// restored is as saved by the run store and read back after a restart.
func restored(t *testing.T, m *AgentLoopMachine, as *agentState) *agentState {
	t.Helper()
	b, err := json.Marshal(as)
	if err != nil {
		t.Fatal(err)
	}
	var data any
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	got, err := m.ensureTyped(&machine.State{Data: data})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A tool that awaits with no Resume of its own is still called AGAIN, not
// anew: its hooks do not run a second time (they ran on the first call), and
// it knows it is checking an answer, not asking a new question.
func TestAnAwaitWithoutResumeIsCalledAgainNotAnew(t *testing.T) {
	hooksRun, calls, firsts := 0, 0, 0
	hooks := &HookChain{OnPreToolExec: []func(api.LoopContext, string, map[string]any) (*api.ToolDecision, error){
		func(api.LoopContext, string, map[string]any) (*api.ToolDecision, error) { hooksRun++; return nil, nil },
	}}
	m, as := awaitingMachine(func(ctx context.Context, _ json.RawMessage) (*api.ToolCallResult, error) {
		calls++
		if _, again := api.AwaitResumeFrom(ctx); !again {
			firsts++
			return nil, &api.ErrAwait{EventURI: "cwm:!workspace/object/q-9/updated"}
		}
		return &api.ToolCallResult{Data: "answered"}, nil
	}, "{}", hooks)

	if res := dispatchOnce(t, m, as); res.NextState != "awaiting" {
		t.Fatalf("first call: %s, want awaiting", res.NextState)
	}
	as = restored(t, m, as)
	if res := dispatchOnce(t, m, as); res.NextState != "thinking" {
		t.Fatalf("called again: %s, want the next turn", res.NextState)
	}
	if calls != 2 || firsts != 1 || hooksRun != 1 {
		t.Errorf("calls=%d firsts=%d hooks=%d; want the tool called twice, as a first call once, its hooks once", calls, firsts, hooksRun)
	}
}

// The awaiting call is called again with the very input it was first called
// with — read back exactly after a restart, even a number JSON's float64
// cannot hold.
func TestAnAwaitingCallKeepsItsInputExactly(t *testing.T) {
	const args = `{"big":9007199254740993,"text":"a"}`
	var inputs []string
	m, as := awaitingMachine(func(ctx context.Context, in json.RawMessage) (*api.ToolCallResult, error) {
		inputs = append(inputs, string(in))
		if _, again := api.AwaitResumeFrom(ctx); !again {
			return nil, &api.ErrAwait{EventURI: "cwm:!workspace/object/q-8/updated", Resume: map[string]any{"request": "q-8"}}
		}
		return &api.ToolCallResult{Data: "ok"}, nil
	}, args, &HookChain{})

	dispatchOnce(t, m, as)
	as = restored(t, m, as)
	dispatchOnce(t, m, as)
	if len(inputs) != 2 || !strings.Contains(inputs[1], "9007199254740993") {
		t.Errorf("inputs = %v; want the second call to get the first call's input exactly", inputs)
	}
}

// A tool may ask to be called again every so often while it awaits (an
// answer that could come with no event, a request that could disappear): the
// wait then times out at that period and the call checks again.
func TestAnAwaitCanBeRecheckedPeriodically(t *testing.T) {
	m, as := awaitingMachine(func(context.Context, json.RawMessage) (*api.ToolCallResult, error) {
		return nil, &api.ErrAwait{EventURI: "cwm:!workspace/object/q-7/updated", RecheckEvery: time.Hour}
	}, "{}", &HookChain{})
	res := dispatchOnce(t, m, as)
	if res.Wait == nil || res.Wait.Timeout != time.Hour {
		t.Fatalf("wait = %+v; want it to time out after the tool's recheck period", res.Wait)
	}
}

// A saved state that cannot be read back fails the run with the reason — it
// is never half-read into a run that goes on with a broken conversation.
func TestAStateThatCannotBeReadBackFailsTheRun(t *testing.T) {
	m := &AgentLoopMachine{plugins: NewPluginSet(), hooks: &HookChain{}, loop: NewLoopContext()}
	data := map[string]any{
		"turn":     3,
		"messages": []any{map[string]any{"role": "human", "parts": []any{map[string]any{"type": "hologram"}}}},
	}
	_, err := m.Step(context.Background(), &machine.State{Name: "thinking", Data: data}, machine.Event{Type: machine.TickEvent})
	if err == nil || !strings.Contains(err.Error(), "hologram") {
		t.Fatalf("Step on an unreadable state: %v; want an error that says why", err)
	}
}

// A tool may await an in-process topic (a child run's end) instead of an
// event of another process: the machine waits on that topic as is.
func TestAnAwaitOnAnInProcessTopic(t *testing.T) {
	m, as := awaitingMachine(func(context.Context, json.RawMessage) (*api.ToolCallResult, error) {
		return nil, &api.ErrAwait{Topic: "run.child-1.ended"}
	}, "{}", &HookChain{})
	res := dispatchOnce(t, m, as)
	if res.Wait == nil || len(res.Wait.Conditions) != 1 {
		t.Fatalf("wait = %+v", res.Wait)
	}
	if c := res.Wait.Conditions[0]; c.Topic != "run.child-1.ended" || c.EventURI != "" {
		t.Errorf("condition = %+v; want the topic itself, no external event", c)
	}
}
