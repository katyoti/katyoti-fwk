// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/presets"
)

type handlerDelegate struct{}

func (handlerDelegate) Kind() NodeKind { return NodeDelegate }

// delegateIsBackground reports whether the node's mode schedules the child
// in the background — in which case the workflow does NOT suspend: the
// result map gets {run_id, child, background:true} immediately and a later
// `wait for_event: child.<name>.done` can join. Empty mode resolves to the
// worker preset (foreground), preserving the pre-delegate spawn behavior.
func delegateIsBackground(mode string) bool {
	cfg := presets.MustResolvePreset(presets.Mode(mode))
	return cfg.Scheduling.Mode != presets.SchedulingForeground
}

// resolveDelegateTarget maps the node's addressing form to a spawnable
// (kind, config). Inline and taskName forms are precomputed by the loader;
// the URI form resolves here: `<provider>:tasks/<name>` → kind "task" +
// _taskName. Non-task slots are rejected — sync actions belong to `call`.
func resolveDelegateTarget(d *DelegateNode) (kind string, config map[string]any, err error) {
	if d.URI == "" {
		return d.ChildKind, d.Config, nil
	}
	uri, perr := mbapi.ParseToolURI(d.URI)
	if perr != nil {
		return "", nil, fmt.Errorf("delegate %s: invalid uri %q: %w", d.ChildName, d.URI, perr)
	}
	if uri.SlotType != "tasks" {
		return "", nil, fmt.Errorf("delegate %s: uri %q addresses slot %q — only task URIs can be delegated (sync actions use 'call')", d.ChildName, d.URI, uri.SlotType)
	}
	return "task", map[string]any{"_taskName": uri.Method}, nil
}

func (handlerDelegate) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, event machine.Event,
) (machine.ActionFunc, error) {
	d := node.Delegate

	switch frame.Phase {
	case "": // First visit: spawn the child.
		return func(ctx context.Context) (*machine.StepResult, error) {
			if m.spawner == nil {
				return nil, fmt.Errorf("delegate %s: no spawner available", d.ChildName)
			}

			background := delegateIsBackground(d.Mode)
			handle, err := m.spawnChild(ctx, ws, frame, d)
			if err != nil {
				return nil, fmt.Errorf("delegate %s: %w", d.ChildName, err)
			}

			if background {
				// Background: don't suspend. The handle is the result; a
				// later `wait for_event: child.<name>.done` joins it.
				ws.Results[d.ChildName] = map[string]any{
					"run_id":     handle.RunID,
					"child":      d.ChildName,
					"background": true,
				}
				ws.PopFrame()
				return &machine.StepResult{NextState: "running", Data: ws}, nil
			}

			frame.Phase = "awaiting"
			// Suspend: wait for its run's end (settled from its outcome).
			return &machine.StepResult{NextState: "running", Data: ws, Wait: childWait(frame)}, nil
		}, nil

	case "awaiting": // Resumed: child completed.
		return func(_ context.Context) (*machine.StepResult, error) {
			switch event.Label {
			case "done":
				// Extract output from the event payload.
				var output any
				if evt, ok := event.Payload.(*machine.ChildDoneEvent); ok {
					output = evt.Output
				} else {
					output = event.Payload
				}
				ws.Results[d.ChildName] = output

				// Pop frame — delegate is done.
				ws.PopFrame()
				return &machine.StepResult{NextState: "running", Data: ws}, nil

			case "failed":
				var errMsg string
				if evt, ok := event.Payload.(*machine.ChildFailedEvent); ok {
					errMsg = evt.Error
				} else {
					errMsg = fmt.Sprintf("child %s failed", d.ChildName)
				}
				return m.handleNodeError(ws, frame, node, errMsg)

			default:
				return nil, fmt.Errorf("delegate %s: unexpected event label %q", d.ChildName, event.Label)
			}
		}, nil
	}

	return nil, fmt.Errorf("delegate %s: unexpected phase %q", d.ChildName, frame.Phase)
}

func init() { registerNodeHandler(handlerDelegate{}) }
