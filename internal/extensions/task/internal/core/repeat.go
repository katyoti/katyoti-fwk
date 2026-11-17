// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// ErrNotEnded: a run that has not ended is not started again.
var ErrNotEnded = errors.New("the run has not ended")

// Repeat starts an ended run again — an interrupted, failed, stopped or
// completed one — from the request it was started with, kept with its result:
// a new top-level run under a new id, launched by whoever asks (a person by
// default). Nothing of the ended run is taken up: its state is not resumed.
func (s *Service) Repeat(ctx context.Context, in taskapi.RepeatRequest) (taskapi.TaskRunResponse, error) {
	if in.RunID == "" {
		return taskapi.TaskRunResponse{}, fmt.Errorf("repeat: name the run (runId)")
	}
	req, err := s.requestOf(in.RunID)
	if err != nil {
		return taskapi.TaskRunResponse{}, fmt.Errorf("repeat %s: %w", in.RunID, err)
	}
	// A run of its own: not a child, not a background task, not a session's.
	req.ParentRunID, req.BackgroundID, req.SessionID = "", "", ""
	req.Origin = in.Origin
	if req.Origin == nil {
		req.Origin = &taskapi.RunOrigin{Kind: taskapi.OriginPerson}
	}
	return s.Run(ctx, req)
}

// requestOf is what an ended run was started with: from the registry, else
// from its kept result.
func (s *Service) requestOf(runID string) (taskapi.TaskRunRequest, error) {
	var req taskapi.TaskRunRequest
	known, ended := false, false
	s.reg.mutate(runID, func(e *runEntry) {
		known, ended, req = true, !e.terminalAt.IsZero(), e.request
	})
	if known {
		if !ended {
			return req, ErrNotEnded
		}
		return req, nil
	}
	if s.runs != nil {
		if saved, err := s.runs.loadResult(runID); err == nil {
			if saved.Request == nil {
				return req, errors.New("its request was not kept")
			}
			return *saved.Request, nil
		}
	}
	return req, fmt.Errorf("run %q not found", runID)
}
