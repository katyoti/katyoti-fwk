// SPDX-License-Identifier: Apache-2.0

// Package wkflow implements the "workflow" machine strategy — a multi-step
// orchestrator built on the engine's Spawner + Channel primitives.
//
// Per `decisions/node-handler-interface.md` (D1), each NodeKind has its own
// handler file (`node_<kind>.go`); the workflow machine's Step is a thin
// dispatcher that delegates to the registered handler. This file holds the
// machine struct, its Step dispatch entry point, and the small helper that
// resolves spawn input bindings. The factory + lifecycle wrapping + error
// propagation + per-kind handlers live in their own files.
package wkflow

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ── Workflow Machine ──────────────────────────────────────────────────

type workflowMachine struct {
	root          *Node
	tree          map[string]*Node
	input         map[string]any
	spawner       machine.Spawner
	dryRunMode    bool           // when true, every spawned ChildSpec carries DryRunMode
	mode          string         // workflow's own delegation mode; passed to ChildSpec.ParentMode (UC-053/054)
	spawnPolicies []string       // sticky policy names inherited from the parent's spawn (UC-054 transitive case); forwarded to every spawned child via ChildSpec.ParentSpawnPolicyNames
	currentNode   map[string]any // CWM context node inherited from the parent task; passed to every ChildSpec.CurrentNode so kind:node children can read it via Runtime.CurrentNode
	exprCache     *el.Cache
	runID         string // workflow's own runID; recorded at Step() entry so spawnNode can pass it as ChildSpec.ParentID for journal correlation (Phase C.2)
}

func (m *workflowMachine) Name() string { return "workflow" }

// StateLayout names the layout of the WorkflowState it saves: bump it when
// that shape changes (engine.StateLayouter).
func (m *workflowMachine) StateLayout() string { return "workflow/2" }

// RestartsAfterCrash: a workflow working when its process died restarts from
// its last saved step (engine.Restartable). Its children are runs restored
// with it: a step awaiting them settles them again (children.go); a step that
// was starting them starts them again at the same sites — finding them.
func (m *workflowMachine) RestartsAfterCrash() bool { return true }

func (m *workflowMachine) InitialState(_ machine.Event) machine.State {
	return machine.State{
		Name: "running",
		Data: NewWorkflowState(m.root),
	}
}

// Step dispatches to a kind-specific handler based on the top activation frame.
// Each handler reads its frame, interprets the triggering event, and returns
// an ActionFunc that produces a StepResult.
func (m *workflowMachine) Step(ctx context.Context, state *machine.State, event machine.Event) (machine.ActionFunc, error) {
	ws := m.ensureState(state)
	runID := state.RunID
	if event.Type == machine.RestartedEvent {
		event = machine.Event{Type: machine.TickEvent}
	}
	m.runID = runID // make available to spawnNode → ChildSpec.ParentID (Phase C.2)

	// No active frames → workflow is complete.
	if len(ws.Active) == 0 {
		action, err := m.terminate(ws)
		return m.wrapLifecycleEvents(runID, ws, action, err)
	}

	frame := &ws.Active[len(ws.Active)-1]
	node := m.tree[frame.NodeID]
	if node == nil {
		return nil, fmt.Errorf("workflow: unknown node %q", frame.NodeID)
	}
	if frame.Phase == retryWaitPhase { // its retry's wait is over: run it again
		frame.Phase = ""
	}
	// A step awaiting its children reads one that ended, settled from its
	// outcome — or waits again when none has.
	if awaitsChildren(frame) {
		ev, ok := m.childEvent(ws, frame, event)
		if !ok {
			wait := childWait(frame)
			return m.wrapLifecycleEvents(runID, ws, func(context.Context) (*machine.StepResult, error) {
				return &machine.StepResult{NextState: "running", Data: ws, Wait: wait}, nil
			}, nil)
		}
		event = ev
	}

	// D1: dispatch via the per-kind handler registry (`node_handler.go` +
	// `node_<kind>.go` files). The handlers currently delegate back to the
	// stepXxx methods on workflowMachine; a follow-up moves the method
	// bodies into the handler files and migrates the string `Phase` to
	// typed per-kind Phase structs.
	action, err := dispatchNode(ctx, m, ws, frame, node, event)

	return m.wrapLifecycleEvents(runID, ws, action, err)
}

// resolveInputBinds evaluates input bindings against the current workflow
// state. A binding that cannot be evaluated fails the step, saying which —
// its child never runs with the raw expression string as its input.
func (m *workflowMachine) resolveInputBinds(binds map[string]any, ws *WorkflowState) (map[string]any, error) {
	resolved, err := el.ResolveMap(binds, m.buildScope(ws, nil).AsMap(), m.exprCache)
	if err != nil {
		return nil, fmt.Errorf("its input cannot be evaluated: %w", err)
	}
	return resolved, nil
}
