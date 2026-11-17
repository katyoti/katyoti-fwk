// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE EVENT A POINT FIRES ON. A hook entry's `when`, its filter's `when`, its
// `input` and an async entry's `key` speak the hook vocabulary
// (el.HookVocabulary): the object's, plus `event` — {type, id, at, cause,
// payload} — the event the point fires on: an object write's (type
// object.created / updated / deleted, payload {operation} for a write that is
// part of a move, a split or a transition, empty otherwise), a move's
// (object.transitioned / object.transition-refused, payload the event as
// published) or a raised one (EventRaise: its type, its payload). A point
// that fires on no event (a
// before_* gate, an on-demand run) has none: an expression that reads `event`
// there does not hold. Listing the table (ObjectHooks) reads no event either:
// such an entry is listed as applying, "evaluated at the event".

// hookExprs compiles every hook expression against the hook vocabulary.
var hookExprs = el.NewEnvCache(el.HookVocabulary())

var (
	// errAtEvent: the expression reads `event` and the table is listed with
	// no event — it is decided at each event.
	errAtEvent = errors.New("evaluated at the event")
	// errNoEvent: the expression reads `event` at a point that fires on none.
	errNoEvent = errors.New("reads event: this point fires on no event")
)

// hookAt is what a hook table is resolved at: the event's environment (nil:
// none), and whether it is only listed (ObjectHooks).
type hookAt struct {
	event   map[string]any
	listing bool
}

// hookAtEvent is the table resolved at ev (nil: at no event).
func hookAtEvent(ev *api.MatchedEvent) hookAt { return hookAt{event: eventEnv(ev)} }

// eventEnv is `event` as an expression reads it; nil for no event.
func eventEnv(ev *api.MatchedEvent) map[string]any {
	if ev == nil {
		return nil
	}
	var payload any = map[string]any{}
	if len(ev.Payload) > 0 {
		var v any
		if json.Unmarshal(ev.Payload, &v) == nil && v != nil {
			payload = v
		}
	}
	var c any
	if ev.Cause != nil {
		c = map[string]any{"kind": ev.Cause.Kind, "id": ev.Cause.ID, "depth": ev.Cause.Depth}
	}
	at := ""
	if !ev.At.IsZero() {
		at = ev.At.UTC().Format(time.RFC3339Nano)
	}
	return map[string]any{"type": ev.Event, "id": ev.EventID, "at": at, "cause": c, "payload": payload}
}

// hookEnv is the hook vocabulary's environment for obj at an event (nil:
// `event` reads as an empty map — an input template resolves it to nothing).
func (s *Service) hookEnv(obj api.Object, event map[string]any) map[string]any {
	env := s.objectActionEnv(obj)
	if event == nil {
		event = map[string]any{}
	}
	env["event"] = event
	return env
}

// readsEvent reports an expression that needs `event`: it compiles in the hook
// vocabulary and not in the object's.
func readsEvent(expr string) bool {
	if _, err := objectExprs.Compile(expr); err == nil {
		return false
	}
	_, err := hookExprs.Compile(expr)
	return err == nil
}

// hookSubject is filterSubject for a hook table: its `when`s speak the hook
// vocabulary at `at`.
func (s *Service) hookSubject(obj api.Object, effective []api.CapabilityRef, at hookAt) api.FilterSubject {
	var env map[string]any
	envOf := func() map[string]any {
		if env == nil {
			env = s.hookEnv(obj, at.event)
		}
		return env
	}
	subject := s.filterSubjectIn(obj, effective, hookExprs, envOf)
	subject.Eval = func(expr string) (bool, error) {
		if at.event == nil && readsEvent(expr) {
			if at.listing {
				return false, errAtEvent
			}
			return false, errNoEvent
		}
		return hookExprs.EvalPredicate(expr, envOf())
	}
	return subject
}

// filterAtEvent reports a filter refusal that only says its `when` is decided
// at the event (a listed table).
func filterAtEvent(why string) bool { return why == "when: "+errAtEvent.Error() }
