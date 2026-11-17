// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerIf struct{}

func (handlerIf) Kind() NodeKind { return NodeIf }

func (handlerIf) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	_ *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	ifn := node.If
	return func(_ context.Context) (*machine.StepResult, error) {
		scope := m.buildScope(ws, nil)
		condTrue, err := m.exprCache.EvalBool(ifn.Condition, scope.AsMap())
		if err != nil {
			return nil, fmt.Errorf("if condition: %w", err)
		}
		// Pop the if-frame.
		ws.PopFrame()
		// Push chosen branch.
		if condTrue {
			ws.PushFrame(&ifn.Then)
		} else if ifn.Else != nil {
			ws.PushFrame(ifn.Else)
		}
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerIf{}) }
