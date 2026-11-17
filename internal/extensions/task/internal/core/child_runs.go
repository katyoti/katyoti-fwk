// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"

	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// EVERY CHILD IS A RUN — a child a parent starts at a call site (a workflow's
// node, an agent's tool call) is a run like any other: its group's slot, its
// manifest, its place in Runs. Its id is stable (engine.ChildRunID): the same
// parent and site give the same run, so a parent replaying the step that
// started it — after a restart — finds it instead of starting it again.

// spawnSite is where a parent started a child.
type spawnSite struct {
	// Name is the child's name at its parent (a workflow step's name).
	Name string `json:"name"`
	// Site names the call site (node id, iteration, attempt; a tool call).
	Site string `json:"site"`
	// By is the kind of parent: "workflow" or "agent".
	By string `json:"by"`
	// Mode is its delegation mode at the site (its start reports the mode's
	// dimensions).
	Mode string `json:"mode,omitempty"`
}

// runAs starts in under runID — or, when a run with that id exists already
// (live, left on disk by a process that died, or ended), answers it with
// existing true and starts nothing.
func (s *Service) runAs(ctx context.Context, in taskapi.TaskRunRequest, runID string, site *spawnSite) (resp taskapi.TaskRunResponse, existing bool, err error) {
	if !engine.IsRunID(runID) {
		return resp, false, fmt.Errorf("run id %q: not a run id", runID)
	}
	s.startingMu.Lock()
	defer s.startingMu.Unlock()
	if info, _, ok := s.reg.snapshot(runID); ok {
		return responseOf(info), true, nil
	}
	if s.runs != nil {
		if _, err := os.Stat(s.runs.manifestPath(runID)); err == nil {
			return taskapi.TaskRunResponse{RunID: runID, TaskName: in.TaskName, ParentRunID: in.ParentRunID, Status: taskapi.RunStatusPending}, true, nil
		}
		if res, err := s.runs.loadResult(runID); err == nil {
			return taskapi.TaskRunResponse{RunID: runID, TaskName: in.TaskName, ParentRunID: in.ParentRunID, Status: taskapi.RunStatus(res.Status)}, true, nil
		}
	}
	resp, err = s.start(ctx, in, runID, site)
	return resp, false, err
}

// liveParent reports whether parentRunID names a run of this process that has
// not ended.
func (s *Service) liveParent(parentRunID string) bool {
	if parentRunID == "" {
		return false
	}
	e, ok := s.reg.get(parentRunID)
	if !ok {
		return false
	}
	select {
	case <-e.done:
		return false
	default:
		return true
	}
}

func responseOf(info taskapi.RunInfo) taskapi.TaskRunResponse {
	return taskapi.TaskRunResponse{
		RunID: info.RunID, TaskName: info.TaskName, MachineType: info.MachineType, Status: info.Status,
		ParentRunID: info.ParentRunID, SessionID: info.SessionID, StartedAt: info.StartedAt,
	}
}

// stopChildren stops every run of this process whose parent is parentRunID,
// and theirs (Stop recurses).
func (s *Service) stopChildren(parentRunID string) {
	for _, id := range s.reg.ids() {
		info, _, ok := s.reg.snapshot(id)
		if !ok || info.ParentRunID != parentRunID {
			continue
		}
		_, _ = s.Stop(context.Background(), taskapi.StopRequest{RunID: id})
	}
}

// serviceSpawner starts a machine's children as runs of the task service
// (engine.Spawner): each child takes its group's slot, is saved, listed, and
// keeps its stable id — started again from its site, it is found.
type serviceSpawner struct{ s *Service }

func (sp serviceSpawner) Spawn(ctx context.Context, spec engine.ChildSpec) (engine.ChildHandle, error) {
	in := taskapi.TaskRunRequest{
		Input:          spec.Input,
		ParentRunID:    spec.ParentID,
		DryRunMode:     spec.DryRunMode,
		CurrentNode:    spec.CurrentNode,
		MaxDurationS:   spec.MaxDurationS,
		ConfigOverride: childOverride(spec),
		Origin:         &taskapi.RunOrigin{Kind: taskapi.OriginWorkflow, ID: spec.ParentID},
	}
	if name, ok := spec.Config["_taskName"].(string); ok && spec.Kind == "task" {
		in.TaskName = name
	} else {
		in.TaskDef = map[string]any{"name": spec.Name, "type": spec.Kind, "body": spec.Body, "config": spec.Config}
	}
	id := engine.GenerateRunID()
	if spec.Site != "" && spec.ParentID != "" {
		id = engine.ChildRunID(spec.ParentID, spec.Site)
	}
	resp, _, err := sp.s.runAs(ctx, in, id, &spawnSite{Name: spec.Name, Site: spec.Site, By: "workflow", Mode: spec.Mode})
	if err != nil {
		return engine.ChildHandle{}, fmt.Errorf("spawn %s: %w", spec.Name, err)
	}
	return engine.ChildHandle{RunID: resp.RunID, Name: spec.Name, Kind: spec.Kind, Status: engine.ChildRunning}, nil
}

// Cancel stops the child (marked stopped: a restart does not revive it).
func (sp serviceSpawner) Cancel(runID, reason string) error {
	_, err := sp.s.Stop(context.Background(), taskapi.StopRequest{RunID: runID})
	return err
}

// Result is a completed child's output.
func (sp serviceSpawner) Result(runID string) (any, bool) {
	o, ended := sp.Outcome(runID)
	if !ended || o.Status != engine.ChildDone {
		return nil, false
	}
	return o.Output, true
}

// Outcome is how the child ended: from the registry, else from its kept
// result (after a restart, past its retention). A run has ended once its end
// is set in the registry — before its end is told, so a parent woken by it
// reads it ended (its goroutine may not have returned yet).
func (sp serviceSpawner) Outcome(runID string) (engine.ChildOutcome, bool) {
	s := sp.s
	if e, ok := s.reg.get(runID); ok {
		var res taskapi.ResultResponse
		ended := false
		s.reg.mutate(runID, func(e *runEntry) {
			if e.terminalAt.IsZero() {
				return
			}
			ended = true
			info := e.view()
			res.Status, res.Error = string(info.Status), info.Error
			if e.result != nil {
				res.Output = e.result.Output
			}
		})
		if !ended {
			select {
			case <-e.killed:
			default:
				return engine.ChildOutcome{}, false
			}
		}
		if s.runs != nil {
			if saved, err := s.runs.loadResult(runID); err == nil {
				res = saved.ResultResponse
			}
		}
		if res.Status == "" { // killed, no store: from the registry
			info, _, _ := s.reg.snapshot(runID)
			res.Status, res.Error = string(info.Status), info.Error
		}
		return outcomeOf(res), true
	}
	if s.runs != nil {
		if saved, err := s.runs.loadResult(runID); err == nil {
			return outcomeOf(saved.ResultResponse), true
		}
	}
	return engine.ChildOutcome{}, false
}

func outcomeOf(res taskapi.ResultResponse) engine.ChildOutcome {
	switch taskapi.RunStatus(res.Status) {
	case taskapi.RunStatusCompleted:
		return engine.ChildOutcome{Status: engine.ChildDone, Output: res.Output}
	case taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
		why := res.Error
		if why == "" {
			why = "it was " + res.Status
		}
		return engine.ChildOutcome{Status: engine.ChildCancelled, Error: why}
	default:
		why := res.Error
		if why == "" {
			why = "it " + res.Status
		}
		return engine.ChildOutcome{Status: engine.ChildFailed, Error: why}
	}
}

var _ engine.Spawner = serviceSpawner{}

// tellChildStarted publishes a child's start on its parent's lifecycle topics
// (the terminal, the session recorder, the delegation graph).
func (s *Service) tellChildStarted(runID string, in taskapi.TaskRunRequest, site *spawnSite, kind string) {
	if s.channel == nil {
		return
	}
	evt := &engine.ChildStartedEvent{ChildName: site.Name, ChildKind: kind, RunID: runID, ParentRunID: in.ParentRunID, Mode: site.Mode}
	evt.Correlation = engine.Correlation{RunID: runID, ParentRunID: in.ParentRunID}
	s.channel.Bus().Publish(engine.ChildStartedTopic(site.Name), evt)
	s.channel.Bus().Publish(engine.ChildStartedAuditTopic(), evt)
	// Its start in its parent's journal (stamped with the parent's run).
	st := &engine.TaskStartedEvent{RunID: runID, TaskName: site.Name, TaskType: kind, Input: in.Input, Mode: site.Mode, Dimensions: dimensionsForMode(site.Mode)}
	st.Correlation = evt.Correlation
	st.StampEvent(engine.EventTaskStarted, in.ParentRunID)
	s.channel.Bus().Publish(engine.EventTaskStarted, st)
}

// tellChildEnded publishes a child's end on its parent's lifecycle topics, and
// its task.output stamped with its parent's run (the parent's recorder keeps
// it). Observability only: a parent settles from Outcome.
func (s *Service) tellChildEnded(runID string, in taskapi.TaskRunRequest, site *spawnSite, status taskapi.RunStatus, result *engine.RunResult, err error) {
	if s.channel == nil {
		return
	}
	corr := engine.Correlation{RunID: runID, ParentRunID: in.ParentRunID}
	var output any
	if result != nil {
		output = result.Output
	}
	if status == taskapi.RunStatusCompleted {
		evt := &engine.ChildDoneEvent{ChildName: site.Name, RunID: runID, Output: output}
		evt.Correlation = corr
		s.channel.Bus().Publish(engine.ChildDoneTopic(site.Name), evt)
		s.channel.Bus().Publish(engine.ChildDoneAuditTopic(), evt)
	} else {
		why := "it " + string(status)
		if err != nil {
			why = err.Error()
		}
		evt := &engine.ChildFailedEvent{ChildName: site.Name, RunID: runID, Error: why}
		evt.Correlation = corr
		s.channel.Bus().Publish(engine.ChildFailedTopic(site.Name), evt)
		s.channel.Bus().Publish(engine.ChildFailedAuditTopic(), evt)
	}
	outMap, _ := output.(map[string]any)
	if outMap == nil && output != nil {
		outMap = map[string]any{"result": output}
	}
	tEvt := &engine.TaskOutputEvent{RunID: runID, TaskName: site.Name, Status: string(status), Output: outMap}
	tEvt.Correlation = corr
	tEvt.StampEvent(engine.EventTaskOutput, in.ParentRunID)
	s.channel.Bus().Publish(engine.EventTaskOutput, tEvt)
}

// releaseEnded drops what was kept for a run while it ran — its suspension, its
// children's undrained notifications. Called once it has ended.
func (s *Service) releaseEnded(runID string) {
	if s.suspend != nil {
		s.suspend.forget(runID)
	}
	if bg, ok := s.bgMgr.(*bgManager); ok {
		bg.dropNotifications(runID)
	}
}

// endedHere reports whether runID names a run of this process that has ended.
func (s *Service) endedHere(runID string) bool {
	e, ok := s.reg.get(runID)
	if !ok {
		return false
	}
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}
