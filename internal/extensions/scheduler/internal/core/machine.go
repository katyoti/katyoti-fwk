// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// MachineStrategyDef registers the `scheduler` machine strategy: the internal
// scheduler as a long-lived run. Each step ticks (Tick); between steps it waits
// — for a move in the queues or of a plan, or its interval — holding no slot,
// its wait kept across a restart (and rechecked: it steps at once when
// restored). It ends when it has nothing to run (off, or no module auto).
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	api.SchedulerTask, "The internal scheduler: runs the auto to-dos of the auto modules",
	func() (engine.MachineFactory, error) { return machineFactory{}, nil },
)

type machineFactory struct{}

func (machineFactory) Name() string { return api.SchedulerTask }

func (machineFactory) CreateMachine(ctx context.Context, req any) (engine.Machine, error) {
	cfg := api.SchedulerConfig{}
	if er, ok := req.(*taskapi.TaskExecRequest); ok {
		if m, ok := er.Config.(map[string]any); ok {
			switch v := m["interval_seconds"].(type) {
			case float64:
				cfg.IntervalSeconds = int(v)
			case int:
				cfg.IntervalSeconds = v
			}
		}
	}
	svc, err := ServiceIn(ctx)
	if err != nil {
		return nil, err
	}
	return &machine{svc: svc, interval: cfg.Interval()}, nil
}

type machine struct {
	svc      *Service
	interval time.Duration
}

func (*machine) Name() string                           { return api.SchedulerTask }
func (*machine) InitialState(engine.Event) engine.State { return engine.State{Name: "tick"} }

func (m *machine) Step(context.Context, *engine.State, engine.Event) (engine.ActionFunc, error) {
	return func(ctx context.Context) (*engine.StepResult, error) {
		if o := older(ctx); o != "" {
			return &engine.StepResult{NextState: "done", Output: map[string]any{"stopped": "the internal scheduler " + o + " runs"}}, nil
		}
		if !running(ctx) {
			return &engine.StepResult{NextState: "done", Output: map[string]any{"stopped": "nothing to run: the scheduler is off or no module is auto"}}, nil
		}
		rep, err := m.svc.Tick(ctx, api.TickInput{})
		if err != nil {
			rep.Errors = append(rep.Errors, err.Error())
			slog.Warn("scheduler: step failed", "error", err)
		}
		res := &engine.StepResult{
			NextState: "tick",
			Wait: &engine.WaitSpec{
				Conditions:      []engine.WaitCondition{{Topic: engine.PlanChangedTopic(), Label: "moved"}},
				Mode:            engine.WaitAny,
				Timeout:         m.interval,
				TimeoutLabel:    "interval",
				RecheckOnResume: true,
			},
		}
		if len(rep.Started)+len(rep.GivenBack)+len(rep.Errors) > 0 {
			res.Emit = []engine.Event{{Type: engine.PlannerTickTopic(), Payload: rep}}
		}
		return res, nil
	}, nil
}

// older is another internal scheduler run started before this one, still
// active: one runs. A restored run is the older; one started beside it stops.
func older(ctx context.Context) string {
	self := engine.RunIDFromContext(ctx)
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil || self == "" {
		return ""
	}
	me, err := tasks.Status(ctx, taskapi.StatusRequest{RunID: self})
	if err != nil {
		return ""
	}
	runs, err := tasks.Runs(ctx, taskapi.ListRunsRequest{Status: "active"})
	if err != nil {
		return ""
	}
	for _, r := range runs.Runs {
		if r.TaskName == api.SchedulerTask && r.RunID != self && r.StartedAt.Before(me.StartedAt) {
			return r.RunID
		}
	}
	return ""
}
