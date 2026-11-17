// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"os"
	"path/filepath"
	"testing"
)

// : each clean-machine run keeps its results - a new run moves
// the previous run's results/ (results.json, logs, screenshots) into
// history/<its start time>/ and makes results/ afresh, the last run's, where
// the checks read it.
func TestEachRunKeepsItsResults(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "clean-machine")
	runner := filepath.Join(t.TempDir(), "ktw-compose.exe")
	if err := os.WriteFile(runner, []byte("runner"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := SandboxRun{Dir: dir, Market: t.TempDir(), Runner: runner, Plan: Plan{Case: "proof"}}
	finish := func(started string) {
		if err := writeResults(run.Results(), Results{Case: "proof", StartedAt: started, Done: true}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(run.Results(), "gui.png"), []byte("png"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := run.Prepare(); err != nil {
		t.Fatal(err)
	}
	finish("2026-10-16T18:00:00Z")
	if _, err := run.Prepare(); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(dir, HistoryDir, "20261016-180000")
	for _, f := range []string{ResultsFile, "gui.png"} {
		if _, err := os.Stat(filepath.Join(first, f)); err != nil {
			t.Errorf("the first run's %s is not kept: %v", f, err)
		}
	}
	if entries, _ := os.ReadDir(run.Results()); len(entries) != 0 {
		t.Errorf("results/ is not made afresh: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(run.Tools(), RunnerExe)); err != nil {
		t.Errorf("the tools are not there for the new run: %v", err)
	}

	// A third run keeps both earlier ones.
	finish("2026-10-16T19:30:00Z")
	if _, err := run.Prepare(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, HistoryDir))
	if len(entries) != 2 {
		t.Fatalf("history holds %d runs, want 2", len(entries))
	}

	// Nothing to keep before the first run.
	if kept, err := KeepPreviousResults(filepath.Join(t.TempDir(), "none")); err != nil || kept != "" {
		t.Errorf("no previous run: %q %v", kept, err)
	}
}
