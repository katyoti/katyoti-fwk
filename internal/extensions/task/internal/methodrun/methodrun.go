// SPDX-License-Identifier: Apache-2.0

// Package methodrun is the `method` machine strategy: a long
// method of any service, sidecars included, run as a task-engine run that
// owns a display panel. The run opens its panel, calls the method through
// ActionService with the panel's id in the call's context, completes the
// panel with the method's result, its failure or its cancel, and answers
// {panel, value}. A run is stopped by TaskService.Stop or by the person's
// « cancel » on the panel (subjects.PanelInSubject): either ends the call's
// context, which reaches the method in its sidecar.
//
// The run lends its panel to the method (panel.Lend): the
// method's own progress.Emit reports into it, in process or from its sidecar
// (the KTSP call carries the handle and its answer gives the last sequence
// back); the run alone completes the panel. A call that ends with no answer
// (cancelled) still completes after the method's last event: the run follows
// its panel's events and goes on after the last one it saw.
//
// A caller that must answer at once mints the panel id first
// (panel.NewID), starts the run with taskapi.MethodRunDef and gives back the
// run id and the panel id together.
package methodrun

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
	"github.com/katyoti/katyoti-fwk/internal/kts/subjects"
)

// MachineStrategyDef registers "method" in the MachineDomain.
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	taskapi.MethodMachine, "A long method run as a run, with its panel",
	func() (engine.MachineFactory, error) { return &factory{}, nil },
)

// Extension contributes the method machine strategy.
func Extension(init *application.ApplicationInit) {
	init.AddContribute(taskapi.MethodMachine, "", func(app *application.Application) error {
		return strategies.ContributeStrategy(app, MachineStrategyDef)
	})
}

// Seams between the machine and the container - package-level so tests
// substitute a stub without bootstrapping an Application.
var (
	resolveActionService = func(ctx context.Context) (actionsapi.ActionService, error) {
		return application.UseService[actionsapi.ActionService](ctx)
	}
	resolveBus = func(ctx context.Context) (mbapi.MessageBus, error) {
		return application.UseService[mbapi.MessageBus](ctx)
	}
	resolveScope = func(context.Context) panel.Scope {
		t, u := cenv.ResolveTenantUniverse(application.Env().HomeDir())
		return panel.Scope{Tenant: t, Universe: u}
	}
)

// The statuses a method run's panel completes with.
const (
	StatusOK        = "ok"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// Output is what a method run answers: its panel and the method's value.
type Output struct {
	Panel string          `json:"panel"`
	Value json.RawMessage `json:"value,omitempty"`
}

type factory struct{}

func (f *factory) Name() string { return taskapi.MethodMachine }

// CreateMachine reads the run's config ({uri, panel, title, object}) and its input,
// the method's input.
func (f *factory) CreateMachine(_ context.Context, request any) (engine.Machine, error) {
	var cfg map[string]any
	var input map[string]any
	switch r := request.(type) {
	case *taskapi.TaskExecRequest:
		cfg, _ = r.Config.(map[string]any)
		input = r.Input
	case map[string]any:
		cfg = r
		input, _ = r["input"].(map[string]any)
	}
	uri, _ := cfg["uri"].(string)
	if uri == "" {
		return nil, fmt.Errorf("method: config uri: the method to run")
	}
	if u, err := mbapi.ParseToolURI(uri); err != nil || u.SlotType == "tasks" {
		return nil, fmt.Errorf("method: %q is no method URI (a task is delegated, not run as a method)", uri)
	}
	id, _ := cfg["panel"].(string)
	if id == "" {
		id = panel.NewID()
	}
	if !panel.ValidID(id) {
		return nil, fmt.Errorf("method: %q is no panel id", id)
	}
	title, _ := cfg["title"].(string)
	if title == "" {
		title = uri
	}
	object, _ := cfg["object"].(string)
	r := &run{uri: uri, panelID: id, title: title, object: object, input: input}
	return engine.OneShotMachine(taskapi.MethodMachine, r.do), nil
}

type run struct {
	uri     string
	panelID string
	title   string
	object  string // the CWM object worked on: a kept outcome lands on it
	input   map[string]any
}

// do runs the method once under its panel.
func (r *run) do(ctx context.Context) (any, error) {
	bus, err := resolveBus(ctx)
	if err != nil || bus == nil {
		return nil, fmt.Errorf("method %s: no message bus: %v", r.uri, err)
	}
	acts, err := resolveActionService(ctx)
	if err != nil || acts == nil {
		return nil, fmt.Errorf("method %s: no ActionService: %v", r.uri, err)
	}
	scope := resolveScope(ctx)
	publish := func(ctx context.Context, subject string, payload []byte) error {
		return bus.Publish(ctx, subject, payload)
	}
	h, err := panel.OpenPanelWithID(ctx, scope, publish, r.panelID, panel.Open{
		Type: taskapi.ProgressPanelType, Title: r.title, Owner: engine.RunIDFromContext(ctx), Object: r.object,
	})
	if err != nil {
		return nil, fmt.Errorf("method %s: open its panel: %w", r.uri, err)
	}

	// The person's « cancel » on the panel ends the call, as a stop does.
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	sub, err := bus.Subscribe(ctx, subjects.PanelInSubject(scope.Tenant, scope.Universe, r.panelID), func(_ context.Context, m mbapi.Message) {
		if in, err := panel.DecodeInput(m.Data()); err == nil && in.Kind == panel.InputCancel {
			cancel()
		}
	})
	if err == nil {
		defer func() { _ = sub.Unsubscribe() }()
	}
	// The events the method sends on the lent panel, so the run's own go on
	// after them even when the call gives no answer back.
	if out, err := bus.Subscribe(ctx, subjects.PanelOutSubject(scope.Tenant, scope.Universe, r.panelID), func(_ context.Context, m mbapi.Message) {
		if e, err := panel.DecodeEvent(m.Data()); err == nil {
			h.Advance(e.Seq)
		}
	}); err == nil {
		defer func() { _ = out.Unsubscribe() }()
	}
	callCtx = panel.Lend(callCtx, h)

	input, err := json.Marshal(r.input)
	if err != nil {
		return nil, r.finish(h, StatusFailed, map[string]any{"error": err.Error()}, err)
	}
	out, err := acts.Run(callCtx, actionsapi.RunInput{
		URI: r.uri, Input: input, Context: map[string]any{"panel": r.panelID},
	})
	switch {
	case callCtx.Err() != nil:
		return nil, r.finish(h, StatusCancelled, nil, fmt.Errorf("method %s: cancelled", r.uri))
	case err != nil:
		return nil, r.finish(h, StatusFailed, map[string]any{"error": err.Error()}, fmt.Errorf("method %s: %w", r.uri, err))
	}
	if cerr := r.complete(h, StatusOK, out.Value); cerr != nil {
		return nil, cerr
	}
	return Output{Panel: r.panelID, Value: out.Value}, nil
}

// finish completes the panel with how the run ended and answers err.
func (r *run) finish(h *panel.Handle, status string, result any, err error) error {
	if cerr := r.complete(h, status, result); cerr != nil {
		return fmt.Errorf("%w (and its panel: %v)", err, cerr)
	}
	return err
}

// complete sends the panel's complete event on a context of its own: the
// run's may be the one that was cancelled.
func (r *run) complete(h *panel.Handle, status string, result any) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.Complete(ctx, status, result)
}
