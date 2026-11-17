// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerLoop struct{}

func (handlerLoop) Kind() NodeKind { return NodeLoop }

// loopPhase tracks the current iteration of a finite `loop` node. Stored
// in frame.Local under key "iteration" so the JSON checkpoint shape is
// unchanged.
type loopPhase struct {
	Iteration int
}

func loadLoopPhase(f *ActivationFrame) loopPhase {
	return loopPhase{Iteration: getInt(f.Local, "iteration")}
}

func (p loopPhase) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["iteration"] = p.Iteration
}

func (handlerLoop) Step(
	_ context.Context, _ *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	loop := node.Loop
	phase := loadLoopPhase(frame)

	return func(_ context.Context) (*machine.StepResult, error) {
		if phase.Iteration >= loop.Times {
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}
		phase.Iteration++
		phase.save(frame)
		ws.PushFrame(&loop.Body)
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerLoop{}) }
