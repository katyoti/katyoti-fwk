// SPDX-License-Identifier: Apache-2.0

package core

// A run's steps as a progress list's stages: a page follows a
// workflow run live - display's progress-list polls RunStages.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// RunStages answers the run's direct child runs grouped by task.
func (s *Service) RunStages(ctx context.Context, in taskapi.RunStagesRequest) (taskapi.RunStagesResponse, error) {
	if strings.TrimSpace(in.Run) == "" {
		return taskapi.RunStagesResponse{}, fmt.Errorf("run-stages: a run is required")
	}
	all, err := s.Runs(ctx, taskapi.ListRunsRequest{})
	if err != nil {
		return taskapi.RunStagesResponse{}, err
	}
	var parent *taskapi.RunInfo
	var children []taskapi.RunInfo
	for i, r := range all.Runs {
		switch {
		case r.RunID == in.Run:
			parent = &all.Runs[i]
		case r.ParentRunID == in.Run:
			children = append(children, r)
		}
	}
	if parent == nil {
		return taskapi.RunStagesResponse{}, fmt.Errorf("run-stages: no run %s", in.Run)
	}
	return runStages(*parent, children), nil
}

// runStages maps a run's child runs to stages: one per task, in order of its
// first start; done/total over its runs; failed with the first error,
// running while one is active, waiting before any started; its time from
// first start to last end once all ended.
func runStages(parent taskapi.RunInfo, children []taskapi.RunInfo) taskapi.RunStagesResponse {
	sort.SliceStable(children, func(i, j int) bool { return children[i].StartedAt.Before(children[j].StartedAt) })
	byTask := map[string]*taskapi.RunStage{}
	var order []string
	first := map[string]time.Time{}
	last := map[string]time.Time{}
	active := map[string]bool{}
	for _, c := range children {
		st, ok := byTask[c.TaskName]
		if !ok {
			st = &taskapi.RunStage{ID: c.TaskName, Label: c.TaskName}
			byTask[c.TaskName] = st
			order = append(order, c.TaskName)
			first[c.TaskName] = c.StartedAt
		}
		st.Total++
		switch c.Status {
		case taskapi.RunStatusCompleted:
			st.Done++
		case taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
			if st.Message == "" {
				st.Message = firstNonBlank(c.Error, string(c.Status))
			}
			st.State = "failed"
		default:
			active[c.TaskName] = true
		}
		if c.EndedAt != nil && c.EndedAt.After(last[c.TaskName]) {
			last[c.TaskName] = *c.EndedAt
		}
	}
	out := taskapi.RunStagesResponse{Stages: []taskapi.RunStage{}, Done: parent.EndedAt != nil || terminal(parent.Status)}
	for _, id := range order {
		st := byTask[id]
		switch {
		case st.State == "failed":
		case active[id]:
			st.State = "running"
		default:
			st.State = "done"
		}
		if st.State != "running" && !last[id].IsZero() && !first[id].IsZero() {
			st.Ms = last[id].Sub(first[id]).Milliseconds()
		}
		out.Stages = append(out.Stages, *st)
	}
	return out
}

func terminal(st taskapi.RunStatus) bool {
	switch st {
	case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
		return true
	}
	return false
}

func firstNonBlank(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
