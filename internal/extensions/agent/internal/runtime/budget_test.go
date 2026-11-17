// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	llm "github.com/katyoti/katyoti-fwk/internal/models/api"
)

// A run whose model calls have cost its budget makes no further call: it
// ends "budget_exceeded" (max_budget_usd). Under its budget, it goes on.
func TestAnAgentStopsAtItsBudget(t *testing.T) {
	m := &AgentLoopMachine{plugins: NewPluginSet(), hooks: &HookChain{}, loop: NewLoopContext()}
	step := func(spent float64) *machine.StepResult {
		t.Helper()
		as := &agentState{StartedAt: time.Now(), MaxTurns: 50, MaxBudgetUSD: 0.50,
			TotalUsage: api.Usage{Cost: &llm.Cost{TotalUSD: spent}}}
		action, err := m.Step(context.Background(), &machine.State{Name: "thinking", Data: as}, machine.Event{Type: machine.TickEvent})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = recover() }() // under budget, the step goes on to call a model this test has none of
		res, _ := action(context.Background())
		return res
	}
	res := step(0.51)
	if res == nil || res.Output == nil {
		t.Fatalf("over its budget the run did not end: %+v", res)
	}
	if r, ok := res.Output.(*api.RunResult); !ok || r.Status != api.RunBudgetExceeded {
		t.Fatalf("output %+v; want status budget_exceeded", res.Output)
	}
	if res := step(0.49); res != nil && res.Output != nil {
		if r, ok := res.Output.(*api.RunResult); ok && r.Status == api.RunBudgetExceeded {
			t.Fatal("under its budget the run was stopped for it")
		}
	}
}

// The budget comes from the run's request.
func TestAnAgentsBudgetComesFromItsRequest(t *testing.T) {
	m := &AgentLoopMachine{runRequest: &api.AgentRunRequest{MaxBudgetUSD: 1.25}}
	st := m.InitialState(machine.Event{})
	if as, ok := st.Data.(*agentState); !ok || as.MaxBudgetUSD != 1.25 {
		t.Fatalf("initial state %+v; want its budget 1.25", st.Data)
	}
}
