// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"encoding/json"
	"testing"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A workflow restored inside a foreach body still sees its loop: loop.item and
// loop.index are those of the iteration it was in, read back from the saved
// state as they were live.
func TestALoopScopeSurvivesARestore(t *testing.T) {
	m := &workflowMachine{root: &Node{ID: "root", Kind: NodeSequence}}
	saved := &WorkflowState{Active: []ActivationFrame{
		{NodeID: "root", Kind: NodeSequence, Local: map[string]any{"index": 0}},
		{NodeID: "each", Kind: NodeForeach, Local: map[string]any{"items": []any{"a", "b"}, "index": 1, "loop_scope": &LoopScope{Index: 0, Item: "a"}}},
		{NodeID: "w", Kind: NodeWait, Phase: "waiting"},
	}}
	b, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	ws := m.ensureState(&machine.State{Data: decoded})
	ls := activeLoopScope(ws)
	if ls == nil || ls.Index != 0 || ls.Item != "a" {
		t.Fatalf("restored loop scope = %#v; want index 0, item a", ls)
	}
	if p := loadForeachPhase(&ws.Active[1]); p.LoopScope == nil || p.Index != 1 || len(p.Items) != 2 {
		t.Errorf("restored foreach phase = %#v; want its items, its index and its loop scope", p)
	}
}
