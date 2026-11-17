// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// A QUEUE WAIT IN PLANIFICATION'S WORDS - the
// scheduler answers why a ready to-do waits as a structured wait (its cause and
// what holds it, schedapi.QueueWait) and gives it no meaning; planification
// names it once, here, as a kind K1-K6 and who moves it. planning check's ready
// and the wait namer planification contributes to the pull both read it, so a
// pull's waiting line and the check name one wait alike.

// waitKind is a queue wait in planification's words: its kind, why, who
// moves it and since when ("" keeps the to-do's).
type waitKind struct {
	Kind, Reason, Actor, ActorKind, Since string
	// Cause is an incident's cause, Act the person's act (api.Blocker).
	Cause, Act string
}

// on puts the words on a blocker; why, when given, is the scheduler's own
// line, kept as the reason - after the « an unclassified wait: » head for a
// wait no kind names, which says it is one.
func (w waitKind) on(b api.Blocker, why string) api.Blocker {
	b.Kind, b.Reason, b.Actor, b.ActorKind, b.Cause, b.Act = w.Kind, w.Reason, w.Actor, w.ActorKind, w.Cause, w.Act
	switch {
	case why != "" && w.Kind == api.BlockerIncident:
		b.Reason = unclassifiedWait + why
	case why != "":
		b.Reason = why
	}
	if w.Since != "" {
		b.Since = w.Since
	}
	return b
}

// label is what a pull's waiting line starts with: the kind's K-number, the
// kind and who moves it.
func (w waitKind) label() string {
	return api.BlockerClass(w.Kind) + " " + w.Kind + " by " + w.Actor
}

// queueWaitWords is the one mapping from a ready to-do's queue wait to its
// kind and actor. Pure - no store, no CWM read: the namer runs outside the
// store's hold.
//   - a person's step, whatever holds it: K1 for what the person gives (an
//     answer, no interaction), K2 for what the person does on a result
//     (validate, challenge, decide); the person it is given to, if any;
//   - lock: K4, held by the worker holding the module and its request;
//   - reserved: K4, held by the to-do ahead in the queue that reserves it;
//   - backoff: K4, held by the scheduler's back-off until it ends;
//   - assigned: K3, ready for the worker it is given to;
//   - members: K3, ready for a member of its holding list;
//   - any other cause: K6, an unclassified wait.
func queueWaitWords(t api.WorkTodo, w schedapi.QueueWait) waitKind {
	if t.For == schedapi.ForPerson {
		out := waitKind{Kind: api.BlockerAnswer, Reason: "the person's answer: " + stepTitle(t), Actor: "the person", ActorKind: api.ActorPerson}
		if i := t.Packet.Interaction; i != "" && i != api.InteractionProvide {
			out.Kind, out.Act, out.Reason = api.BlockerAct, i, "the person's "+i+": "+stepTitle(t)
		}
		if t.Assignee != nil {
			out.Actor = t.Assignee.ID
		}
		return out
	}
	of := func(name string) string {
		if w.HolderRequest != "" {
			return name + " (" + w.HolderRequest + ")"
		}
		return name
	}
	switch w.Cause {
	case schedapi.WaitLock:
		out := waitKind{Kind: api.BlockerHeld, Reason: "module " + w.Module + " is in use", ActorKind: api.ActorAgent}
		switch {
		case w.Holder != nil:
			out.Actor, out.ActorKind = of(w.Holder.ID), actorKind(*w.Holder)
		case w.HolderTodo != "":
			out.Actor = of(w.HolderTodo)
		default:
			out.Actor = "the worker holding module " + w.Module
		}
		return out
	case schedapi.WaitReserved:
		return waitKind{Kind: api.BlockerHeld, Reason: "module " + w.Module + " is reserved ahead in the queue",
			Actor: of(orDefault(w.HolderTodo, "a to-do")) + ", ready ahead in the queue", ActorKind: api.ActorAgent}
	case schedapi.WaitBackoff:
		return waitKind{Kind: api.BlockerHeld, Reason: "retried after " + w.Until, Actor: "the scheduler's back-off",
			ActorKind: api.ActorSystem, Since: w.Until}
	case schedapi.WaitAssigned:
		out := waitKind{Kind: api.BlockerReady, Reason: "given to one worker: only it takes it", Actor: "the worker it is given to", ActorKind: api.ActorAgent}
		if w.Holder != nil {
			out.Actor, out.ActorKind = w.Holder.ID, actorKind(*w.Holder)
		}
		return out
	case schedapi.WaitMembers:
		return waitKind{Kind: api.BlockerReady, Reason: "held for the members of " + w.Group, Actor: "a member of " + w.Group, ActorKind: api.ActorAgent}
	}
	return waitKind{Kind: api.BlockerIncident, Cause: api.CauseUnclassified, Reason: unclassifiedWait + "the queue's " + orDefault(w.Cause, "unnamed") + " wait",
		Actor: "the reconciler", ActorKind: api.ActorSystem}
}

// NameWait is planification's wait namer (schedapi.WaitNamer): the label a
// pull's waiting line of one of its to-dos starts with - the kind and who
// moves it, as planning check names that wait (queueWaitWords).
func NameWait(_ context.Context, t schedapi.Todo, w schedapi.QueueWait) string {
	return queueWaitWords(decode(t), w).label()
}
