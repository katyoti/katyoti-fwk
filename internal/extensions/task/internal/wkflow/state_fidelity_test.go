// SPDX-License-Identifier: Apache-2.0

package wkflow

import (
	"reflect"
	"testing"

	machine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine/enginetest"
)

// A WORKFLOW'S SAVED STATE READS BACK WHOLE — every kind of frame a workflow
// keeps (what each control node remembers between its steps), its children,
// results and data are, read back after a restart, what was saved: each
// frame's phase loads as it was saved. A frame's Local holds JSON values once
// read back (an int is a float64): the loaders read both — that is what this
// holds.
func TestEveryFrameReadsBackWhole(t *testing.T) {
	frame := func(kind NodeKind, id string) ActivationFrame {
		return ActivationFrame{NodeID: id, Kind: kind, Local: map[string]any{}}
	}
	seq, par, race, loop, while, each, dlg := frame(NodeSequence, "seq@w"), frame(NodeParallel, "par@w.1"),
		frame(NodeRace, "race@w.2"), frame(NodeLoop, "loop@w.3"), frame(NodeWhile, "while@w.4"),
		frame(NodeForeach, "each@w.5"), frame(NodeDelegate, "dlg@w.5.body")
	sequencePhase{Index: 2}.save(&seq)
	parallelPhase{Pending: 3, Completed: 1}.save(&par)
	par.Local["retry_b1"] = 2
	appendLocalString(&par, "fallback_names", "b1-fallback")
	racePhase{Pending: 2, Failed: 1, LastFailure: "b2: boom"}.save(&race)
	loopPhase{Iteration: 4}.save(&loop)
	whilePhase{Iteration: 5}.save(&while)
	items := []any{"x", 2.0, map[string]any{"k": "v"}}
	foreachPhase{Items: items, Index: 1, LoopScope: &LoopScope{Index: 1, Item: 2.0}}.save(&each)
	dlg.Phase = "awaiting"
	retryCounter{Count: 1}.save(&dlg)

	saved := &WorkflowState{
		Active:   []ActivationFrame{seq, par, race, loop, while, each, dlg},
		Children: map[string]machine.ChildHandle{"b1": {RunID: "r-b1", Name: "b1", Kind: "agent", Status: machine.ChildStatus("running")}},
		Results:  map[string]any{"step1": map[string]any{"ok": true}},
		Data:     map[string]any{"count": 3.0},
	}
	m := &workflowMachine{}
	got := m.ensureState(enginetest.ReadBack(t, machine.State{Name: "running", Data: saved}))
	if len(got.Active) != len(saved.Active) {
		t.Fatalf("frames read back = %d, want %d", len(got.Active), len(saved.Active))
	}
	for i, f := range got.Active {
		if f.NodeID != saved.Active[i].NodeID || f.Kind != saved.Active[i].Kind || f.Phase != saved.Active[i].Phase {
			t.Errorf("frame %d = %s %s %q; want %s %s %q", i, f.NodeID, f.Kind, f.Phase, saved.Active[i].NodeID, saved.Active[i].Kind, saved.Active[i].Phase)
		}
	}
	a := got.Active
	check := func(what string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s read back %#v, want %#v", what, got, want)
		}
	}
	check("sequence", loadSequencePhase(&a[0]), sequencePhase{Index: 2})
	check("parallel", loadParallelPhase(&a[1]), parallelPhase{Pending: 3, Completed: 1})
	check("parallel retries", getInt(a[1].Local, "retry_b1"), 2)
	check("parallel fallbacks", appendLocalString(&a[1], "fallback_names", "next"), []string{"b1-fallback", "next"})
	check("race", loadRacePhase(&a[2]), racePhase{Pending: 2, Failed: 1, LastFailure: "b2: boom"})
	check("loop", loadLoopPhase(&a[3]), loopPhase{Iteration: 4})
	check("while", loadWhilePhase(&a[4]), whilePhase{Iteration: 5})
	check("foreach", loadForeachPhase(&a[5]), foreachPhase{Items: items, Index: 1, LoopScope: &LoopScope{Index: 1, Item: 2.0}})
	check("retries", loadRetryCounter(&a[6]), retryCounter{Count: 1})
	check("children", got.Children, saved.Children)
	check("results", got.Results, saved.Results)
	check("data", got.Data, saved.Data)
}
