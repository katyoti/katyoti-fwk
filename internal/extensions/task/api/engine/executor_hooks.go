// SPDX-License-Identifier: Apache-2.0

package engine

import "context"

// D4 — Executor hook surface.
//
// The executor's react-commit-dispatch loop was a closed shop pre-D4:
// observers (tracing, metrics, policy injection) had no integration
// point. Hooks open the loop without leaking the executor's internals:
// each callback gets a typed view of the state/event/result; none can
// reach the inbox or store directly.
//
// Ordering — hooks fire in registration order at four points:
//
//	OnPreStep  → before machine.Step (may short-circuit by returning err)
//	OnPostStep → after action() returns (always called; observes err too)
//	OnSuspend  → when result.Wait is non-nil, just before Channel.Wait
//	OnResume   → when an event re-enters the loop after a Wait
//
// Error semantics — OnPreStep error short-circuits THIS step (result is
// Failed, OnPostStep still fires with the error). All other hook errors
// are logged via slog and the loop continues — observability hooks must
// not poison the loop. Use OnPreStep for policy enforcement; everything
// else for pure observation.
//
// Performance — hooks are called inline. Empty-hook-chain overhead is
// one pointer-load per step (the slice header). Real hooks should keep
// their work bounded; long-running work belongs in a goroutine the hook
// kicks off, not the hook body.

// ExecutorHook is the integration point. All four methods are required
// (use the zero-value `NoopHook` to embed when you only want a subset).
type ExecutorHook interface {
	// OnPreStep fires before machine.Step. Return non-nil to abort this
	// step — the executor treats the return as if Step itself errored
	// (result = Failed; OnPostStep still fires with the err).
	OnPreStep(ctx context.Context, state *State, event Event) error

	// OnPostStep fires after the action func returns (or after OnPreStep
	// aborted). Observes the StepResult and any error. Errors from this
	// hook are logged and ignored — the loop continues.
	OnPostStep(ctx context.Context, state *State, result *StepResult, err error)

	// OnSuspend fires when a step produces a WaitSpec, immediately before
	// the executor calls Channel.Wait. Observation only; the spec can't
	// be modified.
	OnSuspend(ctx context.Context, runID string, spec *WaitSpec)

	// OnResume fires when a queued event unblocks an Inbox.Get and the
	// executor is about to feed it to machine.Step again. Observation
	// only.
	OnResume(ctx context.Context, runID string, event Event)
}

// NoopHook is an embeddable zero-implementation. Useful when an
// observer only cares about a subset of the four callbacks:
//
//	type MyTracer struct {
//		engine.NoopHook
//	}
//	func (t *MyTracer) OnPreStep(...) error { ...; return nil }
type NoopHook struct{}

func (NoopHook) OnPreStep(context.Context, *State, Event) error         { return nil }
func (NoopHook) OnPostStep(context.Context, *State, *StepResult, error) {}
func (NoopHook) OnSuspend(context.Context, string, *WaitSpec)           {}
func (NoopHook) OnResume(context.Context, string, Event)                {}

// RegisterHook adds an observer to the executor's loop. Hooks fire in
// registration order. Idempotent for the same pointer — registering the
// same hook twice produces two invocations per event, so callers
// shouldn't double-register.
func (e *Executor) RegisterHook(hook ExecutorHook) {
	if hook == nil {
		return
	}
	e.hooks = append(e.hooks, hook)
}

// callPreStep invokes OnPreStep across all hooks. Returns the first non-
// nil error so the caller can short-circuit. Hooks AFTER the failing one
// are skipped — they didn't see the step start successfully.
func (e *Executor) callPreStep(ctx context.Context, state *State, event Event) error {
	for _, h := range e.hooks {
		if err := h.OnPreStep(ctx, state, event); err != nil {
			return err
		}
	}
	return nil
}

// callPostStep invokes OnPostStep across all hooks. Always called — even
// when the step errored. Hook errors are intentionally ignored (the
// observer can't poison the loop).
func (e *Executor) callPostStep(ctx context.Context, state *State, result *StepResult, err error) {
	for _, h := range e.hooks {
		h.OnPostStep(ctx, state, result, err)
	}
}

// callSuspend invokes OnSuspend across all hooks. Fired after the
// executor decides to wait but before Channel.Wait subscribes.
func (e *Executor) callSuspend(ctx context.Context, runID string, spec *WaitSpec) {
	for _, h := range e.hooks {
		h.OnSuspend(ctx, runID, spec)
	}
}

// callResume invokes OnResume across all hooks. Fired after an inbox
// event unblocks the suspend, before the event is fed to the next Step.
func (e *Executor) callResume(ctx context.Context, runID string, event Event) {
	for _, h := range e.hooks {
		h.OnResume(ctx, runID, event)
	}
}
