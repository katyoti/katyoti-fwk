// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE SUBSCRIPTIONS TO AN EVENT TYPE (api/event_subscriptions.go). The same
// layers resolveHooks lays in an object's table, read WITHOUT an object: the
// module objects' cross-module entries, the `hook` contributions, the
// capabilities, the policies, the object types, then each object's own
// entries (resolved on that object). Only async entries at the type's point
// are listed — a subscription; a sync on_* entry is a hook CWM runs. Each
// declaration is listed once: which closer layer replaces or removes a
// farther entry is an object's business, read with ObjectHooks.

// Reasons an entry is listed as applying while something else decides.
const (
	subReasonAtEvent   = "evaluated at the event"
	subReasonPerObject = "decided per object: when "
	subReasonShared    = "fires for every event at "
	subReasonNoPoint   = "not subscribable: the event type names no point"
)

// EventSubscriptions lists the subscriptions to one event type, or to every
// declared one, farthest layer first.
func (s *Service) EventSubscriptions(_ stdctx.Context, in api.EventSubscriptionsInput) (api.EventSubscriptionsOutput, error) {
	var types []api.EventType
	if in.Event != "" {
		t, ok := s.eventTypeNamed(in.Event)
		if !ok {
			return api.EventSubscriptionsOutput{}, fmt.Errorf("%w: EventSubscriptions: event type %q is not declared (no event-type names it)", api.ErrInvalidInput, in.Event)
		}
		types = []api.EventType{t}
	} else {
		types = s.declaredEventTypes()
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.EventSubscriptionsOutput{}, err
	}
	// How many event types name each point: an entry that is not a
	// cross-module one fires for every event at its point.
	byPoint := map[api.HookPoint]int{}
	for _, t := range s.declaredEventTypes() {
		if t.Point != "" {
			byPoint[t.Point]++
		}
	}
	out := api.EventSubscriptionsOutput{Events: make([]api.EventTypeSubscriptions, 0, len(types))}
	for _, t := range types {
		e := api.EventTypeSubscriptions{Event: t.Name, Point: t.Point, Published: t.Published, Provider: t.Provider,
			Payload: t.Payload, Subscriptions: []api.EventSubscription{}}
		if t.Point == "" {
			e.Reason = subReasonNoPoint
		} else {
			e.Subscriptions = s.subscriptionsTo(t, byPoint[t.Point] > 1)
		}
		out.Events = append(out.Events, e)
	}
	return out, nil
}

// declaredEventTypes is every declared event type, each name once: CWM's own,
// then the packs' in catalog order (eventTypeNamed's precedence).
func (s *Service) declaredEventTypes() []api.EventType {
	seen := map[string]bool{}
	var out []api.EventType
	for _, list := range [][]api.EventType{api.BuiltinEventTypes, s.decl().EventTypes} {
		for _, t := range list {
			if t.Name == "" || seen[t.Name] {
				continue
			}
			seen[t.Name] = true
			out = append(out, t)
		}
	}
	return out
}

// subscriptionsTo is every async entry subscribing to t, by layer, farthest
// first. shared: several event types name t's point.
func (s *Service) subscriptionsTo(t api.EventType, shared bool) []api.EventSubscription {
	point := t.Point
	var out []api.EventSubscription
	// A declared entry of a layer that is not a module's nor an object's.
	add := func(source, provider, declaredOn string, entry api.HookEntry, refused string) {
		if entry.URI == "" || entry.Async == nil || entry.On != nil || entry.When == "false" {
			return
		}
		sub := subscriptionOf(source, provider, declaredOn, point, entry)
		if refused != "" {
			sub.Applies, sub.Reason = false, refused
		} else {
			sub.Reason = staticReason(entry, point, shared)
		}
		out = append(out, sub)
	}

	// 1 — the cross-module subscriptions naming the type, every emitter.
	if subs := s.moduleSubscriptions(); subs != nil {
		decls := make([]moduleSubDecl, 0, len(subs.byID))
		for _, d := range subs.byID {
			if d.Entry.On.Event == t.Name {
				decls = append(decls, d)
			}
		}
		sort.Slice(decls, func(a, b int) bool { return decls[a].id() < decls[b].id() })
		for _, d := range decls {
			e := d.Entry
			sub := subscriptionOf(api.ModuleSubscriptionSource(d.Subscriber), "", d.Subscriber, d.Point, e)
			on := *e.On
			sub.On, sub.Writes = &on, d.writes()
			if why := s.moduleSubRefusal(d); why != "" {
				sub.Applies, sub.Reason = false, why
			} else {
				sub.Reason = staticReason(e, d.Point, false)
			}
			out = append(out, sub)
		}
	}
	// 2 — the `hook` contributions at the point.
	for _, h := range s.decl().Hooks {
		if h.Type != point {
			continue
		}
		entry := api.HookEntry{URI: h.URI, When: h.When, Input: h.Input, Async: h.Async}
		if f := h.Filter; len(f.Types)+len(f.Capabilities)+len(f.Incoming)+len(f.Outgoing) > 0 || f.When != "" {
			f := h.Filter
			entry.Filter = &f
		}
		add(h.Source(), h.Provider, h.Name, entry, "")
	}
	// 3 — the capabilities.
	for _, c := range s.decl().Capabilities {
		for _, e := range c.Hooks[point] {
			add("capability:"+c.Name, c.Provider, c.Name, e, "")
		}
	}
	// 4 — the policies: one that applies to no object never matches.
	seen := map[string]bool{}
	for _, p := range s.decl().Policies {
		if p.Name == "" || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		refused := ""
		if p.Applies.IsZero() {
			_, refused = p.AppliesTo("", nil)
		}
		for _, e := range p.Hooks[point] {
			add("policy:"+p.Name, p.Provider, p.Name, e, refused)
		}
	}
	// 5 — the object types.
	for _, ot := range s.decl().Objects {
		for _, e := range ot.Hooks[point] {
			add("type:"+ot.Name, ot.Provider, ot.Name, e, "")
		}
	}
	// 6 — each object's own entries, resolved on it: its `when` reads the
	// object, so what it answers is the object's.
	for _, obj := range s.index().objectsWithHooks() {
		if len(obj.Hooks[point]) == 0 {
			continue
		}
		for _, r := range s.resolveHooks(obj, s.effectiveRefs(obj), point, hookAt{listing: true}) {
			if r.Source != "object" || r.Async == nil {
				continue // another layer's (listed above), or a hook CWM runs
			}
			entry := ownEntry(obj, point, r.URI)
			sub := subscriptionOf("object:"+obj.ID, "", obj.ID, point, entry)
			sub.Applies = r.Applies
			switch {
			case !r.Applies:
				sub.Reason = r.Reason
			case strings.HasSuffix(r.Reason, subReasonAtEvent):
				sub.Reason = joinReasons(subReasonAtEvent, sharedReason(point, shared))
			default:
				sub.Reason = joinReasons(r.Reason, sharedReason(point, shared))
			}
			out = append(out, sub)
		}
	}
	return out
}

// subscriptionOf is one entry as a subscription, applying until a rule says
// otherwise.
func subscriptionOf(source, provider, declaredOn string, point api.HookPoint, e api.HookEntry) api.EventSubscription {
	var a api.AsyncSpec
	if e.Async != nil {
		a = e.Async.WithDefaults()
	}
	return api.EventSubscription{Subscription: api.SubscriptionID(source, point, e.URI), Source: source, Provider: provider,
		DeclaredOn: declaredOn, URI: e.URI, When: e.When, Filter: e.Filter, Async: a, Applies: true}
}

// ownEntry is the object's own entry at point running uri: the closest one,
// as its table keeps it.
func ownEntry(obj api.Object, point api.HookPoint, uri string) api.HookEntry {
	es := obj.Hooks[point]
	for i := len(es) - 1; i >= 0; i-- {
		if es[i].URI == uri && es[i].When != "false" {
			return es[i]
		}
	}
	return api.HookEntry{URI: uri}
}

// staticReason says what decides an entry that applies when no object is
// asked: the event (its or its filter's `when` reads `event`), each object
// (another `when`), and — at a point several event types share — every event
// at that point.
func staticReason(e api.HookEntry, point api.HookPoint, shared bool) string {
	filterWhen := ""
	if e.Filter != nil {
		filterWhen = e.Filter.When
	}
	var why string
	switch {
	case (e.When != "" && readsEvent(e.When)) || (filterWhen != "" && readsEvent(filterWhen)):
		why = subReasonAtEvent
	case e.When != "":
		why = subReasonPerObject + e.When
	case filterWhen != "":
		why = subReasonPerObject + filterWhen
	}
	return joinReasons(why, sharedReason(point, shared))
}

// sharedReason is the shared-point reason, "" when the point is one type's.
func sharedReason(point api.HookPoint, shared bool) string {
	if !shared {
		return ""
	}
	return subReasonShared + string(point)
}

// joinReasons joins the non-empty reasons with "; ".
func joinReasons(rs ...string) string {
	var keep []string
	for _, r := range rs {
		if r != "" {
			keep = append(keep, r)
		}
	}
	return strings.Join(keep, "; ")
}
