// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"encoding/json"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ensureState reads the per-frame WorkflowState out of machine.State.Data.
// A state restored from a checkpoint arrives JSON-decoded (a map): it is
// decoded back, so the workflow continues where it was. On the first Step
// (no state yet) it is a fresh state rooted at m.root.
func (m *workflowMachine) ensureState(state *machine.State) *WorkflowState {
	if ws, ok := state.Data.(*WorkflowState); ok {
		return ws
	}
	if state.Data != nil {
		ws := &WorkflowState{}
		if b, err := json.Marshal(state.Data); err == nil && json.Unmarshal(b, ws) == nil && len(ws.Active) > 0 {
			if ws.Children == nil {
				ws.Children = make(map[string]machine.ChildHandle)
			}
			if ws.Results == nil {
				ws.Results = make(map[string]any)
			}
			if ws.Data == nil {
				ws.Data = make(map[string]any)
			}
			for i := range ws.Active {
				if ws.Active[i].Local == nil {
					ws.Active[i].Local = make(map[string]any)
				}
				rehydrateLocal(&ws.Active[i])
			}
			return ws
		}
	}
	return NewWorkflowState(m.root)
}

// rehydrateLocal gives back their Go types to the frame values a JSON restore
// turned into maps: a foreach frame's loop scope (`loop.item`, `loop.index` of
// the iteration it was in).
func rehydrateLocal(f *ActivationFrame) {
	if f.Kind != NodeForeach {
		return
	}
	if m, ok := f.Local["loop_scope"].(map[string]any); ok {
		ls := &LoopScope{}
		if b, err := json.Marshal(m); err == nil && json.Unmarshal(b, ls) == nil {
			f.Local["loop_scope"] = ls
		}
	}
}

// indexTree walks the Node tree once and populates a flat ID→Node map for
// O(1) lookups during Step dispatch. Called once at machine construction.
func indexTree(tree map[string]*Node, n *Node) {
	tree[n.ID] = n
	if n.Sequence != nil {
		for i := range n.Sequence.Steps {
			indexTree(tree, &n.Sequence.Steps[i])
		}
	}
	if n.Parallel != nil {
		for i := range n.Parallel.Branches {
			indexTree(tree, &n.Parallel.Branches[i])
		}
	}
	if n.Race != nil {
		for i := range n.Race.Branches {
			indexTree(tree, &n.Race.Branches[i])
		}
	}
	if n.If != nil {
		indexTree(tree, &n.If.Then)
		if n.If.Else != nil {
			indexTree(tree, n.If.Else)
		}
	}
	if n.Switch != nil {
		for k := range n.Switch.Cases {
			c := n.Switch.Cases[k]
			indexTree(tree, &c)
		}
		if n.Switch.Default != nil {
			indexTree(tree, n.Switch.Default)
		}
	}
	if n.Loop != nil {
		indexTree(tree, &n.Loop.Body)
	}
	if n.While != nil {
		indexTree(tree, &n.While.Body)
	}
	if n.Foreach != nil {
		indexTree(tree, &n.Foreach.Body)
	}
	if n.OnError != nil && n.OnError.Fallback != nil {
		indexTree(tree, n.OnError.Fallback)
	}
}

// getInt extracts an int from a map[string]any, handling both int and
// float64 (the latter is how JSON deserialization restores numeric values).
// Returns 0 if the key is missing or the value isn't numeric.
func getInt(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	}
	return 0
}

// getString extracts a string from a map[string]any. Returns "" if the key
// is missing or the value isn't a string.
func getString(m map[string]any, key string) string {
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

// toSlice casts an any to []any. Used by handlerForeach to materialize the
// `items` expression into an iterable slice. Returns nil if the value
// isn't already a slice (caller treats nil as "no items").
func toSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// strVal extracts a string from a map[string]any. Returns "" if the key is
// missing or the value isn't a string.
func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
