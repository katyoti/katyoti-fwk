// SPDX-License-Identifier: Apache-2.0

// Package humantask is the human task: the `human-task` machine
// strategy and the collector of its to-dos. A run of it puts one scheduler
// to-do for a person - the human task's one record and claim -, waits durably
// for its answer, and ends with the answer as its result.
package humantask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/eventbus"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// MachineStrategyDef registers the `human-task` machine strategy.
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	api.HumanTaskStrategy, "A person's piece of work: asks once, waits durably for the answer",
	func() (engine.MachineFactory, error) { return factory{}, nil },
)

// recheckEvery is how often a waiting human task reads its to-do again, event
// or not: an answer collected while no run listened.
const recheckEvery = time.Hour

// The machine's states.
const (
	stateAsk  = "ask"
	stateShow = "show" // its card's panel opened: placed in the chat, then waited
	stateWait = "wait"
)

type factory struct{}

func (factory) Name() string { return api.HumanTaskStrategy }

// CreateMachine reads what the human task asks from its request: the task's
// config, its input overriding it field by field (a direct caller gives a map).
func (factory) CreateMachine(ctx context.Context, req any) (engine.Machine, error) {
	spec, err := SpecOf(req)
	if err != nil {
		return nil, err
	}
	if spec, err = Resolve(ctx, spec); err != nil {
		return nil, err
	}
	return &Machine{Spec: spec}, nil
}

// Kinds are the kinds of questions contributed, in the order
// a list shows their counts, a name once (api.AskKinds).
func Kinds(ctx context.Context) ([]api.AskKind, error) { return api.AskKinds(ctx) }

// Resolve refuses a human task whose kind no pack contributes, and gives it
// its kind's label and defaults (api.ResolveAsk).
func Resolve(ctx context.Context, s api.HumanTask) (api.HumanTask, error) {
	return api.ResolveAsk(ctx, s)
}

// SpecOf is the human task a request asks: config then input, field by field.
func SpecOf(req any) (api.HumanTask, error) {
	var layers []map[string]any
	switch r := req.(type) {
	case *taskapi.TaskExecRequest:
		if m, ok := r.Config.(map[string]any); ok {
			layers = append(layers, m)
		}
		layers = append(layers, r.Input)
	case map[string]any:
		layers = append(layers, r)
	case api.HumanTask:
		return check(r)
	}
	merged := map[string]any{}
	for _, l := range layers {
		for k, v := range l {
			if v != nil && v != "" {
				merged[k] = v
			}
		}
	}
	var spec api.HumanTask
	raw, err := json.Marshal(merged)
	if err != nil {
		return spec, fmt.Errorf("%s: %w", api.HumanTaskStrategy, err)
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		return spec, fmt.Errorf("%s: what it asks does not read: %w", api.HumanTaskStrategy, err)
	}
	return check(spec)
}

// check refuses a human task that asks nothing, or does not say what is
// expected of the person (its kind); whether the kind is known
// is Resolve's, which reads the kinds contributed.
func check(s api.HumanTask) (api.HumanTask, error) {
	s.Kind = strings.TrimSpace(s.Kind)
	if s.Kind == "" {
		return s, fmt.Errorf("%w: %s: no kind - a question says first what is expected", cwmapi.ErrInvalidInput, api.HumanTaskStrategy)
	}
	s.Title = strings.TrimSpace(s.Title)
	if s.Title == "" {
		if q := strings.TrimSpace(s.Question); q != "" {
			s.Title, _, _ = strings.Cut(q, "\n")
		}
	}
	if s.Title == "" {
		return s, fmt.Errorf("%w: %s: no question (title)", cwmapi.ErrInvalidInput, api.HumanTaskStrategy)
	}
	return s, nil
}

// Machine is one human task's run.
type Machine struct {
	Spec api.HumanTask
}

func (*Machine) Name() string                           { return api.HumanTaskStrategy }
func (*Machine) InitialState(engine.Event) engine.State { return engine.State{Name: stateAsk} }

// data is what the run keeps between its steps: the to-do it put, and the
// panel of its card in a chat - whether it was placed there.
type data struct {
	Todo   string       `json:"todo"`
	Panel  *panel.Track `json:"panel,omitempty"`
	Placed bool         `json:"placed,omitempty"`
}

func dataOf(v any) data {
	var d data
	raw, err := json.Marshal(v)
	if err == nil {
		_ = json.Unmarshal(raw, &d)
	}
	return d
}

// Step asks once, then waits until the to-do carries its answer.
func (m *Machine) Step(_ context.Context, st *engine.State, _ engine.Event) (engine.ActionFunc, error) {
	d := dataOf(st.Data)
	return func(ctx context.Context) (*engine.StepResult, error) {
		todos, err := application.UseService[schedapi.TodoService](ctx)
		if err != nil {
			return nil, fmt.Errorf("%s: no scheduler: %w", api.HumanTaskStrategy, err)
		}
		if d.Todo == "" {
			// One need, one question: what an open human task
			// already asks is waited on there, what a proven answer settled
			// is answered by it - neither is asked again.
			covering, err := api.Covering(ctx, todos, m.Spec.Satisfies)
			if err != nil {
				return nil, fmt.Errorf("%s: what already asks %v: %w", api.HumanTaskStrategy, m.Spec.Satisfies, err)
			}
			if covering != nil {
				d.Todo = covering.ID
			} else {
				todo, err := seatOf(ctx, TodoOf(m.Spec, engine.RunIDFromContext(ctx), clock()), m.Spec)
				if err != nil {
					return nil, fmt.Errorf("%s: who may answer: %w", api.HumanTaskStrategy, err)
				}
				out, err := todos.Put(ctx, schedapi.PutInput{Todo: todo})
				if err != nil {
					return nil, fmt.Errorf("%s: ask: %w", api.HumanTaskStrategy, err)
				}
				d.Todo = out.Todo.ID
			}
		}
		got, err := todos.Get(ctx, schedapi.GetInput{ID: d.Todo})
		if err != nil {
			if errors.Is(err, cwmapi.ErrNotFound) {
				return nil, fmt.Errorf("%s: its to-do %s is gone: no answer will come", api.HumanTaskStrategy, d.Todo)
			}
			return nil, err
		}
		if a, ok := api.AnswerOf(got.Todo); ok {
			return &engine.StepResult{NextState: "done", Data: d, Output: a, Panels: m.answeredCard(ctx, &d, got.Todo, a)}, nil
		}
		if m.inChat() {
			if d.Panel == nil {
				// Its card shows in the chat that asks it: the panel opens
				// with this step, placed in the conversation by the next.
				open, err := m.openCard(ctx, &d, got.Todo)
				if err != nil {
					return nil, fmt.Errorf("%s: its card in the chat: %w", api.HumanTaskStrategy, err)
				}
				return &engine.StepResult{NextState: stateShow, Data: d, Panels: open}, nil
			}
			m.placeCard(ctx, &d)
		}
		now := clock()
		t := remindOrEscalate(ctx, todos, got.Todo, now)
		return &engine.StepResult{NextState: stateWait, Data: d, Wait: waitFor(d.Todo, nextDate(t), now)}, nil
	}, nil
}

// waitFor is the durable wait on a to-do's answer: woken when it is
// collected, read again at its next date (follow-up, due) or at least hourly,
// and at once after a restart.
func waitFor(todo string, next, now time.Time) *engine.WaitSpec {
	timeout := recheckEvery
	if !next.IsZero() {
		if d := next.Sub(now); d < timeout {
			timeout = max(d, time.Second)
		}
	}
	return &engine.WaitSpec{
		Conditions:      []engine.WaitCondition{{Topic: engine.HumanTaskAnsweredTopic(todo), Label: "answered"}},
		Mode:            engine.WaitAny,
		Timeout:         timeout,
		TimeoutLabel:    "recheck",
		RecheckOnResume: true,
	}
}

// TodoOf is the to-do a human task puts: for a person, in the module of what
// it is about (the requests module when that holds no to-dos), keyed by its
// run so a replayed ask is put once.
func TodoOf(s api.HumanTask, run string, now time.Time) schedapi.Todo {
	priority := s.Priority
	if priority == "" {
		priority = "P2"
	}
	target := s.About
	if target == "" {
		target = schedapi.RequestsModule
	}
	work := map[string]any{"doer": "person", "task": api.HumanTaskStrategy, api.WorkAsk: asMap(s)}
	if s.Kind != "" {
		work["interaction"] = s.Kind
		if s.Label != "" {
			work[api.WorkLabel] = s.Label
		}
	}
	if run != "" {
		work["run"] = run
	}
	if at := whenOf(s.FollowUp, now); at != "" {
		work[api.WorkFollowUp] = at
	}
	if at := whenOf(s.Due, now); at != "" {
		work[api.WorkDue] = at
	}
	t := schedapi.Todo{
		Module:   ModuleOf(s.About),
		Target:   target,
		Title:    s.Title,
		For:      schedapi.ForPerson,
		Start:    schedapi.StartManual,
		Priority: priority,
		State:    schedapi.StateReady,
		Since:    now.UTC().Format(time.RFC3339),
		Owner:    api.Owner,
		Work:     work,
		// The holding list it waits on is its scheduler group: only its
		// members pull it.
		Group:    listOf(s),
		Assignee: s.Assignee,
	}
	if run != "" {
		t.Key = api.HumanTaskStrategy + ":" + run
	}
	return t
}

// asMap is what a human task asks as the to-do's files keep it: its JSON
// field names, the empty ones left out.
func asMap(s api.HumanTask) map[string]any {
	m := map[string]any{}
	if raw, err := json.Marshal(s); err == nil {
		_ = json.Unmarshal(raw, &m)
	}
	return m
}

// ModuleOf is the module that holds a human task's to-do: what it is about's,
// unless that is a system module - only the requests module holds to-dos
// among them - or it is about nothing.
func ModuleOf(about string) string {
	m, _, ok := strings.Cut(about, ".")
	if !ok || m == "" || strings.HasPrefix(m, cwmapi.SystemModulePrefix) {
		return schedapi.RequestsModule
	}
	return m
}

// Collect is the inbox's collector of its to-dos: a human task's answer is
// kept on its to-do, archived, and the run waiting on it is told.
func Collect(ctx context.Context, t schedapi.Todo) error {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return err
	}
	// An answer is never lost: kept before anything goes on.
	kept, err := Store(ctx, t)
	if err != nil {
		return err
	}
	if _, err := todos.Archive(ctx, schedapi.ArchiveInput{ID: t.ID, Note: "answered, kept as " + kept}); err != nil {
		return err
	}
	if bus, err := application.UseService[eventbus.EventBus](ctx); err == nil && bus != nil {
		_ = bus.Publish(engine.HumanTaskAnsweredTopic(t.ID), t.ID)
	}
	return nil
}
