// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A workflow run's child runs are its stages, one per task in order of first
// start: done/total, failed with the first error, running while one is
// active.
func TestARunsChildRunsAreItsStages(t *testing.T) {
	t0 := time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC)
	at := func(s int) *time.Time { v := t0.Add(time.Duration(s) * time.Second); return &v }
	run := func(task string, st taskapi.RunStatus, start int, end *time.Time, err string) taskapi.RunInfo {
		return taskapi.RunInfo{TaskName: task, Status: st, ParentRunID: "p", StartedAt: *at(start), EndedAt: end, Error: err}
	}
	parent := taskapi.RunInfo{RunID: "p", Status: taskapi.RunStatusRunning}
	got := runStages(parent, []taskapi.RunInfo{
		run("fill-section", taskapi.RunStatusRunning, 5, nil, ""),
		run("apply", taskapi.RunStatusCompleted, 1, at(4), ""),
		run("fill-section", taskapi.RunStatusCompleted, 5, at(9), ""),
		run("flows", taskapi.RunStatusFailed, 10, at(11), "no source"),
		run("flows", taskapi.RunStatusFailed, 10, at(12), "later"),
	})
	if got.Done {
		t.Fatal("a running run is not done")
	}
	want := []taskapi.RunStage{
		{ID: "apply", Label: "apply", State: "done", Done: 1, Total: 1, Ms: 3000},
		{ID: "fill-section", Label: "fill-section", State: "running", Done: 1, Total: 2},
		{ID: "flows", Label: "flows", State: "failed", Done: 0, Total: 2, Message: "no source", Ms: 2000},
	}
	if len(got.Stages) != len(want) {
		t.Fatalf("stages = %+v", got.Stages)
	}
	for i := range want {
		if got.Stages[i] != want[i] {
			t.Errorf("stage %d = %+v, want %+v", i, got.Stages[i], want[i])
		}
	}
	end := runStages(taskapi.RunInfo{RunID: "p", Status: taskapi.RunStatusCompleted, EndedAt: at(20)}, nil)
	if !end.Done || end.Stages == nil {
		t.Fatalf("an ended run with no child = %+v", end)
	}
}
