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

// An agent restarted after a crash in the middle of its tool calls does not
// run them again — they may or may not have taken effect: each call not done
// yet is answered "interrupted by a restart", and the model, on its next
// turn, decides what to redo. The calls done before the crash keep their
// results.
func TestARestartedAgentAnswersItsInFlightCallsInterrupted(t *testing.T) {
	calls := 0
	plugin := &nonWrapperPlugin{
		name: "p",
		defs: []api.ToolDef{{Name: "write"}, {Name: "delegate"}},
		handler: func(context.Context, string, json.RawMessage) (*api.ToolCallResult, error) {
			calls++
			return &api.ToolCallResult{Data: "done"}, nil
		},
	}
	set := NewPluginSet()
	set.Add(plugin)
	m := &AgentLoopMachine{plugins: set, hooks: &HookChain{}, loop: NewLoopContext()}
	if !machine.RestartsAfterCrash(m) {
		t.Fatal("the agent loop does not restart after a crash")
	}
	as := &agentState{StartedAt: time.Now(), Turn: 2, LastResponse: &llm.ContentResponse{Choices: []*llm.ContentChoice{{
		ToolCalls: []llm.ToolCall{toolCall("1", "write"), toolCall("2", "delegate")},
	}}}}
	action, err := m.Step(context.Background(), &machine.State{Name: "tool_dispatch", Data: as}, machine.Event{Type: machine.RestartedEvent})
	if err != nil {
		t.Fatal(err)
	}
	res, err := action(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("%d tool calls run again after the restart; want none", calls)
	}
	if res.NextState != "thinking" || res.Wait != nil {
		t.Errorf("after the restart: %s, want the next turn", res.NextState)
	}
	conv, _ := json.Marshal(as.Messages)
	if n := strings.Count(string(conv), "interrupted by a restart"); n != 2 {
		t.Errorf("%d calls answered interrupted, want both: %s", n, conv)
	}
	if as.LastResponse != nil || as.Batch != nil {
		t.Error("the interrupted batch is still pending")
	}
}
