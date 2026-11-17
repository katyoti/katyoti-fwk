// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestParseSessionTaskTree pins the cold-reconstruction parse, keyed by RUN ID.
// child_started carries the complete delegation event (label + run + parent +
// mode + kind); task_started fills input; task_output the outcome — all merging
// on run id. team_event entries become the narrative; resources come from
// session_start/end.
func TestParseSessionTaskTree(t *testing.T) {
	journal := `{"type":"session_start","ts":"t0","data":{"model":"claude-fable-5","task":"root"}}
{"type":"child_started","ts":"t1","data":{"runId":"run-child-1","parentRunId":"run-root","childName":"researcher","childKind":"agent","mode":"background"}}
{"type":"task_started","ts":"t1b","data":{"run_id":"run-child-1","task_name":"prompt-worker","mode":"background","input":{"topic":"teams"},"correlation":{"parentRunId":"run-root"}}}
{"type":"task_output","ts":"t2","data":{"run_id":"run-child-1","task_name":"prompt-worker","status":"completed","output":{"summary":"done"},"correlation":{"runId":"run-child-1","parentRunId":"run-root"}}}
{"type":"team_event","ts":"t3","data":{"action":"created","team_id":"team-1","team_name":"crew","state":"forming","correlation":{"runId":"run-root","teamId":"team-1"}}}
{"type":"team_event","ts":"t4","data":{"action":"member-joined","team_id":"team-1","member_id":"researcher","state":"active","correlation":{"runId":"run-root"}}}
{"type":"unknown_kind","ts":"t5","data":{"x":1}}
{"type":"session_end","ts":"t6","data":{"durationMs":1234,"usage":{"input_tokens":100,"output_tokens":50}}}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.jsonl")
	if err := os.WriteFile(path, []byte(journal), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tree, err := parseSessionTaskTree(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if tree.Model != "claude-fable-5" || tree.TotalTokens != 150 || tree.DurationMs != 1234 {
		t.Errorf("resources = model %q tokens %d dur %d", tree.Model, tree.TotalTokens, tree.DurationMs)
	}

	if len(tree.Tasks) != 1 {
		t.Fatalf("tasks = %d, want 1: %+v", len(tree.Tasks), tree.Tasks)
	}
	task := tree.Tasks[0]
	if task.Name != "researcher" || task.RunID != "run-child-1" || task.ParentRunID != "run-root" {
		t.Errorf("identity merge failed (child_started label must win over task_started type): %+v", task)
	}
	if task.Mode != "background" || task.Kind != "agent" {
		t.Errorf("delegation intent missing: mode=%q kind=%q", task.Mode, task.Kind)
	}
	if task.Input["topic"] != "teams" {
		t.Errorf("input missing: %#v", task.Input)
	}
	if task.Status != "completed" || task.Output["summary"] != "done" {
		t.Errorf("outcome missing: status=%q output=%#v", task.Status, task.Output)
	}

	if len(tree.TeamEvents) != 2 {
		t.Fatalf("team events = %d, want 2", len(tree.TeamEvents))
	}
	if tree.TeamEvents[0].Action != "created" || tree.TeamEvents[0].TeamID != "team-1" {
		t.Errorf("narrative[0] = %+v", tree.TeamEvents[0])
	}
	if tree.TeamEvents[1].MemberID != "researcher" || tree.TeamEvents[1].State != "active" {
		t.Errorf("narrative[1] = %+v", tree.TeamEvents[1])
	}
}

// TestParseSessionTaskTree_SameNameNoCollapse is the regression for the bug
// that orphaned the Teams tree: a recursive delegate chain reuses ONE task-type
// name ("prompt-worker") across distinct runs. Name-keying collapsed them into
// a single node and lost every edge; run-id keying must keep them distinct and
// preserve the parent chain root → A → B.
func TestParseSessionTaskTree_SameNameNoCollapse(t *testing.T) {
	journal := `{"type":"task_started","ts":"t0","data":{"run_id":"run-root","task_name":"chat","correlation":{"parentRunId":""}}}
{"type":"task_started","ts":"t1","data":{"run_id":"run-a","task_name":"prompt-worker","correlation":{"parentRunId":"run-root"}}}
{"type":"task_started","ts":"t2","data":{"run_id":"run-b","task_name":"prompt-worker","correlation":{"parentRunId":"run-a"}}}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.jsonl")
	if err := os.WriteFile(path, []byte(journal), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tree, err := parseSessionTaskTree(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(tree.Tasks) != 3 {
		t.Fatalf("tasks = %d, want 3 (run-id keyed, no name collapse): %+v", len(tree.Tasks), tree.Tasks)
	}
	byRun := map[string]SessionTaskNode{}
	for _, n := range tree.Tasks {
		byRun[n.RunID] = n
	}
	if byRun["run-root"].ParentRunID != "" {
		t.Errorf("root must have no parent: %+v", byRun["run-root"])
	}
	if byRun["run-a"].ParentRunID != "run-root" {
		t.Errorf("run-a parent = %q, want run-root", byRun["run-a"].ParentRunID)
	}
	if byRun["run-b"].ParentRunID != "run-a" {
		t.Errorf("run-b parent = %q, want run-a (chain preserved)", byRun["run-b"].ParentRunID)
	}
}

// TestParseSessionTaskTree_CommsCallsLedger pins the Phase-C cold projection:
// inter-agent messages, workflow call observability, and the shared-state
// ledger reconstruct from their journaled audit events.
func TestParseSessionTaskTree_CommsCallsLedger(t *testing.T) {
	journal := `{"type":"agent_message","ts":"t1","data":{"from":"alice","to":"bob","text":"hi","summary":"greet"}}
{"type":"agent_message","ts":"t2","data":{"from":"alice","to":"*","team":"crew","text":"all hands"}}
{"type":"call_completed","ts":"t3","data":{"node":"fetch","uri":"svc:slot/method","durationMs":12,"attempt":1,"outcome":"ok","emitter":"run-wf"}}
{"type":"ledger_snapshot","ts":"t4","data":{"state":{"goal":"g"},"version":1,"emitter":"run-led"}}
{"type":"ledger_complete","ts":"t5","data":{"finalState":{"goal":"done"},"reason":"fin","emitter":"run-led"}}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "llm.jsonl")
	if err := os.WriteFile(path, []byte(journal), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	tree, err := parseSessionTaskTree(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(tree.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(tree.Messages))
	}
	if tree.Messages[0].From != "alice" || tree.Messages[0].To != "bob" || tree.Messages[0].Text != "hi" {
		t.Errorf("direct message wrong: %+v", tree.Messages[0])
	}
	if tree.Messages[1].Team != "crew" || tree.Messages[1].To != "*" {
		t.Errorf("group message wrong: %+v", tree.Messages[1])
	}
	if len(tree.Calls) != 1 || tree.Calls[0].Node != "fetch" || tree.Calls[0].Outcome != "ok" || tree.Calls[0].RunID != "run-wf" {
		t.Errorf("call wrong: %+v", tree.Calls)
	}
	if len(tree.Ledger) != 2 {
		t.Fatalf("ledger = %d, want 2", len(tree.Ledger))
	}
	if tree.Ledger[0].Kind != "snapshot" || tree.Ledger[0].Version != 1 || tree.Ledger[0].State["goal"] != "g" {
		t.Errorf("ledger snapshot wrong: %+v", tree.Ledger[0])
	}
	if tree.Ledger[1].Kind != "complete" || tree.Ledger[1].Reason != "fin" {
		t.Errorf("ledger complete wrong: %+v", tree.Ledger[1])
	}
}
