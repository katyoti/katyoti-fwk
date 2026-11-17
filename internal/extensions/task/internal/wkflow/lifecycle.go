// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// wrapLifecycleEvents wraps an ActionFunc to emit workflow lifecycle events:
//   - workflow.<runID>.started on the first step (exactly once)
//   - workflow.<runID>.done on successful terminal (NextState=="done")
//   - workflow.<runID>.failed on failure terminal (NextState=="failed")
//
// Lifecycle wrapping is orthogonal to node-kind dispatch, so it lives here
// rather than in node_handler.go or any node_<kind>.go file.
func (m *workflowMachine) wrapLifecycleEvents(runID string, ws *WorkflowState, action machine.ActionFunc, inErr error) (machine.ActionFunc, error) {
	if inErr != nil {
		return nil, inErr
	}
	if action == nil {
		return nil, nil
	}
	return func(ctx context.Context) (*machine.StepResult, error) {
		result, err := action(ctx)
		if err != nil || result == nil {
			return result, err
		}

		// Emit workflow.<runID>.started exactly once, on the first step.
		if !ws.StartedEmitted && runID != "" {
			ws.StartedEmitted = true
			result.Emit = append(result.Emit, machine.Event{
				Type: machine.WorkflowStartedTopic(runID),
				Payload: &machine.WorkflowStartedEvent{
					RunID:  runID,
					Name:   m.root.ID,
					Inputs: m.input,
				},
			})
		}

		// Emit workflow.<runID>.done on successful terminal.
		if result.Output != nil && result.NextState == "done" && runID != "" {
			result.Emit = append(result.Emit, machine.Event{
				Type: machine.WorkflowDoneTopic(runID),
				Payload: &machine.WorkflowDoneEvent{
					RunID:  runID,
					Output: result.Output,
				},
			})
		}

		// Emit workflow.<runID>.failed on failure terminal.
		//
		// Source priority (D1 quirk #2):
		//   1. result.Failure.Reason (typed companion — preferred)
		//   2. result.Output["error"] (legacy map shape — fallback)
		//   3. hardcoded "workflow failed" (last-ditch)
		// Handler/error paths populate (1)+(2) together via failureResult;
		// this ordering means a future caller that drops the map shape (or
		// a typed-only handler) still produces a useful Error string.
		if result.NextState == "failed" && runID != "" {
			errMsg := "workflow failed"
			if result.Failure != nil && result.Failure.Reason != "" {
				errMsg = result.Failure.Reason
			} else if outMap, ok := result.Output.(map[string]any); ok {
				if e, ok := outMap["error"].(string); ok {
					errMsg = e
				}
			}
			result.Emit = append(result.Emit, machine.Event{
				Type: machine.WorkflowFailedTopic(runID),
				Payload: &machine.WorkflowFailedEvent{
					RunID: runID,
					Error: errMsg,
				},
			})
		}

		return result, nil
	}, nil
}

// terminate produces the "workflow complete" StepResult when the activation
// stack is empty. Returns workflow Results as the output.
func (m *workflowMachine) terminate(ws *WorkflowState) (machine.ActionFunc, error) {
	return func(_ context.Context) (*machine.StepResult, error) {
		return &machine.StepResult{
			NextState: "done",
			Data:      ws,
			Output:    ws.Results,
		}, nil
	}, nil
}

// buildScope creates the expression-evaluation scope for the current
// workflow state. The loop argument is optional — when nil, buildScope
// looks for the innermost active foreach frame's LoopScope so the body's
// expressions can resolve `loop.index` / `loop.item` without callers
// having to thread the iteration context explicitly.
//
// Closes D1 quirk #3 (audit finding F-WF-4): pre-fix, foreach wrote
// `_loop_index` / `_loop_item` keys into `ws.Data` because the body
// frame's buildScope() was always called with `loop=nil`. Those keys
// leaked into the user expression scope as `state._loop_*`. With the
// activation-stack walk, the LoopScope is sourced cleanly without
// polluting `ws.Data`.
func (m *workflowMachine) buildScope(ws *WorkflowState, loop *LoopScope) *Scope {
	if loop == nil {
		loop = activeLoopScope(ws)
	}
	return BuildScope(ws, m.input, loop)
}

// activeLoopScope walks the activation stack top-down looking for the
// innermost foreach frame and returns its current iteration state. Returns
// nil if no foreach is active (the common case outside loop bodies).
//
// The foreach frame stashes its LoopScope in `frame.Local["loop_scope"]` at
// each push of the body — see handlerForeach.
func activeLoopScope(ws *WorkflowState) *LoopScope {
	for i := len(ws.Active) - 1; i >= 0; i-- {
		f := &ws.Active[i]
		if f.Kind != NodeForeach {
			continue
		}
		ls, ok := f.Local["loop_scope"].(*LoopScope)
		if !ok || ls == nil {
			continue
		}
		return ls
	}
	return nil
}
