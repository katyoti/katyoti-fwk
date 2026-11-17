// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"context"
	"fmt"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

type handlerForeach struct{}

func (handlerForeach) Kind() NodeKind { return NodeForeach }

// foreachPhase tracks the in-flight iteration state of a foreach node:
// the materialised item list, the next-item index, and the LoopScope
// stashed for activeLoopScope() to surface to body expressions.
//
// Stored in frame.Local under keys "items", "index", "loop_scope" so the
// JSON checkpoint shape is unchanged AND `activeLoopScope` in lifecycle.go
// keeps its existing read path (it does `f.Local["loop_scope"].(*LoopScope)`
// without going through this phase struct).
type foreachPhase struct {
	Items []any
	Index int
	// LoopScope is the current iteration's loop.index/loop.item exposed to
	// user expressions via activeLoopScope. Nil before the first iteration.
	LoopScope *LoopScope
}

func loadForeachPhase(f *ActivationFrame) foreachPhase {
	items, _ := f.Local["items"].([]any)
	ls, _ := f.Local["loop_scope"].(*LoopScope)
	return foreachPhase{
		Items:     items,
		Index:     getInt(f.Local, "index"),
		LoopScope: ls,
	}
}

func (p foreachPhase) save(f *ActivationFrame) {
	if f.Local == nil {
		f.Local = map[string]any{}
	}
	f.Local["items"] = p.Items
	f.Local["index"] = p.Index
	f.Local["loop_scope"] = p.LoopScope
}

// materialised reports whether the Items list has been computed yet (first
// visit produces nil items from getInt's zero-value behaviour). Foreach uses
// this to know when to evaluate the items expression.
func (p foreachPhase) materialised() bool {
	return p.Items != nil
}

func (handlerForeach) Step(
	_ context.Context, m *workflowMachine, ws *WorkflowState,
	frame *ActivationFrame, node *Node, _ machine.Event,
) (machine.ActionFunc, error) {
	fe := node.Foreach
	phase := loadForeachPhase(frame)

	return func(_ context.Context) (*machine.StepResult, error) {
		// Materialize items on first visit.
		if !phase.materialised() {
			scope := m.buildScope(ws, nil)
			raw, err := m.exprCache.Eval(fe.Items, scope.AsMap())
			if err != nil {
				return nil, fmt.Errorf("foreach items: %w", err)
			}
			phase.Items = toSlice(raw)
			phase.Index = 0
		}

		if phase.Index >= len(phase.Items) {
			ws.PopFrame()
			return &machine.StepResult{NextState: "running", Data: ws}, nil
		}

		// TODO: parallel mode — spawn all items concurrently with bounded concurrency.
		// For now, sequential only.
		curIdx := phase.Index
		phase.Index++
		// D1 quirk #3 / audit F-WF-4: stash the iteration state on the
		// foreach frame's Local rather than ws.Data. The body's
		// `m.buildScope()` walks the activation stack and finds this via
		// `activeLoopScope()`, so user expressions resolve `loop.index` /
		// `loop.item` without the `state._loop_*` magic-key leak.
		phase.LoopScope = &LoopScope{Index: curIdx, Item: phase.Items[curIdx]}
		phase.save(frame)

		ws.PushFrame(&fe.Body)
		return &machine.StepResult{NextState: "running", Data: ws}, nil
	}, nil
}

func init() { registerNodeHandler(handlerForeach{}) }
