// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// TestSubscribeRecorder_TaskOutput_LandsInLLMJournal verifies the Phase C.2
// terminal-event hook: publishing a TaskOutputEvent on the bus while a
// recorder is subscribed lands a `task_output` entry in the LLM journal,
// carrying the run's Output map (including any `__spawn_dimensions` snapshot
// from a simulator that recorded its context).
func TestSubscribeRecorder_TaskOutput_LandsInLLMJournal(t *testing.T) {
	dir := t.TempDir()

	recorder, err := engine.NewSessionRecorder(dir, "test-task", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	defer recorder.Close()

	bus := engine.NewMemoryEventBus()
	const runID = "run-test-abc"
	subscribeRecorder(bus, recorder, runID)

	evt := &engine.TaskOutputEvent{
		RunID:    runID,
		TaskName: "demo-task",
		Status:   "completed",
		Output: map[string]any{
			"summary": "ok",
			"__spawn_dimensions": map[string]any{
				"permissions": map[string]any{
					"allowed_tools":   []any{"file-read", "complete"},
					"permission_mode": "restricted",
				},
				"isolation": map[string]any{"mode": "context_isolated"},
			},
		},
	}
	evt.StampEvent(engine.EventTaskOutput, runID)
	bus.Publish(engine.EventTaskOutput, evt)

	// Flush + read llm.jsonl
	_ = recorder.Close() // idempotent per perf-1 fix; flushes any pending writes
	llmPath := recorder.LLMJournal().Path()
	data, err := os.ReadFile(llmPath)
	if err != nil {
		t.Fatalf("read llm journal %s: %v", llmPath, err)
	}

	var found *engine.JournalEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry engine.JournalEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("unmarshal entry %q: %v", line, err)
		}
		if entry.Type == engine.JournalTaskOutput {
			e := entry
			found = &e
			break
		}
	}
	if found == nil {
		t.Fatalf("no task_output entry found in llm journal; full content:\n%s", string(data))
	}

	// The Data field will be the published event as a typed payload or as a map
	// after json round-trip. Marshal+unmarshal again to extract a stable map.
	raw, err := json.Marshal(found.Data)
	if err != nil {
		t.Fatalf("re-marshal data: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal data as map: %v", err)
	}
	if m["run_id"] != runID {
		t.Errorf("run_id = %v, want %s", m["run_id"], runID)
	}
	if m["status"] != "completed" {
		t.Errorf("status = %v, want completed", m["status"])
	}
	output, _ := m["output"].(map[string]any)
	if output == nil {
		t.Fatalf("output missing from task_output payload; got %+v", m)
	}
	dims, _ := output["__spawn_dimensions"].(map[string]any)
	if dims == nil {
		t.Fatalf("__spawn_dimensions missing from output; got %+v", output)
	}
	perms, _ := dims["permissions"].(map[string]any)
	if perms == nil || perms["permission_mode"] != "restricted" {
		t.Errorf("permissions.permission_mode = %v, want restricted (full dims: %+v)", perms, dims)
	}
}

// TestSubscribeRecorder_TaskOutput_FiltersByEmitter verifies the run-isolation
// guarantee: a task_output event from a different run is dropped, so each
// recorder's journal only contains its own run's terminal event.
func TestSubscribeRecorder_TaskOutput_FiltersByEmitter(t *testing.T) {
	dir := t.TempDir()
	recorder, err := engine.NewSessionRecorder(dir, "isolation-test", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	defer recorder.Close()

	bus := engine.NewMemoryEventBus()
	const ourRunID = "run-ours"
	subscribeRecorder(bus, recorder, ourRunID)

	// Publish a foreign event (different emitter).
	foreign := &engine.TaskOutputEvent{RunID: "run-other", Status: "completed"}
	foreign.StampEvent(engine.EventTaskOutput, "run-other")
	bus.Publish(engine.EventTaskOutput, foreign)

	// Publish our own.
	ours := &engine.TaskOutputEvent{RunID: ourRunID, Status: "completed", Output: map[string]any{"k": "v"}}
	ours.StampEvent(engine.EventTaskOutput, ourRunID)
	bus.Publish(engine.EventTaskOutput, ours)

	_ = recorder.Close()
	data, _ := os.ReadFile(recorder.LLMJournal().Path())

	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry engine.JournalEntry
		_ = json.Unmarshal([]byte(line), &entry)
		if entry.Type == engine.JournalTaskOutput {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 task_output entry (own run), got %d; journal:\n%s", count, string(data))
	}
}
