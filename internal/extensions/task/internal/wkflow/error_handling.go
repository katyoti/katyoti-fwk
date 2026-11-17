// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"fmt"
	"math/rand"
	"time"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// handleNodeError checks the node's OnError strategy and returns the
// appropriate StepResult. If no OnError is set, the error propagates and
// the workflow fails.
//
// Called by handlers that can observe a child failure (today: handlerSpawn
// in the "failed" event branch).
//
// Per D1 quirk #2, every failure StepResult populates BOTH the legacy
// `Output.(map)["error"]` shape (backward compat) AND the typed
// `Failure` companion (structured access for new consumers). The Failure
// carries the originating child name when available + the retry count.
func (m *workflowMachine) handleNodeError(ws *WorkflowState, frame *ActivationFrame, node *Node, errMsg string) (*machine.StepResult, error) {
	if node.OnError == nil || node.OnError.Strategy == OnErrorFail {
		// Default: propagate failure.
		return failureResult(ws, errMsg, childNameFromSpawn(node), 0), nil
	}

	switch node.OnError.Strategy {
	case OnErrorContinue:
		// Skip: pop frame, store nil result, continue.
		ws.PopFrame()
		return &machine.StepResult{NextState: "running", Data: ws}, nil

	case OnErrorRetry:
		retry := loadRetryCounter(frame)
		max := node.OnError.MaxRetries
		if max <= 0 {
			max = 1
		}
		if retry.Count < max {
			retry.Count++
			retry.save(frame)
			if delay := retryBackoff(node.OnError, retry.Count); delay > 0 {
				// Wait before the retry: a timer, then the step runs again.
				frame.Phase = retryWaitPhase
				return &machine.StepResult{NextState: "running", Data: ws, Wait: &machine.WaitSpec{
					Mode: machine.WaitAny, Timeout: delay, TimeoutLabel: "retry"}}, nil
			}
			// Reset phase to re-execute the spawn.
			frame.Phase = ""
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}
		// Exhausted retries — propagate.
		return failureResult(ws, fmt.Sprintf("%s (after %d retries)", errMsg, max), childNameFromSpawn(node), max), nil

	case OnErrorFallback:
		if node.OnError.Fallback == nil {
			return failureResult(ws, errMsg+" (no fallback node)", childNameFromSpawn(node), 0), nil
		}
		// Pop the failed frame, push the fallback node.
		ws.PopFrame()
		ws.PushFrame(node.OnError.Fallback)
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}

	// Unknown strategy — propagate.
	return failureResult(ws, errMsg, childNameFromSpawn(node), 0), nil
}

// failureResult builds a structured failure StepResult that satisfies both
// the legacy `Output.(map)["error"]` shape AND the typed `Failure`
// companion (D1 quirk #2). Centralises the convention so handler/error
// paths can't drift.
func failureResult(ws *WorkflowState, reason, child string, retries int) *machine.StepResult {
	output := map[string]any{"error": reason, "results": ws.Results}
	if child != "" {
		output["failed"] = child
	}
	return &machine.StepResult{
		NextState: "failed",
		Data:      ws,
		Output:    output,
		Failure: &machine.Failure{
			Reason:  reason,
			Child:   child,
			Retries: retries,
		},
	}
}

// childNameFromSpawn returns the child/result role name for a delegate or
// call node, or "" for other kinds. Used to populate `Failure.Child` from
// handleNodeError without coupling the helper to the call-site type.
func childNameFromSpawn(node *Node) string {
	switch {
	case node == nil:
		return ""
	case node.Kind == NodeDelegate && node.Delegate != nil:
		return node.Delegate.ChildName
	case node.Kind == NodeCall && node.Call != nil:
		return node.Call.Name
	}
	return ""
}

// retryWaitPhase is a frame waiting before its retry: woken, it runs again
// from its start.
const retryWaitPhase = "retry-wait"

// retryBackoff is the wait before retry n (1-based): BackoffMs doubled each
// retry, at most MaxBackoffMs, ±20 %; zero when no backoff is declared.
func retryBackoff(def *OnErrorDef, n int) time.Duration {
	if def == nil || def.BackoffMs <= 0 {
		return 0
	}
	d := time.Duration(def.BackoffMs) * time.Millisecond
	for i := 1; i < n && d < time.Hour; i++ {
		d *= 2
	}
	if def.MaxBackoffMs > 0 && d > time.Duration(def.MaxBackoffMs)*time.Millisecond {
		d = time.Duration(def.MaxBackoffMs) * time.Millisecond
	}
	jitter := float64(d) * 0.2 * (2*rand.Float64() - 1)
	return d + time.Duration(jitter)
}

// retryCounter tracks how many times the OnErrorRetry strategy has
// re-executed the current node. Shared across all node kinds (any frame
// whose node carries `OnError.Strategy: OnErrorRetry` may carry one).
// Stored in frame.Local under key "_retry_count" so the JSON checkpoint
// shape is unchanged.
type retryCounter struct {
	Count int
}

func loadRetryCounter(f *ActivationFrame) retryCounter {
	return retryCounter{Count: getInt(f.Local, "_retry_count")}
}

func (r retryCounter) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["_retry_count"] = r.Count
}
