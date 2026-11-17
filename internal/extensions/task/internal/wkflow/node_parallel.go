// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerParallel struct{}

func (handlerParallel) Kind() NodeKind { return NodeParallel }

// parallelPhase tracks the in-flight progress of a parallel node: how many
// branches were spawned (pending) and how many have completed (done or
// collected-as-done in collect mode). Frame.Phase is the string state-
// machine label ("" / "awaiting"); these counters are the resume-side
// state. Stored in frame.Local under keys "pending"/"completed" so the
// JSON checkpoint shape is unchanged.
type parallelPhase struct {
	Pending   int
	Completed int
}

func loadParallelPhase(f *ActivationFrame) parallelPhase {
	return parallelPhase{
		Pending:   getInt(f.Local, "pending"),
		Completed: getInt(f.Local, "completed"),
	}
}

func (p parallelPhase) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["pending"] = p.Pending
	f.Local["completed"] = p.Completed
}

func (handlerParallel) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, event machine.Event,
) (machine.ActionFunc, error) {
	par := node.Parallel

	switch frame.Phase {
	case "": // First visit: spawn all branches.
		return func(ctx context.Context) (*machine.StepResult, error) {
			if m.spawner == nil {
				return nil, fmt.Errorf("parallel: no spawner available")
			}
			spawned := 0
			for i := range par.Branches {
				branch := &par.Branches[i]
				if branch.Kind != NodeDelegate || branch.Delegate == nil {
					// Non-delegate branches are not supported (yet) inside
					// parallel — skipping one silently would also corrupt the
					// pending count below and hang the node forever.
					return m.handleNodeError(ws, frame, node,
						fmt.Sprintf("parallel: branch %d (kind %s) is not a delegate — non-delegate branches are not supported", i, branch.Kind))
				}
				d := branch.Delegate
				if _, err := m.spawnChild(ctx, ws, frame, d); err != nil {
					return nil, fmt.Errorf("parallel spawn %s: %w", d.ChildName, err)
				}
				spawned++
			}
			if spawned == 0 {
				// Zero children means zero events: suspending here would
				// hang forever on an empty WaitSpec.
				return m.handleNodeError(ws, frame, node, "parallel: no spawnable branches")
			}
			frame.Phase = "awaiting"
			// Pending counts what was actually spawned — counting declared
			// branches would make Completed unreachable if any were skipped.
			parallelPhase{Pending: spawned, Completed: 0}.save(frame)
			return &machine.StepResult{NextState: "running", Data: ws, Wait: childWait(frame)}, nil
		}, nil

	case "awaiting": // Resumed on each child event.
		return func(ctx context.Context) (*machine.StepResult, error) {
			phase := loadParallelPhase(frame)

			// Extract child name from label (format: "done:childName" or "failed:childName")
			label := event.Label
			if len(label) > 5 && label[:5] == "done:" {
				childName := label[5:]
				var output any
				if evt, ok := event.Payload.(*machine.ChildDoneEvent); ok {
					output = evt.Output
				} else {
					output = event.Payload
				}
				ws.Results[childName] = output
				phase.Completed++
			} else if len(label) > 7 && label[:7] == "failed:" {
				failedName := label[7:]

				// Branch-level on_error takes precedence over the node's
				// on_branch_error accounting. Without this check the
				// failed: path goes straight to collect/fail_fast and a
				// branch's `on_error: fallback` never runs.
				if res, handled, err := m.handleParallelBranchFailure(ctx, ws, frame, par, &phase, failedName); handled {
					if err != nil {
						return nil, err
					}
					if res != nil {
						return res, nil
					}
					// handled in-place (continue / retry respawn) — fall
					// through to the completion check below.
				} else if par.OnBranchError == "fail_fast" {
					// Cancel its own still-running branches (matches race
					// semantics) — never another step's children.
					if m.spawner != nil {
						m.cancelOwnChildren(frame, failedName, "parallel branch failed (fail_fast)")
					}
					ws.PopFrame()
					return &machine.StepResult{
						NextState: "failed",
						Data:      ws,
						Output:    map[string]any{"error": "parallel branch failed", "failed": failedName, "results": ws.Results},
						// D1 quirk #2: typed companion to the Output map.
						Failure: &machine.Failure{
							Reason: "parallel branch failed",
							Child:  failedName,
						},
					}, nil
				} else {
					phase.Completed++ // collect mode: count as done
				}
			}

			phase.save(frame)
			if phase.Completed >= phase.Pending {
				ws.PopFrame()
				return &machine.StepResult{NextState: "running", Data: ws}, nil
			}
			// Still waiting for more children: the same wait goes on.
			return &machine.StepResult{NextState: "running", Data: ws, KeepWaiting: true}, nil
		}, nil
	}

	return nil, fmt.Errorf("parallel: unexpected phase %q", frame.Phase)
}

// findParallelBranch returns the branch node whose delegate child-name
// matches, or nil. Fallback children spawned during awaiting are NOT
// branches — a failing fallback falls through to on_branch_error accounting
// (single-level recovery by design).
func findParallelBranch(par *ParallelNode, name string) *Node {
	for i := range par.Branches {
		b := &par.Branches[i]
		if b.Kind == NodeDelegate && b.Delegate != nil && b.Delegate.ChildName == name {
			return b
		}
	}
	return nil
}

// handleParallelBranchFailure applies the FAILED branch's own on_error
// strategy. Returns handled=false when the branch has no on_error (or the
// strategy is fail/unknown) — the caller then applies on_branch_error.
// When handled: a nil StepResult means the failure was absorbed in-place
// (continue, or a retry/fallback child was respawned on EXISTING
// subscriptions); a non-nil StepResult re-suspends with NEW subscriptions
// (fallback under a different child name).
func (m *workflowMachine) handleParallelBranchFailure(
	ctx context.Context, ws *WorkflowState, frame *ActivationFrame,
	par *ParallelNode, phase *parallelPhase, failedName string,
) (*machine.StepResult, bool, error) {
	branch := findParallelBranch(par, failedName)
	if branch == nil || branch.OnError == nil {
		return nil, false, nil
	}

	switch branch.OnError.Strategy {
	case OnErrorContinue:
		// Absorb: the branch counts as completed with a nil result.
		phase.Completed++
		return nil, true, nil

	case OnErrorRetry:
		rkey := "retry_" + failedName
		count := getInt(frame.Local, rkey)
		maxRetries := branch.OnError.MaxRetries
		if maxRetries <= 0 {
			maxRetries = 1
		}
		if count >= maxRetries {
			// Exhausted — NOT handled; fall back to on_branch_error.
			return nil, false, nil
		}
		if frame.Local == nil {
			frame.Local = map[string]any{}
		}
		frame.Local[rkey] = count + 1
		// Respawn under the same name, as its next attempt: a new run, so
		// the step waits again on its children's ends.
		if _, err := m.spawnChild(ctx, ws, frame, branch.Delegate); err != nil {
			return nil, true, fmt.Errorf("parallel: retry %s: %w", failedName, err)
		}
		return &machine.StepResult{NextState: "running", Data: ws, Wait: childWait(frame)}, true, nil

	case OnErrorFallback:
		fb := branch.OnError.Fallback
		if fb == nil || fb.Kind != NodeDelegate || fb.Delegate == nil {
			// Malformed / non-delegate fallback — not handled here.
			return nil, false, nil
		}
		if _, err := m.spawnChild(ctx, ws, frame, fb.Delegate); err != nil {
			return nil, true, fmt.Errorf("parallel: fallback for %s: %w", failedName, err)
		}
		appendLocalString(frame, "fallback_names", fb.Delegate.ChildName)
		// The fallback runs under a new name: the step waits on every child
		// it has not settled — its still-running branches and the fallback.
		return &machine.StepResult{NextState: "running", Data: ws, Wait: childWait(frame)}, true, nil
	}

	return nil, false, nil
}

// appendLocalString appends a value to a []string stored in frame.Local
// under key, tolerating the []any shape a JSON checkpoint reload produces.
// Returns the updated slice.
func appendLocalString(frame *ActivationFrame, key, value string) []string {
	if frame.Local == nil {
		frame.Local = map[string]any{}
	}
	var out []string
	switch v := frame.Local[key].(type) {
	case []string:
		out = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	}
	out = append(out, value)
	frame.Local[key] = out
	return out
}

func init() { registerNodeHandler(handlerParallel{}) }
