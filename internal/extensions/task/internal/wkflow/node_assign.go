// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerAssign struct{}

func (handlerAssign) Kind() NodeKind { return NodeAssign }

func (handlerAssign) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	_ *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	asgn := node.Assign
	return func(_ context.Context) (*machine.StepResult, error) {
		val := asgn.Value
		if exprStr, ok := val.(string); ok {
			if inner, wrapped := el.UnwrapExpression(exprStr); wrapped {
				// ${expr}: parse failure becomes an explicit workflow error.
				result, err := m.exprCache.Eval(inner, m.buildScope(ws, nil).AsMap())
				if err != nil {
					return nil, fmt.Errorf("assign %q: %w", asgn.Target, err)
				}
				val = result
			}
			// Bare strings stay literal — wrap in ${...} to opt into evaluation.
		}
		if ws.Data == nil {
			ws.Data = make(map[string]any)
		}
		ws.Data[asgn.Target] = val
		ws.PopFrame()
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerAssign{}) }
