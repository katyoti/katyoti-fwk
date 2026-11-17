// SPDX-License-Identifier: Apache-2.0

package wkflow

import "time"

// Scope is the evaluation context available to workflow expressions.
// Built from WorkflowState + workflow inputs before each evaluation.
//
// Scope is the workflow-specific projection — el.Cache and el.ResolveMap
// take a flat map[string]any, which Scope produces via AsMap.
type Scope struct {
	// Inputs are the workflow's own input parameters (from TaskRunRequest.Input).
	Inputs map[string]any `expr:"inputs"`

	// State is the workflow's shared data namespace (WorkflowState.Data).
	State map[string]any `expr:"state"`

	// Results maps child names to their output values (WorkflowState.Results).
	Results map[string]any `expr:"results"`

	// Loop is injected inside foreach body scope. Nil outside loops.
	Loop *LoopScope `expr:"loop"`

	// Now is the current time (injectable for testing).
	Now time.Time `expr:"now"`
}

// LoopScope provides loop iteration context for foreach expressions.
type LoopScope struct {
	Index int `expr:"index"`
	Item  any `expr:"item"`
}

// AsMap converts the scope to a flat map for el.Cache evaluation.
func (s *Scope) AsMap() map[string]any {
	m := map[string]any{
		"inputs":  s.Inputs,
		"state":   s.State,
		"results": s.Results,
		"now":     s.Now,
	}
	if s.Loop != nil {
		m["loop"] = map[string]any{
			"index": s.Loop.Index,
			"item":  s.Loop.Item,
		}
	}
	return m
}

// BuildScope creates a Scope from workflow state and inputs.
func BuildScope(ws *WorkflowState, inputs map[string]any, loop *LoopScope) *Scope {
	state := ws.Data
	if state == nil {
		state = make(map[string]any)
	}
	results := ws.Results
	if results == nil {
		results = make(map[string]any)
	}
	if inputs == nil {
		inputs = make(map[string]any)
	}
	return &Scope{
		Inputs:  inputs,
		State:   state,
		Results: results,
		Loop:    loop,
		Now:     time.Now(),
	}
}
