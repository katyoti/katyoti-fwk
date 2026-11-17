// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// resolveActionService is the seam between the call handler and the DI
// container — package-level so tests can substitute a stub ActionService
// without bootstrapping an Application.
var resolveActionService = func(ctx context.Context) (actionsapi.ActionService, error) {
	return application.UseService[actionsapi.ActionService](ctx)
}

type handlerCall struct{}

func (handlerCall) Kind() NodeKind { return NodeCall }

// Step executes the call primitive: ONE synchronous action invocation via
// ActionService — in-action, no child machine, no lifecycle topics (the
// A-hybrid model). Per attempt it emits exactly one typed
// call.<name>.completed event (uri / duration / attempt / outcome) so the
// journal and session view can account for the call without it ever
// appearing as a team member. Failures (including timeout) route through
// handleNodeError, so on_error retry/fallback/continue compose; a retry
// re-enters this handler with the frame's retry counter incremented.
func (handlerCall) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	c := node.Call

	if frame.Phase != "" {
		return nil, fmt.Errorf("call %s: unexpected phase %q", c.Name, frame.Phase)
	}

	return func(ctx context.Context) (*machine.StepResult, error) {
		attempt := loadRetryCounter(frame).Count + 1

		// The loader guarantees URI presence; parse failure here is an
		// authoring error — fail through on_error like any node error.
		if _, perr := mbapi.ParseToolURI(c.URI); perr != nil {
			return callFailureResult(m, ws, frame, node, c, attempt, 0, "error",
				fmt.Sprintf("call %s: invalid uri %q: %v", c.Name, c.URI, perr))
		}
		if uri, _ := mbapi.ParseToolURI(c.URI); uri.SlotType == "tasks" {
			return callFailureResult(m, ws, frame, node, c, attempt, 0, "error",
				fmt.Sprintf("call %s: uri %q addresses a task — tasks are state-machine runs, use 'delegate'", c.Name, c.URI))
		}

		svc, err := resolveActionService(ctx)
		if err != nil {
			return callFailureResult(m, ws, frame, node, c, attempt, 0, "error",
				fmt.Sprintf("call %s: ActionService unavailable: %v", c.Name, err))
		}

		resolvedInput, err := m.resolveInputBinds(c.InputBinds, ws)
		if err != nil {
			return callFailureResult(m, ws, frame, node, c, attempt, 0, "error", fmt.Sprintf("call %s: %v", c.Name, err))
		}
		inputJSON, err := json.Marshal(resolvedInput)
		if err != nil {
			return callFailureResult(m, ws, frame, node, c, attempt, 0, "error",
				fmt.Sprintf("call %s: encode input: %v", c.Name, err))
		}

		callCtx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutMs)*time.Millisecond)
		defer cancel()

		// Run in a goroutine so a runner that ignores ctx cannot block the
		// workflow past the timeout. The goroutine is bounded by the action
		// itself; the buffered channel lets it finish and be collected even
		// after we stop waiting.
		type runRes struct {
			out actionsapi.RunOutput
			err error
		}
		started := time.Now()
		resCh := make(chan runRes, 1)
		go func() {
			out, rerr := svc.Run(callCtx, actionsapi.RunInput{URI: c.URI, Input: inputJSON})
			resCh <- runRes{out, rerr}
		}()

		select {
		case r := <-resCh:
			durMs := time.Since(started).Milliseconds()
			if r.err != nil {
				return callFailureResult(m, ws, frame, node, c, attempt, durMs, "error",
					fmt.Sprintf("call %s: %v", c.Name, r.err))
			}
			// Sync actions return Value; a TaskID here means the URI was
			// async after all — surface it loudly rather than store a handle
			// the author didn't ask for.
			if r.out.TaskID != "" {
				return callFailureResult(m, ws, frame, node, c, attempt, durMs, "error",
					fmt.Sprintf("call %s: uri %q ran ASYNC (task_id %s) — use 'delegate' for state-machine runs", c.Name, c.URI, r.out.TaskID))
			}
			var value any
			if len(r.out.Value) > 0 {
				if uerr := json.Unmarshal(r.out.Value, &value); uerr != nil {
					value = string(r.out.Value)
				}
			}
			ws.Results[c.Name] = value
			ws.PopFrame()
			return &machine.StepResult{
				NextState: "running",
				Data:      ws,
				Emit:      callCompletedEvents(c, attempt, durMs, "ok", ""),
			}, nil

		case <-callCtx.Done():
			durMs := time.Since(started).Milliseconds()
			outcome := "timeout"
			if ctx.Err() != nil {
				outcome = "error" // parent cancellation, not the call's own timeout
			}
			return callFailureResult(m, ws, frame, node, c, attempt, durMs, outcome,
				fmt.Sprintf("call %s: %v after %dms (timeout_ms=%d)", c.Name, callCtx.Err(), durMs, c.TimeoutMs))
		}
	}, nil
}

// callCompletedEvents builds the per-attempt observability events: the
// name-templated topic (live observers) PLUS the flat audit mirror (so the
// SessionRecorder journals it and the cold session view shows the call). Same
// payload on both.
func callCompletedEvents(c *CallNode, attempt int, durMs int64, outcome, errMsg string) []machine.Event {
	payload := &machine.CallCompletedEvent{
		Node:       c.Name,
		URI:        c.URI,
		DurationMs: durMs,
		Attempt:    attempt,
		Outcome:    outcome,
		Error:      errMsg,
	}
	return []machine.Event{
		{Type: machine.CallCompletedTopic(c.Name), Payload: payload},
		{Type: machine.CallCompletedAuditTopic(), Payload: payload},
	}
}

// callFailureResult routes a call failure through the node's on_error
// strategy and attaches the attempt's call.completed event to whatever
// StepResult that produces (fail / continue / retry / fallback).
func callFailureResult(
	m *workflowMachine, ws *WorkflowState, frame *ActivationFrame, node *Node,
	c *CallNode, attempt int, durMs int64, outcome, errMsg string,
) (*machine.StepResult, error) {
	res, err := m.handleNodeError(ws, frame, node, errMsg)
	if res != nil {
		res.Emit = append(res.Emit, callCompletedEvents(c, attempt, durMs, outcome, errMsg)...)
	}
	return res, err
}

func init() { registerNodeHandler(handlerCall{}) }
