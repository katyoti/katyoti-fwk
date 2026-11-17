// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerSwitch struct{}

func (handlerSwitch) Kind() NodeKind { return NodeSwitch }

func (handlerSwitch) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	_ *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	sw := node.Switch
	return func(_ context.Context) (*machine.StepResult, error) {
		scope := m.buildScope(ws, nil)
		result, err := m.exprCache.Eval(sw.On, scope.AsMap())
		if err != nil {
			return nil, fmt.Errorf("switch on: %w", err)
		}
		key := fmt.Sprintf("%v", result)
		ws.PopFrame()
		if branch, ok := sw.Cases[key]; ok {
			ws.PushFrame(&branch)
		} else if sw.Default != nil {
			ws.PushFrame(sw.Default)
		}
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerSwitch{}) }
