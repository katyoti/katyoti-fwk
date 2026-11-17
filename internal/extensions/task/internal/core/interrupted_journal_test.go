// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// sessionsDir is a test's own sessions folder (never the user's).
func sessionsDir(t *testing.T) string { return scratchDir(t) }

// scratchDir is a folder a test's runs may still write to as it ends — a run
// still waiting keeps its journal open, a run released by its cleanup writes
// its end — which Windows will not remove at once: it is removed as well as
// can be (t.TempDir fails the test).
func scratchDir(t *testing.T) string {
	d, err := os.MkdirTemp("", "ktw-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

type journalLine struct {
	Type string         `json:"type"`
	Seq  int64          `json:"seq"`
	Data map[string]any `json:"data"`
}

func journalOf(t *testing.T, sessionsDir, sessionID string) []journalLine {
	t.Helper()
	f, err := os.Open(filepath.Join(sessionsDir, sessionID, "llm.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []journalLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l journalLine
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

// A run interrupted by a restart has its end in its session journal, as any
// run: its transition (interrupted, why) and its task_output (failed, why) —
// numbered after what the run wrote before. (Its History ended with no end.)
func TestAnInterruptedRunsJournalSaysHowItEnded(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	stubborn := make(chan struct{})
	t.Cleanup(func() { close(stubborn) })
	first := durableService(t, dir)
	useStubborn(first, stubborn)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "stubborn", Group: "g"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "running", func() bool { return groupLoad(t, first, "g").Running == 1 })
	began(t, first.runs, r.RunID)
	orphan(t, first.runs, r.RunID)

	second := durableService(t, dir)
	second.sessionsDir = first.sessionsDir
	useStubborn(second, stubborn)
	second.RestoreRuns(ctx)

	var lifecycle, output *journalLine
	var last int64
	eventually(t, "its end journalled", func() bool {
		lifecycle, output = nil, nil
		lines := journalOf(t, first.sessionsDir, r.SessionID)
		for i := range lines {
			l := &lines[i]
			if l.Seq <= last {
				t.Fatalf("seq %d after %d: the journal is numbered again", l.Seq, last)
			}
			last = l.Seq
			switch {
			case l.Type == "run_lifecycle" && l.Data["transition"] == "interrupted":
				lifecycle = l
			case l.Type == "task_output" && l.Data["status"] == "failed":
				output = l
			}
		}
		last = 0
		return lifecycle != nil && output != nil
	})
	if reason, _ := lifecycle.Data["reason"].(string); !strings.Contains(reason, "restart") {
		t.Errorf("interrupted because %q; want why", reason)
	}
	if msg, _ := output.Data["error"].(string); msg == "" {
		t.Errorf("task_output %+v; want its error", output.Data)
	}
	if lines := journalOf(t, first.sessionsDir, r.SessionID); lines[0].Type != "task_started" {
		t.Errorf("first line %q; want the run's own start before its end", lines[0].Type)
	}
}

// A run's journal holds its transitions, in order — its History says what
// happened to it, not only what it did.
func TestARunsJournalHoldsItsTransitions(t *testing.T) {
	svc := groupsService(t, taskapi.TaskGroupsConfig{})
	r, err := svc.Run(context.Background(), taskapi.TaskRunRequest{TaskName: "waiter"})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, svc, r.RunID) == taskapi.RunStatusWaiting })
	wake(t, svc, r.RunID)
	want := "started waiting woken completed"
	eventually(t, "journalled", func() bool {
		var got []string
		for _, l := range journalOf(t, svc.sessionsDir, r.SessionID) {
			if l.Type == "run_lifecycle" {
				got = append(got, l.Data["transition"].(string))
			}
		}
		return strings.Join(got, " ") == want
	})
}
