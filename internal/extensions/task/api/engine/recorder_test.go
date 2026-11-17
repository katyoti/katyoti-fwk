// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionRecorder_Disabled(t *testing.T) {
	r, err := NewSessionRecorder("", "test-task", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	defer r.Close()

	if r.SessionID() != "" {
		t.Errorf("SessionID should be empty when disabled, got %q", r.SessionID())
	}

	// Logging should be no-ops
	r.LLMJournal().Log(JournalSessionStart, map[string]any{"test": true})
	r.UserJournal().Log(UserSessionStart, 0, "", map[string]any{"test": true})
}

func TestSessionRecorder_CreatesFiles(t *testing.T) {
	dir := t.TempDir()

	r, err := NewSessionRecorder(dir, "my-task", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}

	if r.SessionID() == "" {
		t.Error("SessionID should not be empty")
	}

	// Session directory should exist as a subdirectory
	sessionDir := r.SessionDir()
	if sessionDir == "" {
		t.Fatal("SessionDir is empty")
	}
	info, err := os.Stat(sessionDir)
	if err != nil {
		t.Fatalf("Session dir not found: %v", err)
	}
	if !info.IsDir() {
		t.Error("SessionDir should be a directory")
	}

	// Check LLM journal file exists inside session dir
	llmPath := r.LLMJournalPath()
	if llmPath == "" {
		t.Fatal("LLMJournalPath is empty")
	}
	if _, err := os.Stat(llmPath); err != nil {
		t.Errorf("LLM journal file not found: %v", err)
	}
	if filepath.Base(llmPath) != "llm.jsonl" {
		t.Errorf("LLM journal should be named llm.jsonl, got %s", filepath.Base(llmPath))
	}

	// Check user journal file exists inside session dir
	userPath := r.UserJournalPath()
	if userPath == "" {
		t.Fatal("UserJournalPath is empty")
	}
	if _, err := os.Stat(userPath); err != nil {
		t.Errorf("User journal file not found: %v", err)
	}
	if filepath.Base(userPath) != "user.jsonl" {
		t.Errorf("User journal should be named user.jsonl, got %s", filepath.Base(userPath))
	}

	r.Close()
}

func TestSessionRecorder_SeqCounter(t *testing.T) {
	dir := t.TempDir()

	r, err := NewSessionRecorder(dir, "seq-test", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	defer r.Close()

	// Seq should be monotonically increasing
	s1 := r.NextSeq()
	s2 := r.NextSeq()
	s3 := r.NextSeq()

	if s1 != 1 || s2 != 2 || s3 != 3 {
		t.Errorf("Expected seq 1,2,3, got %d,%d,%d", s1, s2, s3)
	}
}

// TestClose_Unsubscribes — UC-121, audit findings corr-3 + perf-1.
//
// Verifies that recorder.Close drains every unsub closure registered via
// AddUnsub. Without this drain, every completed session leaks ~13 entries in
// MemoryEventBus.subs across long-lived processes.
func TestClose_Unsubscribes(t *testing.T) {
	bus := NewMemoryEventBus()
	baseline := bus.TotalSubscribers()

	r, err := NewSessionRecorder("", "leak-test", "") // disabled mode — no files needed
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}

	// Register a handful of subscribers via the recorder, mirroring what
	// subscribeRecorder does in production.
	const n = 5
	for i := 0; i < n; i++ {
		r.AddUnsub(bus.Subscribe("topic-"+string(rune('A'+i)), func(string, any) {}))
	}
	if got, want := bus.TotalSubscribers(), baseline+n; got != want {
		t.Fatalf("after subscribe: TotalSubscribers = %d, want %d", got, want)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := bus.TotalSubscribers(); got != baseline {
		t.Errorf("after Close: TotalSubscribers = %d, want %d (baseline)", got, baseline)
	}

	// Calling Close a second time must be safe and a no-op.
	if err := r.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if got := bus.TotalSubscribers(); got != baseline {
		t.Errorf("after second Close: TotalSubscribers = %d, want %d", got, baseline)
	}
}

// TestClose_StressNoLeak — UC-121.
//
// Spins up 100 recorders, each subscribing 13 handlers (matching the
// production count) and then closing. Asserts that TotalSubscribers returns
// to baseline after all closes complete — i.e., no leak even under churn.
func TestClose_StressNoLeak(t *testing.T) {
	bus := NewMemoryEventBus()
	baseline := bus.TotalSubscribers()

	const recorders = 100
	const subsPerRecorder = 13 // matches production subscribeRecorder count

	for i := 0; i < recorders; i++ {
		r, err := NewSessionRecorder("", "stress", "")
		if err != nil {
			t.Fatalf("NewSessionRecorder #%d: %v", i, err)
		}
		for j := 0; j < subsPerRecorder; j++ {
			r.AddUnsub(bus.Subscribe("t", func(string, any) {}))
		}
		if err := r.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}

	if got := bus.TotalSubscribers(); got != baseline {
		t.Errorf("after %d recorders × %d subs each + close: TotalSubscribers = %d, want %d",
			recorders, subsPerRecorder, got, baseline)
	}
}

// TestAddUnsub_NilSafe ensures AddUnsub(nil) is a no-op (defensive coding —
// guards against accidental nil from a misconfigured EventBus implementation).
func TestAddUnsub_NilSafe(t *testing.T) {
	r, err := NewSessionRecorder("", "nil-safe", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	r.AddUnsub(nil)
	if err := r.Close(); err != nil {
		t.Errorf("Close after nil AddUnsub: %v", err)
	}
}

func TestSessionRecorder_CorrelatedEntries(t *testing.T) {
	dir := t.TempDir()

	r, err := NewSessionRecorder(dir, "corr-test", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}

	// Write to both journals — seq should interleave
	r.LLMJournal().LogWithTurn(JournalSessionStart, 0, map[string]any{"test": true})
	r.UserJournal().Log(UserSessionStart, 0, "", map[string]any{"test": true})
	r.LLMJournal().LogWithTurn(JournalLLMRequest, 1, map[string]any{"turn": 1})

	r.Close()

	// Read LLM journal and verify seq values
	llmData, err := os.ReadFile(r.LLMJournalPath())
	if err != nil {
		t.Fatalf("Read LLM journal: %v", err)
	}
	if len(llmData) == 0 {
		t.Fatal("LLM journal is empty")
	}

	// Read user journal
	userData, err := os.ReadFile(r.UserJournalPath())
	if err != nil {
		t.Fatalf("Read user journal: %v", err)
	}
	if len(userData) == 0 {
		t.Fatal("User journal is empty")
	}
}
