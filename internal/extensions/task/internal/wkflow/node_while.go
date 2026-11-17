// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerWhile struct{}

func (handlerWhile) Kind() NodeKind { return NodeWhile }

// whilePhase tracks the current iteration of a `while` node. Stored in
// frame.Local under key "iteration" so the JSON checkpoint shape is
// unchanged. Shares the same key as loopPhase by intent — both kinds use
// "iteration" semantics and the discriminator is `frame.Kind`.
type whilePhase struct {
	Iteration int
}

func loadWhilePhase(f *ActivationFrame) whilePhase {
	return whilePhase{Iteration: getInt(f.Local, "iteration")}
}

func (p whilePhase) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["iteration"] = p.Iteration
}

func (handlerWhile) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	wh := node.While
	phase := loadWhilePhase(frame)

	return func(_ context.Context) (*machine.StepResult, error) {
		if phase.Iteration >= wh.MaxIterations {
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}
		scope := m.buildScope(ws, nil)
		condTrue, err := m.exprCache.EvalBool(wh.Condition, scope.AsMap())
		if err != nil {
			return nil, fmt.Errorf("while condition: %w", err)
		}
		if !condTrue {
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}
		phase.Iteration++
		phase.save(frame)
		ws.PushFrame(&wh.Body)
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerWhile{}) }
