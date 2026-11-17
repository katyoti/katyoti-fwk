// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/events/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// Reactions is the events extension's part of a reaction: it takes CWM's
// matches (Take), runs them (RunTodo, also the Run verb), collects them
// (Collect), and answers the reactions verbs. Every service is resolved from
// the context: CWM, the scheduler, the ActionService and the TaskService.
type Reactions struct {
	// taskPoll is how often a started task run is asked whether it ended.
	taskPoll time.Duration
	// backoff is the seconds before a failed reaction is tried again (doubled
	// at each attempt by the scheduler).
	backoff int
	// sourcesDir is where the sources keep their cursors ("": the default).
	sourcesDir string
}

// WithSourcesDir points the Events read at the sources' cursor folder.
func (r *Reactions) WithSourcesDir(dir string) *Reactions { r.sourcesDir = dir; return r }

// NewReactions builds the reactions part; backoffSeconds <= 0 is the default.
func NewReactions(backoffSeconds int) *Reactions {
	if backoffSeconds <= 0 {
		backoffSeconds = api.DefaultBackoff
	}
	return &Reactions{taskPoll: 2 * time.Second, backoff: backoffSeconds}
}

var _ api.ReactionService = (*Reactions)(nil)

// --- the taker ---------------------------------------------------------------

// Take puts one to-do for a match in the module written: keyed by the match's
// id, so a match handed over again (a crash, a retry of the handover) is the
// same to-do. The to-do carries the match — its events, their cause and depth
// (: a reaction is self-contained wherever it is pulled).
func (r *Reactions) Take(ctx context.Context, m cwmapi.Match) error {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return fmt.Errorf("reactions: the scheduler: %w", err)
	}
	match, err := asMap(m)
	if err != nil {
		return err
	}
	priority := m.Subscription.Priority
	if priority == "" {
		priority = cwmapi.DefaultAsyncPriority
	}
	attempts := m.Subscription.Attempts
	if attempts <= 0 {
		attempts = cwmapi.DefaultAsyncAttempts
	}
	// The reaction is the SUBSCRIBER's work: its
	// queue, reading the emitter's object and its own module, writing what it
	// declared (by default its own module) — CWM refuses a write elsewhere.
	queue, reads, writes := m.Module, []string{m.Module}, []string{m.Module}
	if sub := m.Subscription.Module; sub != "" && sub != m.Module {
		queue, reads, writes = sub, []string{m.Module, sub}, []string{sub}
	}
	if len(m.Subscription.Writes) > 0 {
		writes = append([]string(nil), m.Subscription.Writes...)
	}
	_, err = todos.Put(ctx, schedapi.PutInput{Todo: schedapi.Todo{
		Module:         queue,
		Target:         m.ObjectID,
		Title:          "reaction — " + m.Subscription.URI + " on " + m.ObjectID,
		For:            schedapi.ForAgent,
		Group:          api.ReactionGroup,
		Start:          schedapi.StartAuto,
		Priority:       priority,
		State:          schedapi.StateReady,
		Reads:          reads,
		Writes:         writes,
		Owner:          api.Owner,
		Key:            m.ID,
		MaxAttempts:    attempts,
		BackoffSeconds: r.backoff,
		Work: map[string]any{
			api.WorkDoer:      api.WorkDoerValue,
			api.WorkURI:       m.Subscription.URI,
			api.WorkMatch:     match,
			api.WorkInstructs: "Run this reaction: `ktw-agents reactions run --todo <this to-do> --by <you>` runs its action under the reaction's cause and reports it.",
		},
	}})
	return err
}

// --- the runner --------------------------------------------------------------

// RunTodo is the scheduler's runner for the events owner: the internal worker
// pulled the to-do; it runs it. A task or agent action answers its run, which
// the scheduler watches; a sync action has reported when it returns.
func (r *Reactions) RunTodo(ctx context.Context, t schedapi.Todo) (string, string, error) {
	if t.Work[api.WorkDoer] == api.WorkDoerDelayed {
		cwm, err := application.UseService[cwmapi.CWMService](ctx)
		if err != nil {
			return "", "", err
		}
		_, err = raiseDelayed(ctx, cwm, t, schedapi.Worker)
		return "", "", err
	}
	out, err := r.run(ctx, t, schedapi.Worker)
	if err != nil {
		return "", "", err
	}
	return out.Run, "", nil
}

// Run is the verb an outside worker uses: the to-do must be held by the caller.
func (r *Reactions) Run(ctx context.Context, in api.ReactionRunInput) (api.ReactionRunOutput, error) {
	if in.Todo == "" || in.By.ID == "" {
		return api.ReactionRunOutput{}, fmt.Errorf("%w: reactions: Run needs the to-do and who runs it", cwmapi.ErrInvalidInput)
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return api.ReactionRunOutput{}, err
	}
	got, err := todos.Get(ctx, schedapi.GetInput{ID: in.Todo})
	if err != nil {
		return api.ReactionRunOutput{}, err
	}
	t := got.Todo
	if t.Owner != api.Owner {
		return api.ReactionRunOutput{}, fmt.Errorf("%w: reactions: %s is not a reaction (its owner is %s)", cwmapi.ErrInvalidInput, t.ID, t.Owner)
	}
	if t.State != schedapi.StateClaimed || t.Claim == nil || t.Claim.By != in.By {
		return api.ReactionRunOutput{}, fmt.Errorf("%w: reactions: %s is not held by %s — pull it first (todos pull --id %s)", cwmapi.ErrInvalidInput, t.ID, in.By.ID, t.ID)
	}
	return r.run(ctx, t, in.By)
}

// run runs one reaction to-do held by `by`: its action under the reaction's
// cause, then its report through the contract.
func (r *Reactions) run(ctx context.Context, t schedapi.Todo, by cwmapi.Actor) (api.ReactionRunOutput, error) {
	m, uri, err := matchOf(t)
	if err != nil {
		return api.ReactionRunOutput{}, err
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil {
		return api.ReactionRunOutput{}, fmt.Errorf("reactions: the action service: %w", err)
	}
	input, err := json.Marshal(inputOf(m))
	if err != nil {
		return api.ReactionRunOutput{}, err
	}
	// The reaction's cause: its writes stage what they trigger in the outbox,
	// released by the collector on success (CWM's ReactionCause).
	rctx := cause.WithCause(ctx, cwmapi.ReactionCause(m))
	// An ATOMIC subscription's writes are staged in the reaction's change
	// set: opened at every attempt (a leftover set
	// of a dead attempt is discarded by it), committed after the action and
	// before success is reported, discarded on a failed attempt.
	atomic := m.Subscription.Atomic
	if atomic {
		if err := beginSet(ctx, m.ID); err != nil {
			return api.ReactionRunOutput{}, err
		}
	}
	res, runErr := actions.Run(rctx, actionsapi.RunInput{URI: uri, Input: input, Context: map[string]any{"_target": m.ObjectID}})
	out := api.ReactionRunOutput{Todo: t.ID}
	switch {
	case runErr != nil:
		out.Status, out.Summary = schedapi.StatusFail, "its action failed: "+runErr.Error()
	case res.TaskID != "":
		out.Status, out.Summary, out.Run = "started", "its action started run "+res.TaskID, res.TaskID
		go r.reportWhenEnded(context.WithoutCancel(ctx), t.ID, res.TaskID, by, m.ID, atomic)
		return out, nil
	default:
		out.Status, out.Summary = verdictOf(res.Value)
	}
	if atomic {
		out.Status, out.Summary = settleSet(ctx, m.ID, out.Status, out.Summary)
	}
	if err := report(ctx, t.ID, out.Status, out.Summary, by); err != nil {
		return out, err
	}
	return out, nil
}

// reportWhenEnded reports a reaction whose action is a task or agent run once
// that run has ended: success when it completed, fail otherwise.
func (r *Reactions) reportWhenEnded(ctx context.Context, todo, run string, by cwmapi.Actor, reaction string, atomic bool) {
	tasks, err := application.UseService[taskapi.TaskService](ctx)
	if err != nil {
		slog.Warn("reactions: no task service to follow a reaction's run", "todo", todo, "run", run, "error", err)
		return
	}
	for {
		st, err := tasks.Status(ctx, taskapi.StatusRequest{RunID: run})
		if err == nil {
			switch st.Status {
			case taskapi.RunStatusCompleted, taskapi.RunStatusFailed, taskapi.RunStatusCancelled:
				status, summary := schedapi.StatusSuccess, "its run "+run+" completed"
				if st.Status != taskapi.RunStatusCompleted {
					status, summary = schedapi.StatusFail, "its run "+run+" ended "+string(st.Status)
				}
				if atomic {
					status, summary = settleSet(ctx, reaction, status, summary)
				}
				_ = report(ctx, todo, status, summary, by)
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.taskPoll):
		}
	}
}

// beginSet opens a reaction's change set.
func beginSet(ctx context.Context, reaction string) error {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return err
	}
	_, err = cwm.ChangeSetBegin(ctx, cwmapi.ChangeSetBeginInput{Reaction: reaction})
	return err
}

// settleSet ends an atomic attempt's change set: a success commits it — a
// revision moved since the reaction read it fails the attempt, to rerun on the
// current state; a failure discards it, so nothing of the attempt is written.
// It answers the attempt's status and summary, as the commit left them.
func settleSet(ctx context.Context, reaction, status, summary string) (string, string) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return schedapi.StatusFail, summary + "; its change set unreachable: " + err.Error()
	}
	if status == schedapi.StatusSuccess {
		got, err := cwm.ChangeSetCommit(ctx, cwmapi.ChangeSetCommitInput{Reaction: reaction})
		switch {
		case errors.Is(err, cwmapi.ErrConflict):
			return schedapi.StatusFail, "conflict at commit — an object it wrote changed meanwhile: rerun on the current state (" + err.Error() + ")"
		case err != nil:
			_, _ = cwm.ChangeSetDiscard(ctx, cwmapi.ChangeSetDiscardInput{Reaction: reaction})
			return schedapi.StatusFail, "its change set did not commit: " + err.Error()
		}
		return status, fmt.Sprintf("%s; committed %d write(s)", summary, len(got.Writes))
	}
	if d, err := cwm.ChangeSetDiscard(ctx, cwmapi.ChangeSetDiscardInput{Reaction: reaction}); err == nil {
		summary = fmt.Sprintf("%s; %d staged write(s) discarded", summary, len(d.Discarded))
	}
	return status, summary
}

// --- the collector -----------------------------------------------------------

// Collect is the scheduler's collector for the events owner. Success releases
// what the reaction's writes staged in CWM's outbox, then archives the to-do.
// A failure past its attempts, or a blocked one, holds the outbox: the to-do
// stays reported, for a person to retry or drop. A failure with
// attempts left never reaches here: the scheduler puts it back after its
// backoff — nothing is released on failure.
func (r *Reactions) Collect(ctx context.Context, t schedapi.Todo) error {
	if t.Work[api.WorkDoer] == api.WorkDoerDelayed {
		if t.Result == nil || t.Result.Status != schedapi.StatusSuccess {
			return nil // held, like a reaction: visible, retried by a person
		}
		todos, err := application.UseService[schedapi.TodoService](ctx)
		if err != nil {
			return err
		}
		_, err = todos.Archive(ctx, schedapi.ArchiveInput{ID: t.ID, Note: "raised"})
		return err
	}
	if t.Result == nil || t.Result.Status != schedapi.StatusSuccess {
		return nil // held: reported, visible, for reactions retry / drop
	}
	m, _, err := matchOf(t)
	if err != nil {
		return err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return err
	}
	rel, err := cwm.OutboxRelease(ctx, cwmapi.OutboxReleaseInput{Reaction: m.ID})
	if err != nil {
		return fmt.Errorf("reactions: release the outbox of %s: %w", m.ID, err)
	}
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return err
	}
	_, err = todos.Archive(ctx, schedapi.ArchiveInput{ID: t.ID, Note: fmt.Sprintf("released: %d match(es), %d hook call(s)", rel.Matches, rel.Hooks)})
	return err
}

// --- retry, drop, list -------------------------------------------------------

// Retry sends a reaction given up (reported failed or blocked) back to the
// queue: ready, its attempts counted again, its outbox still held.
func (r *Reactions) Retry(ctx context.Context, in api.ReactionRetryInput) (api.ReactionRunOutput, error) {
	todos, t, err := givenUp(ctx, in.Todo)
	if err != nil {
		return api.ReactionRunOutput{}, err
	}
	t.State, t.Result, t.Claim, t.Attempts, t.NotBefore = schedapi.StateReady, nil, nil, 0, ""
	t.Note = "retried by " + in.By.ID
	if _, err := todos.Put(ctx, schedapi.PutInput{Todo: t}); err != nil {
		return api.ReactionRunOutput{}, err
	}
	return api.ReactionRunOutput{Todo: t.ID, Status: schedapi.StateReady, Summary: "back in the queue"}, nil
}

// Drop gives a reaction up: CWM drops its outbox and answers the writes it
// made without their events; the to-do is archived with them in its note.
func (r *Reactions) Drop(ctx context.Context, in api.ReactionDropInput) (api.ReactionDropOutput, error) {
	if strings.TrimSpace(in.Why) == "" {
		return api.ReactionDropOutput{}, fmt.Errorf("%w: reactions: Drop needs why", cwmapi.ErrInvalidInput)
	}
	todos, t, err := givenUp(ctx, in.Todo)
	if err != nil {
		return api.ReactionDropOutput{}, err
	}
	m, _, err := matchOf(t)
	if err != nil {
		return api.ReactionDropOutput{}, err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ReactionDropOutput{}, err
	}
	dropped, err := cwm.OutboxDrop(ctx, cwmapi.OutboxDropInput{Reaction: m.ID})
	if err != nil {
		return api.ReactionDropOutput{}, err
	}
	writes := make([]string, 0, len(dropped.Writes))
	for _, w := range dropped.Writes {
		writes = append(writes, w.ObjectID+" "+w.Action)
	}
	note := "dropped by " + in.By.ID + ": " + in.Why
	if len(writes) > 0 {
		note += " — written without their events: " + strings.Join(writes, ", ")
	}
	if _, err := todos.Archive(ctx, schedapi.ArchiveInput{ID: t.ID, Note: note}); err != nil {
		return api.ReactionDropOutput{}, err
	}
	return api.ReactionDropOutput{Todo: t.ID, Writes: dropped.Writes}, nil
}

// List is a module's open reactions.
func (r *Reactions) List(ctx context.Context, in api.ReactionListInput) (api.ReactionListOutput, error) {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return api.ReactionListOutput{}, err
	}
	all, err := todos.List(ctx, schedapi.ListInput{Module: in.Module})
	if err != nil {
		return api.ReactionListOutput{}, err
	}
	out := api.ReactionListOutput{Reactions: []api.Reaction{}}
	for _, t := range all.Todos {
		if t.Owner != api.Owner {
			continue
		}
		m, uri, err := matchOf(t)
		if err != nil || (in.Object != "" && m.ObjectID != in.Object) {
			continue
		}
		rc := api.Reaction{Todo: t.ID, State: t.State, Subscription: m.Subscription.ID, URI: uri, Object: m.ObjectID, Depth: m.Depth, Attempts: t.Attempts}
		for _, e := range m.Events {
			rc.Events = append(rc.Events, e.Event+" "+e.EventID)
		}
		if m.Cause != nil {
			rc.Cause = m.Cause.String()
		}
		if t.Result != nil {
			rc.Result = t.Result.Status + ": " + t.Result.Summary
		}
		out.Reactions = append(out.Reactions, rc)
	}
	return out, nil
}

// Subscribers lists who subscribes to a module's events: CWM's
// ModuleSubscribers, as the emitter's channel shows it.
func (r *Reactions) Subscribers(ctx context.Context, in api.ReactionSubscribersInput) (cwmapi.ModuleSubscribersOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return cwmapi.ModuleSubscribersOutput{}, err
	}
	return cwm.ModuleSubscribers(ctx, cwmapi.ModuleSubscribersInput{Module: in.Module, Event: in.Event})
}

// Events answers the declared event types with their subscribers as CWM
// resolves them (EventSubscriptions), each subscription's open reactions, and
// the sources' state for the events pack's own types.
func (r *Reactions) Events(ctx context.Context, in api.ReactionEventsInput) (api.ReactionEventsOutput, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ReactionEventsOutput{}, err
	}
	subs, err := cwm.EventSubscriptions(ctx, cwmapi.EventSubscriptionsInput{Event: in.Event})
	if err != nil {
		return api.ReactionEventsOutput{}, err
	}
	open := map[string]int{}
	if todos, err := application.UseService[schedapi.TodoService](ctx); err == nil {
		if list, err := todos.List(ctx, schedapi.ListInput{}); err == nil {
			for _, t := range list.Todos {
				if t.Owner != api.Owner {
					continue
				}
				if m, _, err := matchOf(t); err == nil {
					open[m.Subscription.ID]++
				}
			}
		}
	}
	state := r.sourceState()
	out := api.ReactionEventsOutput{Events: []api.EventView{}}
	for _, e := range subs.Events {
		v := api.EventView{EventTypeSubscriptions: e}
		if v.Subscriptions == nil {
			v.Subscriptions = []cwmapi.EventSubscription{} // an event type nobody subscribes to: an empty list, never null
		}
		for _, s := range e.Subscriptions {
			if n := open[s.Subscription]; n > 0 {
				if v.Open == nil {
					v.Open = map[string]int{}
				}
				v.Open[s.Subscription] = n
			}
		}
		switch e.Event {
		case api.EventCodeChanged:
			if state.Head != "" {
				v.Source = &api.SourceState{Repo: state.Repo, Head: state.Head}
			}
		case api.EventTimerElapsed:
			if len(state.Timers) > 0 {
				v.Source = &api.SourceState{Timers: state.Timers}
			}
		}
		out.Events = append(out.Events, v)
	}
	return out, nil
}

// sourceState reads the sources' cursors (the code source's HEAD, the
// timers' last firings) — read-only; nothing when the sources never ran.
func (r *Reactions) sourceState() cursors {
	return (&Sources{stateDir: r.sourcesDir}).load()
}

// --- helpers -----------------------------------------------------------------

// givenUp is a reaction to-do reported failed or blocked, with the scheduler.
func givenUp(ctx context.Context, id string) (schedapi.TodoService, schedapi.Todo, error) {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return nil, schedapi.Todo{}, err
	}
	got, err := todos.Get(ctx, schedapi.GetInput{ID: id})
	if err != nil {
		return nil, schedapi.Todo{}, err
	}
	t := got.Todo
	if t.Owner != api.Owner {
		return nil, t, fmt.Errorf("%w: reactions: %s is not a reaction", cwmapi.ErrInvalidInput, id)
	}
	if t.State != schedapi.StateReported || t.Result == nil || t.Result.Status == schedapi.StatusSuccess {
		return nil, t, fmt.Errorf("%w: reactions: %s is not given up (it is %s)", cwmapi.ErrInvalidInput, id, t.State)
	}
	return todos, t, nil
}

// report hands a reaction's result back through the contract.
func report(ctx context.Context, todo, status, summary string, by cwmapi.Actor) error {
	todos, err := application.UseService[schedapi.TodoService](ctx)
	if err != nil {
		return err
	}
	if len(summary) > 500 {
		summary = summary[:500] + "…"
	}
	_, err = todos.Report(ctx, schedapi.ReportInput{ID: todo, Result: schedapi.Result{Status: status, Summary: summary, By: by}})
	return err
}

// matchOf reads the match and the action URI a reaction to-do carries.
func matchOf(t schedapi.Todo) (cwmapi.Match, string, error) {
	var m cwmapi.Match
	uri, _ := t.Work[api.WorkURI].(string)
	raw, ok := t.Work[api.WorkMatch]
	if !ok || uri == "" {
		return m, "", fmt.Errorf("%w: reactions: %s carries no match", cwmapi.ErrInvalidInput, t.ID)
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return m, "", err
	}
	if err := json.Unmarshal(b, &m); err != nil || m.ID == "" {
		return m, "", fmt.Errorf("%w: reactions: %s carries an unreadable match: %v", cwmapi.ErrInvalidInput, t.ID, err)
	}
	return m, uri, nil
}

// inputOf is what the reaction's action receives: the entry's declared input
// resolved over the object at the write, else the match itself.
func inputOf(m cwmapi.Match) any {
	if len(m.Subscription.ResolvedInput) > 0 {
		return m.Subscription.ResolvedInput
	}
	return map[string]any{"object": m.ObjectID, "module": m.Module, "events": m.Events, "reaction": m.ID}
}

// verdictOf reads a sync action's value: a script's non-zero exit is a failure.
func verdictOf(v json.RawMessage) (string, string) {
	var s struct {
		ExitCode *int   `json:"exit_code"`
		Stderr   string `json:"stderr"`
		Stdout   string `json:"stdout"`
	}
	if json.Unmarshal(v, &s) == nil && s.ExitCode != nil && *s.ExitCode != 0 {
		return schedapi.StatusFail, fmt.Sprintf("its action exited %d: %s", *s.ExitCode, strings.TrimSpace(s.Stderr+" "+s.Stdout))
	}
	return schedapi.StatusSuccess, "its action ran: " + string(v)
}

func asMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}
