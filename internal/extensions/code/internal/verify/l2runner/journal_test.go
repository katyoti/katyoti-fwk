// SPDX-License-Identifier: Apache-2.0

package l2runner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCaptureJournal_HappyPath synthesises a session dir under a tempHome,
// writes a few entries to llm.jsonl and user.jsonl, and asserts the captured
// events are merged + sorted by seq with the correct file labels.
func TestCaptureJournal_HappyPath(t *testing.T) {
	tmpHome := t.TempDir()
	sessionDir := filepath.Join(tmpHome, "sessions", "2026-05-13T15-30-00_demo")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}

	llmLines := []string{
		`{"type":"session_start","seq":1,"ts":"2026-05-13T15:30:00Z","data":{"task":"demo"}}`,
		`{"type":"task_output","seq":3,"ts":"2026-05-13T15:30:02Z","data":{"run_id":"run-1","status":"completed","output":{"summary":"ok"}}}`,
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "llm.jsonl"), []byte(joinLines(llmLines)), 0o644); err != nil {
		t.Fatalf("write llm.jsonl: %v", err)
	}

	userLines := []string{
		`{"type":"prompt_shown","seq":2,"turn":1,"ts":"2026-05-13T15:30:01Z","data":{"prompt":"hi"}}`,
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "user.jsonl"), []byte(joinLines(userLines)), 0o644); err != nil {
		t.Fatalf("write user.jsonl: %v", err)
	}

	events, err := captureJournal(tmpHome)
	if err != nil {
		t.Fatalf("captureJournal: %v", err)
	}
	if got := len(events); got != 3 {
		t.Fatalf("expected 3 events (merged across files), got %d", got)
	}

	// Sequence order across both files: seq 1 (llm), 2 (user), 3 (llm).
	want := []struct {
		seq  int64
		typ  string
		file string
	}{
		{1, "session_start", "llm"},
		{2, "prompt_shown", "user"},
		{3, "task_output", "llm"},
	}
	for i, w := range want {
		if events[i].Seq != w.seq {
			t.Errorf("event[%d].Seq = %d, want %d", i, events[i].Seq, w.seq)
		}
		if events[i].Type != w.typ {
			t.Errorf("event[%d].Type = %q, want %q", i, events[i].Type, w.typ)
		}
		if events[i].File != w.file {
			t.Errorf("event[%d].File = %q, want %q", i, events[i].File, w.file)
		}
	}

	// The task_output event must carry the nested output map intact.
	last := events[2]
	if last.Type != "task_output" {
		t.Fatalf("last event type = %q, want task_output", last.Type)
	}
	output, _ := last.Data["output"].(map[string]any)
	if output == nil || output["summary"] != "ok" {
		t.Errorf("task_output.data.output.summary missing or wrong: %+v", last.Data)
	}
}

// TestCaptureJournal_NoSessionDir verifies that scenarios producing no
// session (e.g. `ktw-agents help`) return (nil, nil) without erroring.
func TestCaptureJournal_NoSessionDir(t *testing.T) {
	tmpHome := t.TempDir()
	events, err := captureJournal(tmpHome)
	if err != nil {
		t.Fatalf("expected nil error when no session dir exists, got %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events, got %d", len(events))
	}
}

// TestCaptureJournal_MalformedLineTolerated proves the parser fails soft on
// individual bad lines — a corrupted entry doesn't take down the whole capture.
func TestCaptureJournal_MalformedLineTolerated(t *testing.T) {
	tmpHome := t.TempDir()
	sessionDir := filepath.Join(tmpHome, "sessions", "bad-session")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	lines := []string{
		`{"type":"session_start","seq":1,"ts":"2026-05-13T15:30:00Z"}`,
		`not valid json at all`,
		`{"type":"task_output","seq":2,"ts":"2026-05-13T15:30:01Z"}`,
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "llm.jsonl"), []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	events, err := captureJournal(tmpHome)
	if err != nil {
		t.Fatalf("captureJournal: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 well-formed events (1 malformed skipped), got %d", len(events))
	}
}

// TestCaptureJournal_RecursiveScan tolerates a non-canonical layout where the
// session dir is nested somewhere under tmpHome (e.g. under a universe-name
// subdir if KTW_DATA isn't set explicitly).
func TestCaptureJournal_RecursiveScan(t *testing.T) {
	tmpHome := t.TempDir()
	deepSessionDir := filepath.Join(tmpHome, "universes", "default", "sessions", "ts1")
	if err := os.MkdirAll(deepSessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deepSessionDir, "llm.jsonl"),
		[]byte(`{"type":"task_output","seq":1,"ts":"2026-05-13T15:30:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	events, err := captureJournal(tmpHome)
	if err != nil {
		t.Fatalf("captureJournal: %v", err)
	}
	if len(events) != 1 || events[0].Type != "task_output" {
		t.Errorf("recursive scan failed: got %+v", events)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}
