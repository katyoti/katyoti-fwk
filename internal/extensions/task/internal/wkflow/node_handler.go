// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// nodeHandler dispatches a single workflow node kind. Each NodeKind has one
// registered implementer; the workflow machine's Step() looks up the
// handler by `frame.Kind` and delegates.
//
// Splits the historical 910-LOC `task_strategy.go` god-method into per-kind
// files per `decisions/node-handler-interface.md` (D1). The interface is
// minimal: a kind discriminator + a Step function that mirrors
// `machine.Machine.Step` semantics but scoped to one node kind.
type nodeHandler interface {
	// Kind returns the NodeKind discriminator used for dispatch.
	Kind() NodeKind

	// Step is the per-kind step function. Reads the top activation frame +
	// the resolved node + the triggering event; returns an ActionFunc that
	// produces a StepResult.
	Step(
		ctx context.Context,
		m *workflowMachine,
		ws *WorkflowState,
		frame *ActivationFrame,
		node *Node,
		event machine.Event,
	) (machine.ActionFunc, error)
}

// nodeHandlers is the per-process registry of node-kind handlers. Each
// `node_<kind>.go` file registers its handler in an init() block.
//
// The map is build-time-only — registrations happen during package init,
// after which it is read-only. No mutex needed.
var nodeHandlers = map[NodeKind]nodeHandler{}

// registerNodeHandler wires a handler into the dispatch registry. Called
// from per-kind init() blocks. Duplicate registrations panic — that means
// two files registered the same kind, which is always a programming error.
func registerNodeHandler(h nodeHandler) {
	kind := h.Kind()
	if _, exists := nodeHandlers[kind]; exists {
		panic(fmt.Sprintf("wkflow: duplicate node handler for kind %q", kind))
	}
	nodeHandlers[kind] = h
}

// dispatchNode looks up the registered handler for `frame.Kind` and
// delegates. Returns an error if no handler is registered — that means a
// new NodeKind was added in node.go without a companion handler file.
func dispatchNode(
	ctx context.Context,
	m *workflowMachine,
	ws *WorkflowState,
	frame *ActivationFrame,
	node *Node,
	event machine.Event,
) (machine.ActionFunc, error) {
	handler, ok := nodeHandlers[frame.Kind]
	if !ok {
		return nil, fmt.Errorf("workflow: no handler registered for node kind %q", frame.Kind)
	}
	return handler.Step(ctx, m, ws, frame, node, event)
}
