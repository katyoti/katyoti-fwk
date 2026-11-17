// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// WorkflowState is the checkpointable state of a running workflow.
// It is stored in engine.State.Data and serialized on each checkpoint.
type WorkflowState struct {
	// Active is the execution stack — one frame per nested control node.
	// The top of the stack (last element) is what Step dispatches on.
	Active []ActivationFrame `json:"active"`

	// Children currently running or completed, indexed by role name.
	Children map[string]engine.ChildHandle `json:"children,omitempty"`

	// Results is the global output map, keyed by child role name.
	// Populated when a spawned child completes (child.done event).
	// Accessible in expressions as state.results.<name>.
	Results map[string]any `json:"results"`

	// Data is free-form shared state, writable by Assign nodes
	// and readable in expressions as state.<key>.
	Data map[string]any `json:"data,omitempty"`

	// Panels are the display panels the workflow's panel steps drive, by
	// name: each one's id, sequence and state, kept with the
	// run so its numbering goes on after a restart.
	Panels map[string]panel.Track `json:"panels,omitempty"`

	// StartedEmitted tracks whether workflow.<runID>.started has been emitted.
	// Set to true on first Step call to emit exactly once.
	StartedEmitted bool `json:"startedEmitted,omitempty"`
}

// ActivationFrame tracks the execution state of one in-flight node.
type ActivationFrame struct {
	// NodeID is the unique identifier of the node being executed.
	NodeID string `json:"nodeId"`

	// Kind is the node's type (for dispatch without tree lookup).
	Kind NodeKind `json:"kind"`

	// Phase is a kind-specific phase label (e.g., "", "awaiting" for spawn).
	Phase string `json:"phase,omitempty"`

	// Local holds kind-specific state: counters, collected results, etc.
	Local map[string]any `json:"local,omitempty"`
}

// NewWorkflowState creates an initial WorkflowState with a single
// activation frame for the root node.
func NewWorkflowState(rootNode *Node) *WorkflowState {
	return &WorkflowState{
		Active: []ActivationFrame{
			{
				NodeID: rootNode.ID,
				Kind:   rootNode.Kind,
				Local:  make(map[string]any),
			},
		},
		Children: make(map[string]engine.ChildHandle),
		Results:  make(map[string]any),
		Data:     make(map[string]any),
	}
}

// PushFrame pushes a fresh activation frame for the given node. Centralises
// the push discipline so handlers don't open-code the slice append +
// ActivationFrame literal.
//
// Per D1 Phase 3.b, this is one half of the "frameStack helper" — the other
// half is PopFrame below. Together they eliminate the class of off-by-one
// risks that plagued the pre-refactor god-method (one handler forgot to
// pop, another double-popped).
func (ws *WorkflowState) PushFrame(n *Node) {
	ws.Active = append(ws.Active, ActivationFrame{
		NodeID: n.ID,
		Kind:   n.Kind,
		Local:  map[string]any{},
	})
}

// PopFrame removes the top activation frame. Caller is responsible for
// checking that Active is non-empty (most callers know they're at a frame
// boundary). Returns the popped frame for callers that need its state.
func (ws *WorkflowState) PopFrame() ActivationFrame {
	last := len(ws.Active) - 1
	popped := ws.Active[last]
	ws.Active = ws.Active[:last]
	return popped
}
