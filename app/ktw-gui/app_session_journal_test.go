// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseLLMJournal_MultiRunNotMerged pins the core fix: a single
// llm.jsonl that interleaves TWO runs (a delegating parent + its child)
// must NOT collapse the runs' same-numbered turns into one card. Each
// turn keeps its originating run (RunID + a T-label assigned by first
// appearance), and turns are emitted in temporal (file) order.
func TestParseLLMJournal_MultiRunNotMerged(t *testing.T) {
	lines := []string{
		`{"type":"session_start","seq":1,"data":{"emitter":"run-A","model":"sonnet","task":"parent"}}`,
		`{"type":"llm_request","seq":2,"turn":1,"data":{"emitter":"run-A","messages":[{"role":"human","text":"start parent"}]}}`,
		`{"type":"session_start","seq":3,"data":{"emitter":"run-B","model":"haiku","task":"child"}}`,
		`{"type":"llm_request","seq":4,"turn":1,"data":{"emitter":"run-B","messages":[{"role":"human","text":"start child"}]}}`,
		`{"type":"tool_call","seq":5,"turn":1,"data":{"emitter":"run-B","tool":"ask-user","id":"c1","input":{"question":"color?"}}}`,
		`{"type":"llm_request","seq":6,"turn":2,"data":{"emitter":"run-A","messages":[{"role":"human","text":"start parent"}]}}`,
		`{"type":"task_output","seq":7,"data":{"emitter":"run-B","task_name":"via-agent-child"}}`,
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.jsonl")
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	conv, err := parseLLMJournal(path)
	if err != nil {
		t.Fatalf("parseLLMJournal: %v", err)
	}

	// Two runs, labelled by first appearance.
	if len(conv.Runs) != 2 {
		t.Fatalf("Runs = %d, want 2 (%+v)", len(conv.Runs), conv.Runs)
	}
	if conv.Runs[0].Label != "T1" || conv.Runs[0].RunID != "run-A" {
		t.Errorf("Run[0] = %+v, want T1/run-A", conv.Runs[0])
	}
	if conv.Runs[1].Label != "T2" || conv.Runs[1].RunID != "run-B" {
		t.Errorf("Run[1] = %+v, want T2/run-B", conv.Runs[1])
	}
	// task_output supplies the child's resolved task name.
	if conv.Runs[1].Task != "via-agent-child" {
		t.Errorf("Run[1].Task = %q, want via-agent-child", conv.Runs[1].Task)
	}

	// Three distinct turns — NOT merged: (run-A,1), (run-B,1), (run-A,2).
	if len(conv.Turns) != 3 {
		t.Fatalf("Turns = %d, want 3 (merged?) %+v", len(conv.Turns), conv.Turns)
	}
	// Temporal (file) order: A#1, B#1, A#2.
	want := []struct {
		label string
		num   int
	}{
		{"T1", 1},
		{"T2", 1},
		{"T1", 2},
	}
	for i, w := range want {
		if conv.Turns[i].RunLabel != w.label || conv.Turns[i].Number != w.num {
			t.Errorf("Turn[%d] = %s#%d, want %s#%d",
				i, conv.Turns[i].RunLabel, conv.Turns[i].Number, w.label, w.num)
		}
	}

	// The child's turn carries its ask-user tool call (not merged onto the
	// parent's turn 1).
	childTurn := conv.Turns[1]
	if len(childTurn.ToolCalls) != 1 || childTurn.ToolCalls[0].Name != "ask-user" {
		t.Errorf("child turn tool calls = %+v, want one ask-user", childTurn.ToolCalls)
	}

	// Header reflects the ROOT run (T1), not whichever session_start came last.
	if conv.Model != "sonnet" {
		t.Errorf("conv.Model = %q, want sonnet (root run)", conv.Model)
	}
}

func joinLines(ls []string) string {
	out := ""
	for _, l := range ls {
		out += l + "\n"
	}
	return out
}
