// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"context"
	"fmt"
)

// OneShotMachine wraps a single function as a trivial Machine.
// One state ("execute") → action → Done.
//
// Use for strategies that don't need multi-step state management:
// program (exec command), template (render), human-gate (prompt).
//
// The function receives ctx (for UseService, cancellation) and returns
// the output (any JSON-serializable value) or an error.
func OneShotMachine(name string, fn func(ctx context.Context) (any, error)) Machine {
	return &oneShotMachine{name: name, fn: fn}
}

type oneShotMachine struct {
	name string
	fn   func(ctx context.Context) (any, error)
}

func (m *oneShotMachine) Name() string { return m.name }

func (m *oneShotMachine) InitialState(_ Event) State {
	return State{Name: "execute"}
}

func (m *oneShotMachine) Step(_ context.Context, state *State, _ Event) (ActionFunc, error) {
	if state.Name != "execute" {
		return nil, fmt.Errorf("oneshot(%s): unexpected state %q", m.name, state.Name)
	}
	fn := m.fn
	return func(ctx context.Context) (*StepResult, error) {
		output, err := fn(ctx)
		if err != nil {
			return nil, err
		}
		return &StepResult{
			NextState: "done",
			Output:    output,
		}, nil
	}, nil
}
