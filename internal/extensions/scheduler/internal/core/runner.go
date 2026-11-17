// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// THE INTERNAL WORKER — one more worker on the contract: a to-do run is pulled
// for the work-todo task, which reads it, gets its object's context, follows
// its skill and reports it — exactly what an outside session does. A run that
// ends without a report is reported failed for it.

// Run runs a ready agent to-do with the internal worker, whatever its
// module's mode (off excepted: nothing of it is handed out).
func (s *Service) Run(ctx context.Context, in api.RunInput) (api.TodoOutput, error) {
	if err := checkActor(in.By); err != nil {
		return api.TodoOutput{}, err
	}
	// Pulled as the work-todo worker: a person's to-do is refused there.
	pulled, err := s.Pull(ctx, api.PullInput{ID: in.ID, LeaseSeconds: api.RunLeaseSeconds, By: api.Worker})
	if err != nil {
		return api.TodoOutput{}, err
	}
	if pulled.Todo == nil {
		return api.TodoOutput{}, invalid("Run: %s", firstOf(pulled))
	}
	t := *pulled.Todo
	// Its owner runs it, when it contributed a runner: no work-todo task.
	if r := runnerOf(ctx, t.Owner); r != nil {
		return s.runByOwner(ctx, t, r)
	}
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		if _, rerr := s.Release(ctx, api.ReleaseInput{ID: t.ID, Note: "no task service to run it", By: api.Worker}); rerr != nil {
			return api.TodoOutput{}, fmt.Errorf("Run: the task service: %v; and giving it back: %w", err, rerr)
		}
		return api.TodoOutput{}, fmt.Errorf("Run: the task service: %w", err)
	}
	group := t.Group
	if group == "" {
		group = api.DefaultGroup
	}
	origin := &taskapi.RunOrigin{Kind: taskapi.OriginAgent, ID: in.By.ID}
	if in.By.Kind == "person" {
		origin.Kind = taskapi.OriginPerson
	}
	run, err := tasks.Run(ctx, taskapi.TaskRunRequest{
		TaskName: api.WorkTodoTask,
		Input:    map[string]any{"todo": t.ID},
		Group:    group,
		Priority: len(priorities) - 1 - rank(t.Priority),
		Origin:   origin,
		Target:   t.Target,
	})
	if err != nil {
		if _, rerr := s.Release(ctx, api.ReleaseInput{ID: t.ID, Note: "its run did not start: " + err.Error(), By: api.Worker}); rerr != nil {
			return api.TodoOutput{}, fmt.Errorf("Run: %v; and giving it back: %w", err, rerr)
		}
		return api.TodoOutput{}, fmt.Errorf("Run: %s did not start: %w", t.ID, err)
	}
	out, err := s.Renew(ctx, api.RenewInput{ID: t.ID, LeaseSeconds: api.RunLeaseSeconds, By: api.Worker, Run: run.RunID, Session: run.SessionID})
	if err != nil {
		return api.TodoOutput{}, err
	}
	out.Note = "run " + run.RunID
	return out, nil
}

// runnerOf is the runner its owner contributed, if any.
func runnerOf(ctx context.Context, owner string) *api.Runner {
	for _, r := range application.UseExtensions[api.Runner](ctx) {
		if r.Owner == owner && r.Run != nil {
			return &r
		}
	}
	return nil
}

// runByOwner hands a pulled to-do to its owner's runner: the run it names is
// recorded on the claim (watched like a work-todo run); none, it has reported.
func (s *Service) runByOwner(ctx context.Context, t api.Todo, r *api.Runner) (api.TodoOutput, error) {
	run, session, err := r.Run(ctx, t)
	if err != nil {
		if _, rerr := s.Release(ctx, api.ReleaseInput{ID: t.ID, Note: "its owner's runner did not start it: " + err.Error(), By: api.Worker}); rerr != nil {
			return api.TodoOutput{}, fmt.Errorf("Run: %v; and giving it back: %w", err, rerr)
		}
		return api.TodoOutput{}, fmt.Errorf("Run: %s did not start: %w", t.ID, err)
	}
	if run == "" {
		out, err := s.Get(ctx, api.GetInput{ID: t.ID})
		out.Note = "run by its owner " + t.Owner
		return out, err
	}
	out, err := s.Renew(ctx, api.RenewInput{ID: t.ID, LeaseSeconds: api.RunLeaseSeconds, By: api.Worker, Run: run, Session: session})
	if err != nil {
		return api.TodoOutput{}, err
	}
	out.Note = "run " + run + " by its owner " + t.Owner
	return out, nil
}

// firstOf says why a pull gave nothing.
func firstOf(p api.PullOutput) string {
	if len(p.Waiting) > 0 {
		return p.Waiting[0]
	}
	return p.Nothing
}

// Tick runs one step of the internal scheduler.
func (s *Service) Tick(ctx context.Context, _ api.TickInput) (api.TickReport, error) {
	var rep api.TickReport
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return rep, fmt.Errorf("Tick: the task service: %w", err)
	}
	s.giveBack(ctx, tasks, &rep)
	s.recollect(ctx, &rep)
	s.catchUp(ctx, &rep)
	p, err := s.Prune(ctx, api.PruneInput{})
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	}
	rep.Pruned = p.Removed
	(&Waits{store: s.store}).remind(ctx, &rep)
	if cfg, _ := application.UseConfig[api.SchedulerConfig](ctx); !cfg.Enabled() {
		rep.Waiting = append(rep.Waiting, "the internal scheduler is off (Scheduler.yaml)")
		return rep, nil
	}
	s.startReady(ctx, tasks, &rep)
	return rep, nil
}

// recollectAfter is how long a reported to-do is left to the collect its report
// started — in this process or another —: past it, that collect was cut (its
// process stopped) and a tick hands it to its owner again.
const recollectAfter = 10 * time.Minute

// recollectTries bounds the collects tried again on one to-do: past it, it is
// left reported (said once) until the process restarts.
const recollectTries = 3

var recollectFailed sync.Map // to-do id -> tries failed

// recollect hands to their owners' collectors the to-dos reported long enough
// ago that the collect their report started cannot still be running.
func (s *Service) recollect(ctx context.Context, rep *api.TickReport) {
	s.collectReported(ctx, time.Now().Add(-recollectAfter), rep)
}

// CollectLeftReported hands to their owners' collectors the to-dos reported
// before the serving process started: the collect their report started died
// with the process before it — what serving does once, every module manual or
// not.
func CollectLeftReported(ctx context.Context, started time.Time) []string {
	var rep api.TickReport
	if svc, err := ServiceIn(ctx); err != nil {
		rep.Errors = append(rep.Errors, err.Error())
	} else {
		svc.collectReported(ctx, started, &rep)
	}
	for _, e := range rep.Errors {
		slog.Warn("scheduler: a to-do left reported was not collected", "error", e)
	}
	return rep.Collected
}

// collectReported collects again each to-do reported before cutoff — never one
// whose collect runs in this process, never one past its tries.
func (s *Service) collectReported(ctx context.Context, cutoff time.Time, rep *api.TickReport) {
	reported, err := s.List(ctx, api.ListInput{State: api.StateReported})
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	for _, t := range reported.Todos {
		if t.Result == nil {
			continue
		}
		if at, err := time.Parse(time.RFC3339Nano, t.Result.At); err == nil && at.After(cutoff) {
			continue
		}
		tries, _ := recollectFailed.Load(t.ID)
		if n, _ := tries.(int); n >= recollectTries {
			continue
		}
		note, err := collect(ctx, t)
		if err != nil {
			n, _ := tries.(int)
			recollectFailed.Store(t.ID, n+1)
			msg := fmt.Sprintf("collect %s again: %v", t.ID, err)
			if n+1 == recollectTries {
				msg += fmt.Sprintf(" — tried %d times: left reported until a restart", recollectTries)
			}
			rep.Errors = append(rep.Errors, msg)
			continue
		}
		if strings.HasPrefix(note, "collected") {
			recollectFailed.Delete(t.ID)
			rep.Collected = append(rep.Collected, t.ID)
		}
	}
}

// giveBack reports failed the to-dos whose internal run ended without a report.
func (s *Service) giveBack(ctx context.Context, tasks taskapi.TaskService, rep *api.TickReport) {
	held, err := s.List(ctx, api.ListInput{State: api.StateClaimed})
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	for _, t := range held.Todos {
		if t.Claim == nil || t.Claim.Run == "" || t.Claim.By != api.Worker {
			continue
		}
		st, err := tasks.Status(ctx, taskapi.StatusRequest{RunID: t.Claim.Run})
		if err != nil {
			continue
		}
		switch st.Status {
		case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled, taskapi.RunStatusKilled:
		default:
			continue
		}
		why := fmt.Sprintf("its run %s ended %s without a report", t.Claim.Run, st.Status)
		if _, err := s.Report(ctx, api.ReportInput{ID: t.ID, Result: api.Result{Status: api.StatusFail, Summary: why, By: api.Worker}}); err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("give back %s: %v", t.ID, err))
			continue
		}
		rep.GivenBack = append(rep.GivenBack, t.ID+": "+why)
	}
}

// startReady runs the takeable auto to-dos of the auto modules, each within
// its task group's window.
func (s *Service) startReady(ctx context.Context, tasks taskapi.TaskService, rep *api.TickReport) {
	windows := map[string]int{} // a group not declared has no window: -1
	if gs, err := tasks.Groups(ctx, taskapi.GroupsRequest{}); err == nil {
		for _, g := range gs.Groups {
			windows[g.Name] = max(g.MaxRunning, 1) - g.Queued
			if !g.On {
				windows[g.Name] = 0
			}
		}
	}
	q, err := s.queue(ctx, api.Filter{For: api.ForAgent}, true, "", "")
	if err != nil {
		rep.Errors = append(rep.Errors, err.Error())
		return
	}
	for _, e := range q {
		if !e.Takeable {
			continue
		}
		group := e.Todo.Group
		if group == "" {
			group = api.DefaultGroup
		}
		w, declared := windows[group]
		if declared && w <= 0 {
			rep.Waiting = append(rep.Waiting, e.Todo.ID+": group "+group+" is paused or has its window queued")
			continue
		}
		out, err := s.Run(ctx, api.RunInput{ID: e.Todo.ID, By: api.Worker})
		if err != nil {
			rep.Errors = append(rep.Errors, fmt.Sprintf("run %s: %v", e.Todo.ID, err))
			continue
		}
		rep.Started = append(rep.Started, e.Todo.ID+" ("+out.Note+")")
		if declared {
			windows[group] = w - 1
		}
	}
}

// wake tells the internal scheduler something moved in the queues.
func wake(ctx context.Context) {
	if bus, err := application.UseService[eventbus.EventBus](ctx); err == nil && bus != nil {
		_ = bus.Publish(engine.PlanChangedTopic(), "todos")
	}
}

// running reports whether the internal scheduler has anything to run: it is on
// and a module is in mode auto — read now, so a run already going ends at its
// next step when none is left.
func running(ctx context.Context) bool {
	if cfg, _ := application.UseConfig[api.SchedulerConfig](ctx); !cfg.Enabled() {
		return false
	}
	svc, err := ServiceIn(ctx)
	if err != nil {
		return false
	}
	modes, err := svc.Modes(ctx, api.ModesInput{})
	if err != nil {
		return false
	}
	for _, m := range modes.Modules {
		if m.Mode == api.ModeAuto {
			return true
		}
	}
	return false
}

// StartScheduler starts the internal scheduler's run when it has something to
// run and none runs yet — what the serving process does once it serves, and
// what setting a module auto does.
func StartScheduler(ctx context.Context) (string, error) {
	if !running(ctx) {
		return "", nil
	}
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		return "", err
	}
	if active(ctx, tasks) != "" {
		return "", nil
	}
	cfg, _ := application.UseConfig[api.SchedulerConfig](ctx)
	run, err := tasks.Run(ctx, taskapi.TaskRunRequest{
		TaskDef: map[string]any{
			"name": api.SchedulerTask, "type": api.SchedulerTask,
			"description": "The internal scheduler: runs the auto to-dos of the auto modules",
			"config":      map[string]any{"interval_seconds": int(cfg.Interval().Seconds())},
		},
		Group:  api.DefaultGroup,
		Origin: &taskapi.RunOrigin{Kind: taskapi.OriginSystem, ID: api.SchedulerTask},
	})
	if err != nil {
		return "", err
	}
	return run.RunID, nil
}

// active is the internal scheduler's run already going, other than this one.
func active(ctx context.Context, tasks taskapi.TaskService) string {
	self := engine.RunIDFromContext(ctx)
	runs, err := tasks.Runs(ctx, taskapi.ListRunsRequest{Status: "active"})
	if err != nil {
		return ""
	}
	for _, r := range runs.Runs {
		if r.TaskName == api.SchedulerTask && r.RunID != self {
			return r.RunID
		}
	}
	return ""
}

// startOnAuto starts the internal scheduler once a module is set auto.
func startOnAuto(ctx context.Context, mode string) {
	if mode != api.ModeAuto {
		return
	}
	if run, err := StartScheduler(ctx); err != nil {
		slog.Debug("scheduler: the internal scheduler did not start", "error", err)
	} else if run != "" {
		slog.Info("scheduler: the internal scheduler runs", "run", run)
	}
}
