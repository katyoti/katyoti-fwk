// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerReturn struct{}

func (handlerReturn) Kind() NodeKind { return NodeReturn }

func (handlerReturn) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	_ *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	ret := node.Return

	return func(_ context.Context) (*machine.StepResult, error) {
		// resolveReturnValue evaluates expressions everywhere in the value tree,
		// not just at the top level. Closes D1 quirk #1 — `return: {x: "${...}"}`
		// previously left the inner `${...}` unevaluated because the top-level
		// type-check saw a map and skipped the eval path.
		output := resolveReturnValue(ret.Value, m.buildScope(ws, nil).AsMap(), m.exprCache)

		if output == nil {
			output = ws.Results
		}
		return &machine.StepResult{
			NextState: "done",
			Data:      ws,
			Output:    output,
		}, nil
	}, nil
}

// resolveReturnValue walks a `return:` value tree and evaluates `${expr}`
// strings everywhere they appear. Bare strings (no `${...}` wrapper) stay
// literal — wrap with `${...}` to opt into evaluation. Wrapped expressions
// that fail to evaluate keep the wrapped string so the failure is visible.
//
// Recursion handles:
//   - string: try wrapped eval; bare strings pass through unchanged
//   - map[string]any: recurse on every value
//   - []any: recurse on every element
//   - everything else: pass through unchanged
func resolveReturnValue(v any, vars map[string]any, cache *el.Cache) any {
	switch x := v.(type) {
	case string:
		if inner, wrapped := el.UnwrapExpression(x); wrapped {
			if result, err := cache.Eval(inner, vars); err == nil {
				return result
			}
			return x // wrapped form that failed to evaluate — keep as-is for visibility
		}
		return x
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = resolveReturnValue(val, vars, cache)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = resolveReturnValue(item, vars, cache)
		}
		return out
	default:
		return v
	}
}

func init() { registerNodeHandler(handlerReturn{}) }
