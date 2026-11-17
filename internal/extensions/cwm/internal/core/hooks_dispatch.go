// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// DISPATCHING THE WRITE POINTS. The Object API runs what the hook table says
// applies at the moment of a write: `before_*` synchronously (they may refuse),
// `on_*` after the write (a failure is reported, never undoes it). What runs is
// an action (a method / script / task) through the ActionService — CWM owns the
// point, the extension owns the work.

// hookCall is what every hook action receives (api.HookCall) when its entry
// declares no input; ev is the event the point fires on (nil: none).
func hookCall(point api.HookPoint, obj api.Object, prior *api.Object, ev *api.MatchedEvent) api.HookCall {
	return api.HookCall{Point: point, Object: &obj, Prior: prior, Event: ev}
}

// hookInput is what one hook's action receives: its declared input, resolved
// over the object and the call's event (the hook vocabulary), or else the
// call itself.
func (s *Service) hookInput(h api.ResolvedHook, call api.HookCall) (json.RawMessage, error) {
	if len(h.Input) == 0 {
		return json.Marshal(call)
	}
	return json.Marshal(interpolateObjectInput(hookExprs, h.Input, s.hookEnv(*call.Object, eventEnv(call.Event))))
}

// applyingHooks is the entries that apply to obj at point, at the event ev
// (nil: a point that fires on none).
func (s *Service) applyingHooks(obj api.Object, point api.HookPoint, ev *api.MatchedEvent) []api.ResolvedHook {
	var out []api.ResolvedHook
	for _, h := range s.resolveHooks(obj, s.effectiveRefs(obj), point, hookAtEvent(ev)) {
		if h.Applies {
			out = append(out, h)
		}
	}
	return out
}

// runBeforeHooks asks every applying before_* entry; the first refusal refuses
// the write. With entries to ask and no ActionService to ask them, the write is
// refused too — a gate that cannot run must not read as a gate that passed.
func (s *Service) runBeforeHooks(ctx context.Context, point api.HookPoint, obj api.Object, prior *api.Object) error {
	hooks := s.applyingHooks(obj, point, nil)
	if len(hooks) == 0 {
		return nil
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return fmt.Errorf("%w: %s %q: %d hook(s) declared and no action service to run them",
			api.ErrRefusedByHook, point, obj.ID, len(hooks))
	}
	call := hookCall(point, obj, prior, nil)
	for _, h := range hooks {
		input, err := s.hookInput(h, call)
		if err != nil {
			return fmt.Errorf("cwm/core: %s hook input: %w", point, err)
		}
		out, err := actions.Run(ctx, actionsapi.RunInput{URI: h.URI, Input: input})
		if err != nil {
			return api.RefusedByHook(string(point), h, err.Error())
		}
		var v api.HookAccept
		if len(out.Value) == 0 || json.Unmarshal(out.Value, &v) != nil || !v.Accept {
			reason := v.Reason
			if reason == "" {
				reason = "did not accept"
			}
			return api.RefusedByHook(string(point), h, reason)
		}
	}
	return nil
}

// fireAfterHooks journals every applying on_* entry, then runs it after the
// write, detached from the caller's cancellation (the write is done; its
// consequences are not the caller's to cancel). The journal (hooks_journal.go)
// is what makes a call survive a stop, a missing action service and a failing
// action: retried, then shown on the object — never silently dropped.
//
// Each call runs under its own cause — {hook, the entry's id, the write's
// depth + 1} — so the writes the action makes say what reaction made them
// and how deep the cascade is; the cause is journaled with the entry, so a
// replay (replayPendingHooks) runs under the same one.
//
// An entry that declares `async` is a SUBSCRIPTION, never dispatched here: it
// becomes a match (matches.go), recorded before the verb returns and handed
// over in the background, under the cascade guards. A write whose cause is a
// reaction runs nothing and hands nothing over: its matches and its hook calls
// are staged in the reaction's outbox (outbox.go), released when the reaction
// succeeds. ev is the event, as published: a write's, or one raised on obj
// (EventRaise, at its type's point) — the entries' `when`, input and key read
// it as `event`.
func (s *Service) fireAfterHooks(ctx context.Context, point api.HookPoint, obj api.Object, prior *api.Object, ev api.MatchedEvent) {
	c, caused := cause.CauseFrom(ctx)
	staged := caused && c.Kind == cause.KindReaction
	hooks := s.applyingHooks(obj, point, &ev)
	if len(hooks) == 0 && !staged {
		return
	}
	var calls, subs []api.ResolvedHook
	for _, h := range hooks {
		if h.Async != nil {
			subs = append(subs, h)
		} else {
			calls = append(calls, h)
		}
	}
	matches := s.matchSubscriptions(ctx, ev, obj, subs)
	entries := s.hookEntries(ctx, point, obj, prior, &ev, calls)
	if staged {
		s.outbox().stage(c.ID, ev, matches, entries)
		return
	}
	detached := context.WithoutCancel(ctx)
	if len(entries) > 0 {
		actions, err := application.UseService[actionsapi.ActionService](ctx)
		if err != nil {
			actions = nil
		}
		j := s.hooks()
		for _, e := range entries {
			j.put(&e)
			if e.State != hookPending {
				continue
			}
			if actions == nil {
				slog.Warn("cwm: hook kept for the next start — no action service", "point", point, "uri", e.URI, "object", obj.ID)
				continue
			}
			s.dispatchHook(detached, actions, e)
		}
	}
	s.recordMatches(detached, matches)
}

// hookEntries builds the journal entry of each on_* call a write owes, under
// its cause; one whose input cannot be built is already failed.
func (s *Service) hookEntries(ctx context.Context, point api.HookPoint, obj api.Object, prior *api.Object, ev *api.MatchedEvent, hooks []api.ResolvedHook) []hookJournalEntry {
	if len(hooks) == 0 {
		return nil
	}
	call := hookCall(point, obj, prior, ev)
	depth := cause.DepthFrom(ctx) + 1
	out := make([]hookJournalEntry, 0, len(hooks))
	for _, h := range hooks {
		input, err := s.hookInput(h, call)
		e := newHookEntry(point, h.URI, obj.ID, input)
		e.Cause = cause.Cause{Kind: cause.KindHook, ID: e.ID, Depth: depth}
		if err != nil {
			// A declaration that cannot build its input fails every attempt alike.
			e.State, e.Error = hookFailed, "input: "+err.Error()
			slog.Warn("cwm: hook input", "point", point, "uri", h.URI, "object", obj.ID, "err", err)
		}
		out = append(out, *e)
	}
	return out
}

// writeEvent is a write's event as a match carries it: its event type is
// object.<action> (object.created …); its payload is {operation} when the
// write is part of an operation (api.WithOperation: a move, a split, a
// transition), empty otherwise.
func writeEvent(ctx context.Context, meta api.EventMeta, action, module string, obj api.Object) api.MatchedEvent {
	ev := api.MatchedEvent{EventMeta: meta, Action: action, Type: obj.Type, ObjectID: obj.ID, Module: module, Event: "object." + action}
	if op := api.OperationFrom(ctx); op != "" {
		ev.Payload, _ = json.Marshal(map[string]string{"operation": op})
	}
	return ev
}

// RunBootstrapHooks dispatches the `bootstrap` point for the project: every
// applying entry on the project object runs through the ActionService, in
// declaration order. Extensions declare what a project needs when it opens
// (the inbox extension, sys-inbox) — as a `hook` contribution whose filter
// admits the project level (the `project` capability its type carries).
//
// It is called once the service is REACHABLE (cwm/local, after construction):
// a bootstrap action calls CWM back, which it cannot do while the service is
// still being built. The errors are returned, one per failing entry; the
// project opens either way.
//
// part says which entries run (BootstrapPart): the binary that holds CWM runs
// the ones it serves at boot, and the ones another sidecar serves once it
// answers on the bus.
//
// It is also the start for the on_* calls a previous process journaled and
// did not see answered (hooks_journal.go), and for the matches it recorded and
// did not see taken (matches.go): they are replayed first, in the background,
// by the pass that runs this binary's own entries.
func (s *Service) RunBootstrapHooks(ctx context.Context, part BootstrapPart) []error {
	// A refused project opens nothing: no change set finished
	// no hook run - its refusal is the one answer.
	if s.refused != nil {
		return []error{s.refused}
	}
	if part != BootstrapOthers {
		// A change-set commit a previous process logged and did not finish is
		// finished first: its writes, then its events (changeset.go).
		s.changeSets()
		if actions, err := application.UseService[actionsapi.ActionService](ctx); err == nil && actions != nil {
			s.replayPendingHooks(context.WithoutCancel(ctx), actions)
		}
		// The matches a previous process recorded and did not see taken.
		s.replayPendingMatches(context.WithoutCancel(ctx))
	}
	// A project is an object at the project level - the `project` capability
	// its type (or its own group) carries - whatever its type is called (a
	// software project, a dossier d'architecture): a list by the type named
	// project found none of them, and no typed project got its inbox.
	projects, err := s.ObjectList(ctx, api.ObjectListInput{Capability: api.CapProject})
	if err != nil {
		return []error{err}
	}
	var errs []error
	for _, p := range projects.Objects {
		var hooks []api.ResolvedHook
		for _, h := range s.applyingHooks(p, api.HookBootstrap, nil) {
			if part.runs(ctx, h.URI) {
				hooks = append(hooks, h)
			}
		}
		if len(hooks) == 0 {
			continue
		}
		actions, err := application.UseService[actionsapi.ActionService](ctx)
		if err != nil || actions == nil {
			return append(errs, fmt.Errorf("bootstrap %q: %d hook(s) declared and no action service to run them", p.ID, len(hooks)))
		}
		for _, h := range hooks {
			input, err := s.hookInput(h, hookCall(api.HookBootstrap, p, nil, nil))
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if _, err := actions.Run(ctx, actionsapi.RunInput{URI: h.URI, Input: input}); err != nil {
				errs = append(errs, fmt.Errorf("bootstrap %s: %w", h.URI, err))
			}
		}
	}
	return errs
}

// BootstrapPart is which of the `bootstrap` entries a pass runs, split by who
// serves the entry's action.
type BootstrapPart int

const (
	// BootstrapAll runs every entry.
	BootstrapAll BootstrapPart = iota
	// BootstrapOwn runs the entries this binary serves: they run in-process,
	// so the project has its system modules before any command runs on it.
	BootstrapOwn
	// BootstrapOthers runs the entries another sidecar serves. Such an action
	// calls CWM back over the bus, which only a process that answers on the
	// bus can take: run before that, each call waits out its request timeout
	// and holds back the process the hub is waiting for.
	BootstrapOthers
)

// runs reports whether the entry naming uri belongs to this pass. An entry
// whose provider cannot be read, or a binary that does not know its own
// provider, counts as this binary's own: it runs at boot.
func (part BootstrapPart) runs(ctx context.Context, uri string) bool {
	if part == BootstrapAll {
		return true
	}
	own := true
	if self, ok := application.SelfProviderFromContext(ctx); ok {
		if u, err := mbapi.ParseToolURI(uri); err == nil {
			own = u.Provider == self
		}
	}
	return own == (part == BootstrapOwn)
}
