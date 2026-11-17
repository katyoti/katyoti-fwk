// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerSequence struct{}

func (handlerSequence) Kind() NodeKind { return NodeSequence }

// sequencePhase is the per-frame state for an in-flight sequence: the
// next-step index. Stored in frame.Local under key "index" so the wire
// format (JSON checkpoint) stays unchanged; the typed struct exists only
// to keep handler call sites typo-safe (compile error on misspelled field
// vs silent zero-value from a misspelled map key).
type sequencePhase struct {
	Index int
}

func loadSequencePhase(f *ActivationFrame) sequencePhase {
	return sequencePhase{Index: getInt(f.Local, "index")}
}

func (p sequencePhase) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["index"] = p.Index
}

func (handlerSequence) Step(
	_ context.Context, _ *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	seq := node.Sequence
	phase := loadSequencePhase(frame)

	return func(_ context.Context) (*machine.StepResult, error) {
		if phase.Index >= len(seq.Steps) {
			// All steps done — pop this sequence frame.
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}

		// Advance index for next visit.
		phase.Index++
		phase.save(frame)

		// Push the next step onto the activation stack.
		ws.PushFrame(&seq.Steps[phase.Index-1])

		// Continue immediately — the new frame will produce its own result.
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerSequence{}) }
