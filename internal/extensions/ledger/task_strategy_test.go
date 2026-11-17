// SPDX-License-Identifier: Apache-2.0

package ledger

import (
	"context"
	"reflect"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// ── mergePatch (RFC 7396) ────────────────────────────────────────────

func TestMergePatch_TopLevelOverwrite(t *testing.T) {
	dst := map[string]any{"a": 1, "b": 2}
	patch := map[string]any{"b": 20, "c": 3}
	got := mergePatch(dst, patch)
	want := map[string]any{"a": 1, "b": 20, "c": 3}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMergePatch_NullDeletes(t *testing.T) {
	dst := map[string]any{"a": 1, "b": 2, "c": 3}
	patch := map[string]any{"b": nil}
	got := mergePatch(dst, patch)
	if _, exists := got["b"]; exists {
		t.Errorf("key 'b' should have been deleted, got %v", got)
	}
	if got["a"] != 1 || got["c"] != 3 {
		t.Errorf("other keys should survive, got %v", got)
	}
}

func TestMergePatch_NestedObjectsMerge(t *testing.T) {
	dst := map[string]any{
		"plan": map[string]any{
			"step1": map[string]any{"status": "done"},
			"step2": map[string]any{"status": "pending"},
		},
	}
	patch := map[string]any{
		"plan": map[string]any{
			"step2": map[string]any{"status": "in_progress", "assignee": "alice"},
		},
	}
	got := mergePatch(dst, patch)
	plan := got["plan"].(map[string]any)
	step1 := plan["step1"].(map[string]any)
	step2 := plan["step2"].(map[string]any)

	if step1["status"] != "done" {
		t.Errorf("step1 should be unchanged: %v", step1)
	}
	if step2["status"] != "in_progress" {
		t.Errorf("step2.status should be updated: %v", step2)
	}
	if step2["assignee"] != "alice" {
		t.Errorf("step2.assignee should be added: %v", step2)
	}
}

func TestMergePatch_NestedNullDeletes(t *testing.T) {
	dst := map[string]any{
		"plan": map[string]any{"step1": "done", "step2": "pending"},
	}
	patch := map[string]any{
		"plan": map[string]any{"step1": nil},
	}
	got := mergePatch(dst, patch)
	plan := got["plan"].(map[string]any)
	if _, exists := plan["step1"]; exists {
		t.Errorf("plan.step1 should be deleted: %v", plan)
	}
	if plan["step2"] != "pending" {
		t.Errorf("plan.step2 should remain: %v", plan)
	}
}

func TestMergePatch_NilDstStarts(t *testing.T) {
	got := mergePatch(nil, map[string]any{"a": 1})
	if got["a"] != 1 {
		t.Errorf("got %v", got)
	}
}

// ── Machine lifecycle ────────────────────────────────────────────────

func TestLedger_InitialStateFromConfig(t *testing.T) {
	f := &ledgerFactory{}
	req := &taskapi.TaskExecRequest{
		Config: map[string]any{
			"initial": map[string]any{"goal": "audit codebase"},
		},
	}
	m, err := f.CreateMachine(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	state := m.InitialState(engine.Event{})
	ls := state.Data.(*ledgerState)
	if ls.Snapshot["goal"] != "audit codebase" {
		t.Errorf("initial snapshot wrong: %v", ls.Snapshot)
	}
	if ls.Version != 0 {
		t.Errorf("initial version should be 0, got %d", ls.Version)
	}
}

func TestLedger_InputOverridesConfigInitial(t *testing.T) {
	f := &ledgerFactory{}
	req := &taskapi.TaskExecRequest{
		Config: map[string]any{
			"initial": map[string]any{"from_config": true},
		},
		Input: map[string]any{
			"initial": map[string]any{"from_input": true},
		},
	}
	m, err := f.CreateMachine(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateMachine: %v", err)
	}
	state := m.InitialState(engine.Event{})
	ls := state.Data.(*ledgerState)
	if _, ok := ls.Snapshot["from_input"]; !ok {
		t.Errorf("input.initial should win: %v", ls.Snapshot)
	}
	if _, ok := ls.Snapshot["from_config"]; ok {
		t.Errorf("config.initial should be overridden: %v", ls.Snapshot)
	}
}

func TestLedger_FirstStepEmitsInitialSnapshot(t *testing.T) {
	m := &ledgerMachine{initial: map[string]any{"goal": "test"}}
	state := engine.State{
		Name:  "running",
		RunID: "run-1",
		Data:  &ledgerState{Snapshot: map[string]any{"goal": "test"}, Version: 0},
	}
	action, err := m.Step(context.Background(), &state, engine.Event{})
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	result, err := action(context.Background())
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	// Two emits: the name-templated topic (live) + the flat audit mirror (journal).
	if len(result.Emit) != 2 {
		t.Fatalf("expected 2 emit events (snapshot + audit mirror), got %d", len(result.Emit))
	}
	if result.Emit[0].Type != engine.LedgerSnapshotTopic("run-1") {
		t.Errorf("emit topic = %q, want %q", result.Emit[0].Type, engine.LedgerSnapshotTopic("run-1"))
	}
	if result.Emit[1].Type != engine.LedgerSnapshotAuditTopic() {
		t.Errorf("mirror topic = %q, want %q", result.Emit[1].Type, engine.LedgerSnapshotAuditTopic())
	}
	evt := result.Emit[0].Payload.(*engine.LedgerSnapshotEvent)
	if evt.Version != 0 {
		t.Errorf("snapshot version = %d, want 0", evt.Version)
	}
	if evt.State["goal"] != "test" {
		t.Errorf("snapshot state wrong: %v", evt.State)
	}
	if result.Wait == nil {
		t.Fatal("expected Wait to be set (suspended)")
	}
	// Should wait on update + complete topics.
	if len(result.Wait.Conditions) != 2 {
		t.Errorf("expected 2 wait conditions, got %d", len(result.Wait.Conditions))
	}
}

func TestLedger_UpdateAppliesPatchAndIncrementsVersion(t *testing.T) {
	m := &ledgerMachine{initial: map[string]any{}}
	state := engine.State{
		Name:  "running",
		RunID: "run-1",
		Data:  &ledgerState{Snapshot: map[string]any{"a": 1}, Version: 5},
	}
	updateEvt := engine.Event{
		Label: "update",
		Payload: &engine.LedgerUpdateEvent{
			Patch: map[string]any{"b": 2},
		},
	}
	action, err := m.Step(context.Background(), &state, updateEvt)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	result, err := action(context.Background())
	if err != nil {
		t.Fatalf("action: %v", err)
	}

	ls := result.Data.(*ledgerState)
	if ls.Snapshot["a"] != 1 || ls.Snapshot["b"] != 2 {
		t.Errorf("merged snapshot wrong: %v", ls.Snapshot)
	}
	if ls.Version != 6 {
		t.Errorf("version should be 6, got %d", ls.Version)
	}

	// Should emit a snapshot event (+ its flat audit mirror).
	if len(result.Emit) != 2 {
		t.Fatalf("expected 2 emit (snapshot + audit mirror), got %d", len(result.Emit))
	}
	if result.Emit[1].Type != engine.LedgerSnapshotAuditTopic() {
		t.Errorf("mirror topic = %q, want %q", result.Emit[1].Type, engine.LedgerSnapshotAuditTopic())
	}
	evt := result.Emit[0].Payload.(*engine.LedgerSnapshotEvent)
	if evt.Version != 6 {
		t.Errorf("emitted snapshot version = %d, want 6", evt.Version)
	}

	// Should suspend again.
	if result.Wait == nil {
		t.Error("expected Wait to be set after update")
	}
}

func TestLedger_CompleteTerminatesWithCurrentSnapshot(t *testing.T) {
	m := &ledgerMachine{}
	state := engine.State{
		Name:  "running",
		RunID: "run-1",
		Data:  &ledgerState{Snapshot: map[string]any{"final": "value"}, Version: 3},
	}
	completeEvt := engine.Event{
		Label:   "complete",
		Payload: &engine.LedgerCompleteEvent{Reason: "done"},
	}
	action, err := m.Step(context.Background(), &state, completeEvt)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	result, err := action(context.Background())
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	if result.Output == nil {
		t.Fatal("Output should be set on complete (terminal)")
	}
	out := result.Output.(map[string]any)
	if out["final"] != "value" {
		t.Errorf("final output should be current snapshot: %v", out)
	}
	// Wait should be nil (terminal).
	if result.Wait != nil {
		t.Error("Wait should be nil on terminal")
	}
}

func TestLedger_CompleteWithFinalStateOverride(t *testing.T) {
	m := &ledgerMachine{}
	state := engine.State{
		Name:  "running",
		RunID: "run-1",
		Data:  &ledgerState{Snapshot: map[string]any{"current": "snapshot"}, Version: 1},
	}
	completeEvt := engine.Event{
		Label: "complete",
		Payload: &engine.LedgerCompleteEvent{
			FinalState: map[string]any{"override": true},
			Reason:     "abort",
		},
	}
	action, err := m.Step(context.Background(), &state, completeEvt)
	if err != nil {
		t.Fatalf("Step: %v", err)
	}
	result, err := action(context.Background())
	if err != nil {
		t.Fatalf("action: %v", err)
	}
	out := result.Output.(map[string]any)
	if out["override"] != true {
		t.Errorf("FinalState override should win: %v", out)
	}
	if _, ok := out["current"]; ok {
		t.Errorf("current snapshot should be overridden: %v", out)
	}
}

func TestLedger_EnsureStateRebuildsFromMap(t *testing.T) {
	// Simulates checkpoint restore — State.Data may be a generic map.
	state := &engine.State{
		Data: map[string]any{
			"snapshot": map[string]any{"x": 1},
			"version":  float64(7), // JSON deserialization gives float64
		},
	}
	ls := ensureState(state, nil)
	if ls.Snapshot["x"] != 1 {
		t.Errorf("snapshot not restored: %v", ls.Snapshot)
	}
	if ls.Version != 7 {
		t.Errorf("version not restored: %d", ls.Version)
	}
}

func TestLedger_FactoryName(t *testing.T) {
	f := &ledgerFactory{}
	if f.Name() != "ledger" {
		t.Errorf("Name() = %q", f.Name())
	}
}

func TestLedger_BodyFieldNameIsEmpty(t *testing.T) {
	f := &ledgerFactory{}
	if f.BodyFieldName() != "" {
		t.Errorf("BodyFieldName() = %q, want empty (no body)", f.BodyFieldName())
	}
}
