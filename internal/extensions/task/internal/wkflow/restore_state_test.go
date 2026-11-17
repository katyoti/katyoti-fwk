// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"encoding/json"
	"testing"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// A workflow's state restored from a checkpoint arrives JSON-decoded: the
// workflow continues where it was (its frames, results, data), not from its
// start.
func TestARestoredWorkflowContinuesWhereItWas(t *testing.T) {
	root := &Node{ID: "root", Kind: NodeSequence}
	m := &workflowMachine{root: root}
	saved := &WorkflowState{
		Active: []ActivationFrame{
			{NodeID: "root", Kind: NodeSequence, Local: map[string]any{"index": 1}},
			{NodeID: "wait-for-approval", Kind: NodeSequence, Phase: "awaiting"},
		},
		Results: map[string]any{"draft": map[string]any{"ok": true}},
		Data:    map[string]any{"round": 2},
	}
	b, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any // what a checkpoint read back from disk holds
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}

	ws := m.ensureState(&machine.State{Data: decoded})
	if len(ws.Active) != 2 || ws.Active[1].NodeID != "wait-for-approval" || ws.Active[1].Phase != "awaiting" {
		t.Fatalf("frames = %+v, want the two saved frames", ws.Active)
	}
	if ws.Results["draft"] == nil || ws.Data["round"] == nil || ws.Children == nil || ws.Active[1].Local == nil {
		t.Errorf("restored state lost its content: %+v", ws)
	}

	if fresh := m.ensureState(&machine.State{}); len(fresh.Active) != 1 || fresh.Active[0].NodeID != "root" {
		t.Errorf("no state yet: %+v, want a fresh state at the root", fresh.Active)
	}
}
