// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSessionRecorder_RestrictiveModes verifies the journals and session
// directory are created with restrictive POSIX permissions so a multi-user
// Linux host doesn't expose LLM transcripts to other local users (R10 /
// audit medium-finding-3). Skipped on Windows where POSIX mode bits are
// not enforced — the ACL is the effective gate there.
func TestSessionRecorder_RestrictiveModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not enforced on Windows; ACL is the effective gate")
	}

	sessionsDir := t.TempDir()
	rec, err := NewSessionRecorder(sessionsDir, "test-task", "")
	if err != nil {
		t.Fatalf("NewSessionRecorder: %v", err)
	}
	defer rec.Close()

	// Session directory: 0o700 (owner-only).
	info, err := os.Stat(rec.SessionDir())
	if err != nil {
		t.Fatalf("stat session dir: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o700 {
		t.Errorf("session dir perm = 0o%o, want 0o700", mode)
	}

	// Each journal file: 0o600 (owner-only).
	for _, p := range []string{rec.LLMJournalPath(), rec.UserJournalPath()} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if mode := fi.Mode().Perm(); mode != 0o600 {
			t.Errorf("%s perm = 0o%o, want 0o600", filepath.Base(p), mode)
		}
	}
}

// TestSessionSummary_SaveJSON_RestrictiveMode verifies that the summary file
// SaveJSON writes also lands at 0o600 — same sensitivity bracket as the
// journals (R10).
func TestSessionSummary_SaveJSON_RestrictiveMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX mode bits not enforced on Windows; ACL is the effective gate")
	}

	dir := t.TempDir()
	path := SummaryPath(dir)
	if err := (&SessionSummary{}).SaveJSON(path); err != nil {
		t.Fatalf("SaveJSON: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("summary.json perm = 0o%o, want 0o600", mode)
	}
}
