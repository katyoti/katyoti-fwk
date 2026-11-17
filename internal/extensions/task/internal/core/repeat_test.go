// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"testing"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// An ended run is started again from the request it was started with — in
// this process (its registry), and in a later one (its kept result): a new
// top-level run, launched by a person. A run that has not ended is refused.
func TestAnEndedRunIsRepeatedFromItsRequest(t *testing.T) {
	ctx := context.Background()
	dir := scratchDir(t)
	first := durableService(t, dir)
	r, err := first.Run(ctx, taskapi.TaskRunRequest{TaskName: "waiter", Group: "g", Input: map[string]any{"n": 7.0}})
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "waiting", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusWaiting })
	if _, err := first.Repeat(ctx, taskapi.RepeatRequest{RunID: r.RunID}); !errors.Is(err, ErrNotEnded) {
		t.Errorf("repeat of a run still waiting: %v; want refused", err)
	}
	if _, err := first.Stop(ctx, taskapi.StopRequest{RunID: r.RunID}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "cancelled", func() bool { return status(t, first, r.RunID) == taskapi.RunStatusCancelled })

	again, err := first.Repeat(ctx, taskapi.RepeatRequest{RunID: r.RunID})
	if err != nil || again.RunID == "" || again.RunID == r.RunID {
		t.Fatalf("repeat: %+v %v; want a new run", again, err)
	}
	info, _, _ := first.reg.snapshot(again.RunID)
	if info.TaskName != "waiter" || info.Origin.Kind != taskapi.OriginPerson || info.ParentRunID != "" {
		t.Errorf("repeated run %+v; want the same task, a person's, top-level", info)
	}
	req, _ := first.requestOf(again.RunID)
	if req.Input["n"] != 7.0 {
		t.Errorf("repeated with input %v; want the ended run's", req.Input)
	}
	_, _ = first.Stop(ctx, taskapi.StopRequest{RunID: again.RunID})

	later := durableService(t, dir) // a later process: its registry knows nothing of it
	more, err := later.Repeat(ctx, taskapi.RepeatRequest{RunID: r.RunID})
	if err != nil || more.RunID == "" {
		t.Fatalf("repeat from the kept result: %+v %v; want a new run", more, err)
	}
	_, _ = later.Stop(ctx, taskapi.StopRequest{RunID: more.RunID})
}
