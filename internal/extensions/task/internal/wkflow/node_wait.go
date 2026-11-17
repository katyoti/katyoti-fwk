// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"
	"time"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerWait struct{}

func (handlerWait) Kind() NodeKind { return NodeWait }

// waitSpec is the wait a wait step arms: its event (a child's — child.<x>.done
// for a child of this workflow — is its run's end, rechecked: settled from how
// it ended), or a timer. Its timeout is what is left of the step's absolute
// deadline, set in its frame when the step began: a recheck, a restore or a
// bus reconnection arms only the rest, never the full length again. A frame
// saved before it kept a deadline — or no frame, the step's first arm —
// arms the full length.
func waitSpec(ws *WorkflowState, w *WaitNode, frame *ActivationFrame) *machine.WaitSpec {
	var spec *machine.WaitSpec
	switch {
	case w.ForEvent != "":
		topic, recheck := w.ForEvent, false
		if name, _, ok := machine.ParseChildTopic(w.ForEvent); ok {
			if h, known := ws.Children[name]; known {
				topic, recheck = machine.RunEndedTopic(h.RunID), true
			}
		}
		spec = &machine.WaitSpec{Conditions: []machine.WaitCondition{{Topic: topic, Label: "event"}}, Mode: machine.WaitAny, RecheckOnResume: recheck}
	case w.DurationMs > 0:
		// Duration-only wait: a timer — a machine wait only its timeout
		// ends. The run holds no slot meanwhile, and its deadline is saved
		// with it (a restart keeps the remaining time).
		spec = &machine.WaitSpec{Mode: machine.WaitAny}
	default:
		return nil
	}
	if d := waitTimeout(w); d > 0 {
		spec.Timeout, spec.TimeoutLabel = d, "timeout"
		if left, _, ok := deadlineLeft(frame); ok {
			spec.Timeout = left
		}
	}
	return spec
}

// waitTimeout is the full length of the wait's timeout; zero: none.
func waitTimeout(w *WaitNode) time.Duration {
	switch {
	case w.ForEvent != "":
		return time.Duration(w.TimeoutMs) * time.Millisecond
	case w.DurationMs > 0:
		return time.Duration(w.DurationMs) * time.Millisecond
	}
	return 0
}

func (handlerWait) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, event machine.Event,
) (machine.ActionFunc, error) {
	w := node.Wait

	switch frame.Phase {
	case "": // Set up the wait.
		return func(ctx context.Context) (*machine.StepResult, error) {
			// Armed at its full length; its deadline kept for every re-arm.
			if d := waitTimeout(w); d > 0 {
				setDeadline(frame, d, "timeout")
			}
			spec := waitSpec(ws, w, nil)
			if spec == nil {
				ws.PopFrame()
				return &machine.StepResult{NextState: "running", Data: ws}, nil
			}
			frame.Phase = "waiting"
			return &machine.StepResult{NextState: "running", Data: ws, Wait: spec}, nil
		}, nil

	case "waiting": // Resumed: its event, its timeout — or a recheck, a restart.
		return func(_ context.Context) (*machine.StepResult, error) {
			if event.Label == "timeout" {
				ws.PopFrame()
				return &machine.StepResult{NextState: "running", Data: ws}, nil
			}
			// A child of this workflow: settled from how it ended.
			if name, action, ok := machine.ParseChildTopic(w.ForEvent); ok {
				if h, known := ws.Children[name]; known && m.spawner != nil {
					outcome, ended := m.spawner.Outcome(h.RunID)
					if action == "started" { // it was started before this step
						ws.PopFrame()
						return &machine.StepResult{NextState: "running", Data: ws}, nil
					}
					if !ended {
						return &machine.StepResult{NextState: "running", Data: ws, Wait: waitSpec(ws, w, frame)}, nil
					}
					done := outcome.Status == machine.ChildDone
					if (action == "done") != done {
						// It ended the other way: what this step waits for will
						// never come.
						return m.handleNodeError(ws, frame, node, fmt.Sprintf("wait: %s ended %s (%s) — it will never be %s", name, outcome.Status, outcome.Error, action))
					}
					ws.PopFrame()
					return &machine.StepResult{NextState: "running", Data: ws}, nil
				}
			}
			if event.Label != "event" { // a recheck, a restart: not its event — it waits on
				return &machine.StepResult{NextState: "running", Data: ws, Wait: waitSpec(ws, w, frame)}, nil
			}
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}, nil
	}

	return nil, fmt.Errorf("wait: unexpected phase %q", frame.Phase)
}

func init() { registerNodeHandler(handlerWait{}) }
