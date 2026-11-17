// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CROSS-MODULE SUBSCRIPTIONS (api/module_subscriptions.go). A module object's
// `hooks:` entry with `on: {module, event}` subscribes the module to another
// module's events (or its own). CWM gathers them from the index once per
// generation of the module objects, lays them as the FARTHEST layer of the
// emitter's objects' hook tables (`module:<subscriber>`, resolveHooks), lists
// them per emitter (ModuleSubscribers) and holds their reactions to the
// modules they declare they write (checkReactionWrites). It records and
// guards; the reactions are the events extension's to run.

// moduleSubDecl is one module object's cross-module entry.
type moduleSubDecl struct {
	Subscriber string
	Point      api.HookPoint
	Entry      api.HookEntry
}

// id is the subscription's id: module:<subscriber>#<point>#<uri>.
func (d moduleSubDecl) id() string {
	return api.SubscriptionID(api.ModuleSubscriptionSource(d.Subscriber), d.Point, d.Entry.URI)
}

// writes is the modules its reactions may write: the declared ones, else the
// subscriber's own.
func (d moduleSubDecl) writes() []string {
	if len(d.Entry.Writes) == 0 {
		return []string{d.Subscriber}
	}
	return append([]string(nil), d.Entry.Writes...)
}

// moduleSubscriptions is every module object's on-entries at one generation:
// by emitting module (in subscriber, point, uri order) and by subscription id.
type moduleSubscriptions struct {
	gen       uint64
	byEmitter map[string][]moduleSubDecl
	byID      map[string]moduleSubDecl
}

// moduleSubscriptions is the gathered declarations, rebuilt when the module
// objects changed (objectIndex.gen). nil when the index cannot load.
func (s *Service) moduleSubscriptions() *moduleSubscriptions {
	ix := s.index()
	if ix.ensureLoaded() != nil {
		return nil
	}
	gen := ix.generation()
	s.moduleSubsMu.Lock()
	defer s.moduleSubsMu.Unlock()
	if s.moduleSubs != nil && s.moduleSubs.gen == gen {
		return s.moduleSubs
	}
	gen, objs := ix.moduleObjects()
	out := &moduleSubscriptions{gen: gen, byEmitter: map[string][]moduleSubDecl{}, byID: map[string]moduleSubDecl{}}
	var all []moduleSubDecl
	for _, e := range objs {
		for point, entries := range e.Object.Hooks {
			for _, h := range entries {
				if h.On == nil || h.URI == "" {
					continue
				}
				all = append(all, moduleSubDecl{Subscriber: e.Object.ID, Point: point, Entry: h})
			}
		}
	}
	sort.SliceStable(all, func(a, b int) bool {
		x, y := all[a], all[b]
		if x.Subscriber != y.Subscriber {
			return x.Subscriber < y.Subscriber
		}
		if x.Point != y.Point {
			return x.Point < y.Point
		}
		return x.Entry.URI < y.Entry.URI
	})
	for _, d := range all {
		if _, dup := out.byID[d.id()]; dup {
			continue // one entry per point and uri (refused at the write); the first stands
		}
		out.byID[d.id()] = d
		out.byEmitter[d.Entry.On.Module] = append(out.byEmitter[d.Entry.On.Module], d)
	}
	s.moduleSubs = out
	return out
}

// eventTypeNamed is the declared event type of that name: CWM's own, then a
// pack's.
func (s *Service) eventTypeNamed(name string) (api.EventType, bool) {
	for _, t := range api.BuiltinEventTypes {
		if t.Name == name {
			return t, true
		}
	}
	for _, t := range s.decl().EventTypes {
		if t.Name == name {
			return t, true
		}
	}
	return api.EventType{}, false
}

// moduleSubRefusal says why a cross-module entry does not apply anywhere —
// not async, no module or event, an undeclared type, a point that is not its
// type's, another module's private type; "" when it applies (its `when` and
// filter still decide per object and event).
func (s *Service) moduleSubRefusal(d moduleSubDecl) string {
	why, _ := s.moduleSubCheck(d)
	return why
}

// moduleSubCheck is moduleSubRefusal, and whether the only fault is a type no
// pack declares (accepted at the write: the pack may come later).
func (s *Service) moduleSubCheck(d moduleSubDecl) (why string, undeclared bool) {
	on := d.Entry.On
	switch {
	case on.Module == "" || on.Event == "":
		return "on: a module and an event are required", false
	case d.Entry.Async == nil:
		return "on: a cross-module entry is a subscription and must be async", false
	}
	et, ok := s.eventTypeNamed(on.Event)
	switch {
	case !ok:
		return "event type " + on.Event + " is not declared (no event-type names it)", true
	case et.Point == "":
		return "event type " + on.Event + " names no point: nothing can subscribe to it", false
	case et.Point != d.Point:
		return "event type " + on.Event + " is at " + string(et.Point) + ", not " + string(d.Point), false
	case !et.Published && on.Module != d.Subscriber:
		return "event type " + on.Event + " is not published", false
	}
	return "", false
}

// checkModuleSubscriptions refuses, at a write, an object whose cross-module
// entries could never be right: one not on a module object, not async, with
// no module or event, at a point that is not its type's, naming another
// module's private event type, or twice at the same point with the same uri.
// An event type no pack declares is accepted — listed as not applying, since
// the pack may arrive later. An update is refused only what it ADDS: an entry
// the stored object already holds as is (its type unpublished since) is kept.
func (s *Service) checkModuleSubscriptions(obj api.Object, prior *api.Object, verb string) error {
	seen := map[string]bool{}
	for point, entries := range obj.Hooks {
		for _, h := range entries {
			if h.On == nil {
				continue
			}
			bad := func(why string) error {
				return fmt.Errorf("%w: %s: %q: hooks.%s %s: %s", api.ErrInvalidInput, verb, obj.ID, point, h.URI, why)
			}
			if !isModuleObjectID(obj.ID) {
				return bad("on: only a module object declares a cross-module subscription")
			}
			d := moduleSubDecl{Subscriber: obj.ID, Point: point, Entry: h}
			if seen[d.id()] {
				return bad("on: one cross-module entry per point and uri (its subscription id " + d.id() + ")")
			}
			seen[d.id()] = true
			if prior != nil && slices.ContainsFunc(prior.Hooks[point], func(p api.HookEntry) bool { return reflect.DeepEqual(p, h) }) {
				continue // stored as is: kept
			}
			if why, undeclared := s.moduleSubCheck(d); why != "" && !undeclared {
				return bad(why)
			}
		}
	}
	return nil
}

// moduleSubLayers is the farthest layer of a hook table: every module
// object's on-entry naming module, one layer each, refused ones kept with
// their reason.
func (s *Service) moduleSubLayers(module string) []hookLayer {
	subs := s.moduleSubscriptions()
	if subs == nil {
		return nil
	}
	decls := subs.byEmitter[module]
	out := make([]hookLayer, 0, len(decls))
	for _, d := range decls {
		e := d.Entry
		e.Writes = d.writes()
		out = append(out, hookLayer{source: api.ModuleSubscriptionSource(d.Subscriber),
			hooks: api.Hooks{d.Point: {e}}, refused: s.moduleSubRefusal(d)})
	}
	return out
}

// moduleOfObject is the module obj is filed in (the index), else the one its
// id names.
func (s *Service) moduleOfObject(id string) string {
	if ix := s.index(); ix.ensureLoaded() == nil {
		if m, ok := ix.moduleOf(id); ok {
			return m
		}
	}
	m, _ := api.ModuleOfObjectID(id)
	return m
}

// checkReactionWrites is the write guard of the cross-module reactions: a
// write whose cause is a reaction of a `module:*` subscription, into a module
// its entry does not declare in `writes` (default the subscriber's own), is
// refused. Every other write — no cause, a hook's, a person's, a reaction of
// a module's own subscription — passes untouched. The declaration is looked
// up from the reaction's id; one no longer declared allows the subscriber's
// own module only.
func (s *Service) checkReactionWrites(ctx stdctx.Context, module, verb string) error {
	c, ok := cause.CauseFrom(ctx)
	if !ok || c.Kind != cause.KindReaction {
		return nil
	}
	rid, ok := api.ParseReactionID(c.ID)
	if !ok {
		return nil
	}
	subscriber, _, _, ok := api.ParseModuleSubscriptionID(rid.Subscription)
	if !ok {
		return nil
	}
	writes := []string{subscriber}
	if subs := s.moduleSubscriptions(); subs != nil {
		if d, found := subs.byID[rid.Subscription]; found {
			writes = d.writes()
		}
	}
	if slices.Contains(writes, module) {
		return nil
	}
	return fmt.Errorf("%w: %s: reaction %s writes module %q, which subscription %s does not declare (writes: %s)",
		api.ErrInvalidInput, verb, c.ID, module, rid.Subscription, strings.Join(writes, ", "))
}

// ModuleSubscribers lists the cross-module subscriptions naming a module:
// each module object's on-entry, its declared writes, and whether it applies
// (Reason says why not).
func (s *Service) ModuleSubscribers(_ stdctx.Context, in api.ModuleSubscribersInput) (api.ModuleSubscribersOutput, error) {
	if in.Module == "" {
		return api.ModuleSubscribersOutput{}, fmt.Errorf("%w: ModuleSubscribers: module required", api.ErrInvalidInput)
	}
	subs := s.moduleSubscriptions()
	if subs == nil {
		if err := s.index().ensureLoaded(); err != nil {
			return api.ModuleSubscribersOutput{}, err
		}
	}
	out := api.ModuleSubscribersOutput{Subscribers: []api.ModuleSubscriber{}}
	if subs == nil {
		return out, nil
	}
	for _, d := range subs.byEmitter[in.Module] {
		if in.Event != "" && d.Entry.On.Event != in.Event {
			continue
		}
		m := api.ModuleSubscriber{Subscriber: d.Subscriber, Event: d.Entry.On.Event, Point: d.Point, URI: d.Entry.URI,
			Subscription: d.id(), Writes: d.writes(), Applies: true}
		if d.Entry.Async != nil {
			m.Priority = d.Entry.Async.WithDefaults().Priority
		}
		if why := s.moduleSubRefusal(d); why != "" {
			m.Applies, m.Reason = false, why
		}
		out.Subscribers = append(out.Subscribers, m)
	}
	return out, nil
}
