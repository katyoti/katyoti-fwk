// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmevents "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/events"
)

// transitionExprs compiles workflow gates against the transition vocabulary —
// the object's, plus `by`, `from`, `to` — so a gate naming anything else is
// refused, never read as nil.
var transitionExprs = el.NewEnvCache(el.TransitionVocabulary())

// transitionKey marks a ctx as carrying a transition, so ObjectUpdate lets the
// lifecycle state change: every other write that would change it is refused.
type transitionKey struct{}

// ObjectTransition moves an object to another state of a workflow: its own
// lifecycle, or — with Capability — the state of a capability that declares a
// workflow (lifecycle_slot.go). The engine is a VALIDATOR: the transition must
// exist from the current state, the state must not be terminal, the gate must
// hold (or be forced where the transition allows it). Then `on_transition`
// hooks are asked (they may refuse), the state is written through
// ObjectUpdate — so before_update / on_update run too, the
// write marked operation transition — and one `transitioned` event is
// published, then fired at on_transitioned (its async entries matched, its
// plain ones run). The move is not written on the object: the event is the
// journal's `transitioned` record — who, when, the note, forced and what the
// guard's functions answered — and the object's history is read from it. A move its guard refuses publishes `transition-refused`,
// fired at on_transition_refused on the unchanged object. A dry run makes
// every check and stops before the write: the moved object is validated as
// the write would, no hook is asked, nothing is written and no event is
// published or fired.
func (s *Service) ObjectTransition(ctx stdctx.Context, in api.ObjectTransitionInput) (api.ObjectTransitionOutput, error) {
	// A transition that lost a race is redone from the state the object is in
	// now: the second of two moves from one state is then judged — and
	// refused — like any move from the new state.
	return retryOnConflict(func() (api.ObjectTransitionOutput, error) { return s.objectTransition(ctx, in) })
}

func (s *Service) objectTransition(ctx stdctx.Context, in api.ObjectTransitionInput) (api.ObjectTransitionOutput, error) {
	switch {
	case in.ID == "" || in.To == "":
		return api.ObjectTransitionOutput{}, fmt.Errorf("%w: id and to required", api.ErrInvalidInput)
	case in.NoCostly && !in.DryRun:
		return api.ObjectTransitionOutput{}, fmt.Errorf("%w: no_costly previews a move — it goes with dry_run; a move runs every function its guard calls", api.ErrInvalidInput)
	case in.By.ID == "" || (in.By.Kind != api.ActorPerson && in.By.Kind != api.ActorAgent):
		return api.ObjectTransitionOutput{}, fmt.Errorf("%w: by {id, kind: person|agent} required — every transition records who made it", api.ErrInvalidInput)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectTransitionOutput{}, err
	}
	// fullObjectRev answers not found; a reaction's staged object is its own.
	obj, module, rev, err := s.fullObjectRev(ctx, in.ID)
	if err != nil {
		return api.ObjectTransitionOutput{}, err
	}
	slot, err := s.lifecycleSlotOf(obj, in.Capability)
	if err != nil {
		return api.ObjectTransitionOutput{}, err
	}
	wf, ok := s.workflowNamed(slot.workflow)
	if !ok {
		return api.ObjectTransitionOutput{}, fmt.Errorf("%w: workflow %q", api.ErrNotFound, slot.workflow)
	}

	from := slot.state
	if from == "" {
		from = wf.Initial
	}
	if st, ok := wf.StateNamed(from); ok && st.Terminal {
		return api.ObjectTransitionOutput{}, api.Invalid(fmt.Errorf("%w: %q is in %s, a terminal state of %s", api.ErrTransitionRefused, in.ID, from, wf.Name),
			"cwm.transitionFromTerminal", map[string]any{"id": in.ID, "state": from, "workflow": wf.Name})
	}
	if _, ok := wf.StateNamed(in.To); !ok {
		return api.ObjectTransitionOutput{}, api.Invalid(fmt.Errorf("%w: %s is not a state of %s", api.ErrTransitionRefused, in.To, wf.Name),
			"cwm.transitionUnknownState", map[string]any{"state": in.To, "workflow": wf.Name})
	}
	candidates := matchingTransitions(wf, from, in.To, in.Transition)
	if len(candidates) == 0 {
		return api.ObjectTransitionOutput{}, api.Invalid(fmt.Errorf("%w: %s has no transition from %s to %s", api.ErrTransitionRefused, wf.Name, from, in.To),
			"cwm.transitionNone", map[string]any{"workflow": wf.Name, "from": from, "to": in.To})
	}
	// Asked by its target state, a move takes the first matching transition
	// whose guard holds (a realized feature: a person's confirm, else an
	// agent's verify); none holding, the first one is refused as before.
	tr := candidates[0]
	if len(candidates) > 1 {
		for _, c := range candidates {
			if c.Gate == "" {
				tr = c
				break
			}
			env := s.gateEnv(ctx, obj, in.By, from, in.To, in.DryRun && in.NoCostly)
			if pass, err := s.transitionCache().EvalPredicate(c.Gate, env); err == nil && pass {
				tr = c
				break
			}
		}
	}

	// The guard: an el predicate, the capabilities' functions bound
	// (gate('ready-to-build'), verify()). A refusal says why with what each
	// function answered, and is published so a planner can plan what makes
	// it hold. Forcing past it is the transition's to allow (force: anyone,
	// or person — a person, with a note); a guard that could not be
	// evaluated is never forced.
	forced := false
	var forcedCalls []api.FunctionAnswer
	if tr.Gate != "" {
		env := s.gateEnv(ctx, obj, in.By, from, in.To, in.DryRun && in.NoCostly)
		pass, err := s.transitionCache().EvalPredicate(tr.Gate, env)
		if err != nil || !pass {
			refused := &api.TransitionRefusedError{Object: in.ID, Transition: tr.Name, From: from, To: in.To, Guard: tr.Gate, Calls: callsOf(env)}
			if err != nil {
				refused.Reason = fmt.Sprintf("the guard %q: %v", tr.Gate, err)
			} else {
				refused.Reason = fmt.Sprintf("the guard %q does not hold%s", tr.Gate, calledFunctions(env))
			}
			if err == nil && in.Force && tr.Force.Allows(in.By, in.Note) {
				forced, forcedCalls = true, refused.Calls
			} else {
				if !in.DryRun { // a question is not a refused move: nobody plans from it
					// Stamped like a raised event: the object's CURRENT
					// revision (read, unchanged — a refusal writes nothing).
					p := cwmevents.ObjectTransitionRefusedPayload{
						EventMeta: s.eventMeta(ctx, rev),
						ObjectID:  in.ID, Module: module, Type: obj.Type, Workflow: wf.Name, Capability: slot.capability,
						Transition: tr.Name, From: from, To: in.To, By: in.By, Guard: tr.Gate, Reason: refused.Reason, Calls: refused.Calls,
					}
					s.Events.PublishObjectTransitionRefused(p)
					s.fireAfterHooks(ctx, api.HookOnTransitionRefused, obj, nil,
						moveEvent(p.EventMeta, api.TransitionRefusedEventAction, "object.transition-refused", module, obj, p))
				}
				switch {
				case in.Force && err == nil && tr.Force == api.ForceNone:
					return api.ObjectTransitionOutput{}, api.Invalid(fmt.Errorf("%w (this transition does not allow forcing)", refused),
						"cwm.forceNotAllowed", map[string]any{"transition": tr.Name, "reason": refused.Reason})
				case in.Force && err == nil:
					return api.ObjectTransitionOutput{}, api.Invalid(fmt.Errorf("%w (forcing it takes a person and a note)", refused),
						"cwm.forceNeedsPerson", map[string]any{"transition": tr.Name, "reason": refused.Reason})
				}
				return api.ObjectTransitionOutput{}, refused
			}
		}
	}

	prior := obj
	prior.Capabilities = cloneCaps(obj.Capabilities)
	next := obj
	next.Capabilities = cloneCaps(obj.Capabilities)
	slot.state = in.To
	slot.record(api.LifecycleEntry{
		From: from, To: in.To, Transition: tr.Name, By: in.By,
		At: now(), Note: in.Note, Forced: forced, Calls: forcedCalls,
	})
	if err := slot.write(&next); err != nil {
		return api.ObjectTransitionOutput{}, err
	}
	if in.DryRun {
		if err := s.checkWrite(next, &prior, "ObjectTransition"); err != nil {
			return api.ObjectTransitionOutput{}, err
		}
		if path, ok := ix.pathOf(in.ID); ok {
			if err := s.checkSizeBudget(path, next, "ObjectTransition"); err != nil {
				return api.ObjectTransitionOutput{}, err
			}
		}
		return api.ObjectTransitionOutput{Object: next, From: from, To: in.To, Transition: tr.Name, Forced: forced, DryRun: true}, nil
	}

	// on_transition may refuse, like a before_* gate.
	if err := s.runBeforeHooks(ctx, api.HookOnTransition, next, &prior); err != nil {
		return api.ObjectTransitionOutput{}, err
	}
	// The move's own write: on_update fires on it as on any write, marked
	// operation transition (event.payload.operation) so an on_update
	// subscriber that wants only edits can skip it.
	wctx := api.WithOperation(stdctx.WithValue(ctx, transitionKey{}, true), api.OperationTransition)
	set := s.openSet(ctx)
	upd, err := s.ObjectUpdate(wctx, api.ObjectUpdateInput{Module: module, Object: next, Revision: rev})
	if err != nil {
		return api.ObjectTransitionOutput{}, err
	}
	if set != nil {
		// Staged in the reaction's change set: the move's event goes out, and
		// on_transitioned fires, when the set is committed (changeset.go).
		s.stageTransition(set, in.ID, cwmevents.ObjectTransitionedPayload{
			ObjectID: in.ID, Module: module, Type: obj.Type, Workflow: wf.Name, Capability: slot.capability,
			From: from, To: in.To, Transition: tr.Name, By: in.By, Note: in.Note, Forced: forced, Calls: forcedCalls,
		}, prior, next)
		return api.ObjectTransitionOutput{Object: next, From: from, To: in.To, Transition: tr.Name, Forced: forced}, nil
	}
	// ONE event id: the event published and the matches at on_transitioned
	// name the same event.
	p := cwmevents.ObjectTransitionedPayload{
		EventMeta: s.eventMeta(ctx, upd.Revision),
		ObjectID:  in.ID, Module: module, Type: obj.Type, Workflow: wf.Name, Capability: slot.capability,
		From: from, To: in.To, Transition: tr.Name, By: in.By, Note: in.Note, Forced: forced, Calls: forcedCalls,
	}
	s.Events.PublishObjectTransitioned(p)
	s.fireAfterHooks(ctx, api.HookOnTransitioned, next, &prior,
		moveEvent(p.EventMeta, api.TransitionedEventAction, "object.transitioned", module, next, p))
	return api.ObjectTransitionOutput{Object: next, From: from, To: in.To, Transition: tr.Name, Forced: forced}, nil
}

// moveEvent is a move's event as a hook reads it (`event`) and a match carries
// it: its payload is the event as published — from, to, by … and, for a
// refusal, the guard, the reason and the calls.
func moveEvent(meta api.EventMeta, action, event, module string, obj api.Object, payload any) api.MatchedEvent {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = nil
	}
	return api.MatchedEvent{EventMeta: meta, Action: action, Type: obj.Type, ObjectID: obj.ID, Module: module, Event: event, Payload: raw}
}

// matchingTransitions are the transitions from `from` to `to` (by name when
// given), in the workflow's order.
func matchingTransitions(wf api.Workflow, from, to, name string) []api.WorkflowTransition {
	var out []api.WorkflowTransition
	for _, t := range wf.Transitions {
		if t.To != to || (name != "" && t.Name != name) {
			continue
		}
		if slices.Contains(t.From, from) || slices.Contains(t.From, "*") {
			out = append(out, t)
		}
	}
	return out
}

// workflowNamed looks a workflow up in the catalog.
func (s *Service) workflowNamed(name string) (api.Workflow, bool) {
	for _, w := range s.decl().Workflows {
		if w.Name == name {
			return w, true
		}
	}
	return api.Workflow{}, false
}

// lifecycleStateChanged reports an update that would change a workflow state —
// the object's own or a capability's — without a transition: refused, so the
// gates cannot be walked around. It names the slot that changed ("" when none).
func (s *Service) lifecycleStateChanged(ctx stdctx.Context, prior *api.Object, next api.Object) (string, bool) {
	if ctx.Value(transitionKey{}) != nil || prior == nil {
		return "", false
	}
	before := s.workflowStates(*prior)
	after := s.workflowStates(next)
	// A slot that had a state keeps it: a different state, or the slot gone,
	// is a change.
	for slot, old := range before {
		if old != "" && after[slot] != old {
			return slot, true
		}
	}
	// An EMPTY state is the workflow's initial one — an object never moved, a
	// capability just given. Leaving it, or a slot appearing anywhere but in
	// its initial state, is a change too: otherwise writing `done` on a new
	// object walks around every guard.
	for slot, now := range after {
		if now == "" || before[slot] != "" {
			continue
		}
		if initial, ok := s.initialStateOf(next, slot); ok && now != initial {
			return slot, true
		}
	}
	return "", false
}

// initialStateOf is the initial state of the workflow slot follows on obj;
// false when obj names no declared workflow for it (nothing gates it).
func (s *Service) initialStateOf(obj api.Object, slot string) (string, bool) {
	ls, err := s.lifecycleSlotOf(obj, slot)
	if err != nil {
		return "", false
	}
	wf, ok := s.workflowNamed(ls.workflow)
	if !ok {
		return "", false
	}
	return wf.Initial, true
}

// statesOutsideWorkflow lists, per slot, a state its declared workflow does not
// have — what a write refuses to ADD. A workflow nobody declares is not
// judged here: that is reported on read (lifecycleProblem), never refused, so
// an object whose pack is absent stays writable.
func (s *Service) statesOutsideWorkflow(obj api.Object) map[string]string {
	out := map[string]string{}
	for slot, state := range s.workflowStates(obj) {
		if state == "" {
			continue
		}
		ls, err := s.lifecycleSlotOf(obj, slot)
		if err != nil {
			continue
		}
		wf, ok := s.workflowNamed(ls.workflow)
		if !ok {
			continue
		}
		if _, ok := wf.StateNamed(state); !ok {
			out[slot] = fmt.Sprintf("%q is not a state of workflow %s", state, wf.Name)
		}
	}
	return out
}

// checkCreateLifecycle refuses a create that would START an object past its
// workflow's initial state — walking around every gate by writing the state at
// birth. An object with no state starts in the initial one.
func (s *Service) checkCreateLifecycle(obj api.Object) error {
	if err := checkRetiredLifecycleFields(nil, obj); err != nil {
		return err
	}
	life, ok := obj.Lifecycle()
	if !ok || life.State == "" {
		return nil
	}
	var name string
	if t, ok := s.lookupObjectType(obj.Type); ok {
		name = t.Workflow
	}
	wf, ok := s.workflowNamed(name)
	if !ok {
		return nil // no workflow to check against: nothing gates this state
	}
	if life.State != wf.Initial {
		return fmt.Errorf("%w: %q would be created in %s; a %s object starts in %s and moves through ObjectTransition",
			api.ErrInvalidInput, obj.ID, life.State, wf.Name, wf.Initial)
	}
	return nil
}

// retiredLifecycleFields are keys the lifecycle group no longer has: an object
// follows its type's workflow (never one it names), and its history is the
// journal's `transitioned` records (never stored on it).
var retiredLifecycleFields = []string{"workflow", "history"}

// checkRetiredLifecycleFields refuses a write that SETS or CHANGES the
// lifecycle group's `workflow` or `history`, like a raw state change: a raw
// `workflow` would swap in a laxer workflow under the same state, a raw
// `history` would forge the moves. An object still carrying one, unchanged,
// stays writable (repaired one write at a time); removing one is always
// allowed.
func checkRetiredLifecycleFields(prior *api.Object, next api.Object) error {
	group := next.Capabilities[api.CapLifecycle]
	var had map[string]any
	if prior != nil {
		had = prior.Capabilities[api.CapLifecycle]
	}
	for _, key := range retiredLifecycleFields {
		v, set := group[key]
		if !set {
			continue
		}
		if old, kept := had[key]; kept && reflect.DeepEqual(old, v) {
			continue
		}
		return api.Invalid(fmt.Errorf("%w: %q: the lifecycle has no %s — an object follows its type's workflow, and its moves are the journal's (ObjectTransition records them)", api.ErrInvalidInput, next.ID, key),
			"cwm.lifecycleRetiredField", map[string]any{"id": next.ID, "field": key})
	}
	return nil
}
