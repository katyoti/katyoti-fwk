// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectHooks resolves the hook table for one object.
//
// ORDER, farthest first — the same closer-wins order as every other layered
// declaration: the cross-module subscriptions naming the object's module
// (`module:<subscriber>`, each module object's `on:` entries,
// module_subscriptions.go), the `hook` contributions whose filter admits the object, every
// capability in force (in the effective order), the policies that apply
// (`policy:<name>`), the object's TYPE (its `hooks:`), then the OBJECT's own
// `hooks:`.
//
// A closer entry with the same uri at the same point REPLACES the farther one
// (never a policy's: a rule imposed on the object is not a default to
// override — nor another module's subscription: its reflex, not the
// emitter's to cancel); a closer entry whose `when` is the literal "false"
// REMOVES it. A module object's own `on:` entries are not in its own table:
// they are the subscriptions of the module they name. An entry kept
// but not applying (its filter or its when does not hold) is listed with
// Applies=false and the reason, so a person can see why a hook stayed silent.
//
// SCOPE. At a gate point (one that may refuse the write: before_*,
// on_transition), an entry applies only where its provider owns the object —
// its type, or a capability the object carries by its own choice — or the
// universe grants it more (api.Scope). One out of scope is listed with
// Applies=false and the grant to add, and cannot remove a farther entry.
func (s *Service) ObjectHooks(ctx stdctx.Context, in api.ObjectHooksInput) (api.ObjectHooksOutput, error) {
	if in.ID == "" {
		return api.ObjectHooksOutput{}, fmt.Errorf("%w: ID required", api.ErrInvalidInput)
	}
	if in.Point != "" && !s.isHookPoint(in.Point) {
		return api.ObjectHooksOutput{}, fmt.Errorf("%w: hook point %q is not declared (no hook-type or event type names it)", api.ErrInvalidInput, in.Point)
	}
	ix := s.index()
	if err := ix.ensureLoaded(); err != nil {
		return api.ObjectHooksOutput{}, err
	}
	obj, ok := ix.get(in.ID)
	if !ok {
		return api.ObjectHooksOutput{}, api.ObjectNotFound(in.ID)
	}
	eff, err := s.ObjectEffectiveCapabilities(ctx, api.ObjectEffectiveCapabilitiesInput{ID: in.ID})
	if err != nil {
		return api.ObjectHooksOutput{}, err
	}
	// Listed at no event: an entry whose `when` reads `event` is listed as
	// applying, "evaluated at the event".
	hooks := s.resolveHooks(obj, eff.Capabilities, in.Point, hookAt{listing: true})
	// Each declared input resolved for THIS object — what the action would
	// receive — so a caller running the hook itself sends it as is (what it
	// reads of `event` resolves to nothing here).
	var env map[string]any
	for i := range hooks {
		if len(hooks[i].Input) == 0 {
			continue
		}
		if env == nil {
			env = s.hookEnv(obj, nil)
		}
		hooks[i].ResolvedInput = interpolateObjectInput(hookExprs, hooks[i].Input, env)
	}
	return api.ObjectHooksOutput{Hooks: hooks}, nil
}

// hookLayer is one contributor's declarations, its name in the table and the
// binary that contributed them ("" for the object's own and its type's: a
// type's entries are its owner's by definition).
type hookLayer struct {
	source   string
	hooks    api.Hooks
	provider string
	// refused, on a cross-module subscription's layer, is why it applies
	// nowhere (another module's private type, …): listed, never applying.
	refused string
}

// resolveHooks layers the declarations and evaluates each surviving entry —
// its filter's and its own `when` in the hook vocabulary, at the event `at`
// carries (hook_event.go).
func (s *Service) resolveHooks(obj api.Object, effective []api.CapabilityRef, only api.HookPoint, at hookAt) []api.ResolvedHook {
	caps := s.capabilitiesByName()
	subject := s.hookSubject(obj, effective, at)
	// The farthest layer: the cross-module subscriptions naming the object's
	// module — another module's reflex (or the module's own, declared on its
	// module object), which no layer of the emitter replaces or removes.
	layers := s.moduleSubLayers(s.moduleOfObject(obj.ID))
	// Then the `hook` contributions whose filter admits the object.
	// One that does not match is not listed (it is not about this object); one
	// whose filter cannot be read is kept WITH its filter, so the table says why.
	for _, h := range s.decl().Hooks {
		entry := api.HookEntry{URI: h.URI, When: h.When, Input: h.Input, Async: h.Async}
		if ok, why := h.Filter.Matches(subject); !ok {
			if !strings.HasPrefix(why, "when: ") {
				continue
			}
			f := h.Filter
			entry.Filter = &f
		}
		layers = append(layers, hookLayer{h.Source(), api.Hooks{h.Type: {entry}}, h.Provider, ""})
	}
	for _, ref := range effective {
		if c, ok := caps[ref.ID]; ok && len(c.Hooks) > 0 {
			layers = append(layers, hookLayer{"capability:" + c.Name, c.Hooks, c.Provider, ""})
		}
	}
	// A policy's entries are a layer of their own, after the capabilities':
	// a verify entry is how a policy is measured.
	for _, p := range s.policiesFor(obj) {
		if len(p.Hooks) > 0 {
			layers = append(layers, hookLayer{"policy:" + p.Name, p.Hooks, p.Provider, ""})
		}
	}
	if t, ok := s.lookupObjectType(obj.Type); ok {
		layers = append(layers, hookLayer{"type:" + t.Name, typeHooks(t), t.Provider, ""})
	}
	if len(obj.Hooks) > 0 {
		layers = append(layers, hookLayer{"object", obj.Hooks, "", ""})
	}

	// Whether each layer's provider owns this object, asked once per layer
	// and only at a gate point.
	gates := s.gatePoints()
	// Where an async entry may be declared: a point some event type names.
	eventPoints := s.eventPoints()
	var sc *api.Scope
	inScope := func(l hookLayer) (bool, string) {
		if l.provider == "" || strings.HasPrefix(l.source, "type:") {
			return true, ""
		}
		if sc == nil {
			v := s.scope()
			sc = &v
		}
		return sc.Applies(l.provider, obj.Type, effective)
	}
	var out []api.ResolvedHook
	for _, point := range s.hookPoints() {
		if only != "" && point != only {
			continue
		}
		// Layer the entries of this point: a closer uri replaces (keeps the
		// farther one's position), "false" removes. A policy's entries are its
		// own — a rule imposed on the object, not a default to override — so
		// two policies asking the same action are two entries.
		type slot struct {
			entry    api.HookEntry
			source   string
			provider string
			outOf    string // why the entry is out of its provider's scope; "" in scope
		}
		var order []string
		byKey := map[string]*slot{}
		for _, l := range layers {
			crossModule := strings.HasPrefix(l.source, "module:")
			outOf := l.refused
			if outOf == "" && gates[point] && len(l.hooks[point]) > 0 {
				if ok, why := inScope(l); !ok {
					outOf = why
				}
			}
			for _, e := range l.hooks[point] {
				if e.URI == "" {
					continue
				}
				if e.On != nil && !crossModule {
					continue // a module object's subscription to a module, not a hook of its own
				}
				key := e.URI
				if crossModule || strings.HasPrefix(l.source, "policy:") {
					key = l.source + " " + e.URI
				}
				if e.When == "false" {
					if outOf == "" { // out of scope, it removes nothing
						delete(byKey, key)
					}
					continue
				}
				prev, seen := byKey[key]
				if !seen {
					order = append(order, key)
				} else if outOf != "" && prev.outOf == "" {
					continue // out of scope, it replaces nothing either
				}
				byKey[key] = &slot{e, l.source, l.provider, outOf}
			}
		}
		for _, key := range order {
			sl, ok := byKey[key]
			if !ok {
				continue // removed by a closer "false"
			}
			r := api.ResolvedHook{Point: point, URI: sl.entry.URI, When: sl.entry.When, Source: sl.source, Provider: sl.provider, Input: sl.entry.Input, Applies: true}
			if sl.outOf != "" {
				r.Applies, r.Reason = false, sl.outOf
			}
			if sl.entry.Async != nil {
				// A subscription, listed with its spec (defaults filled) and its
				// id; refused at a point no event type names — it would match
				// no event, so it must not read as one that waits.
				a := sl.entry.Async.WithDefaults()
				// An object's own entry is its own subscription: the id names
				// the object, or two objects declaring the same action at the
				// same point would be one subscription — one's reaction taken
				// for the other's self-reaction, their matches coalesced.
				source := sl.source
				if source == "object" {
					source = "object:" + obj.ID
				}
				r.Async, r.Subscription = &a, api.SubscriptionID(source, point, sl.entry.URI)
				if r.Applies && !eventPoints[point] {
					r.Applies, r.Reason = false, "async at "+string(point)+": no event type names this point"
				}
			}
			if on := sl.entry.On; on != nil {
				// A cross-module subscription: what it listens to, the modules
				// it may write, and — several event types may share a point —
				// the event's type checked at the event.
				r.On, r.Writes = on, sl.entry.Writes
				if t, _ := at.event["type"].(string); r.Applies && at.event != nil && t != on.Event {
					r.Applies, r.Reason = false, "on "+on.Module+" "+on.Event+": the event is "+t
				}
			}
			if r.Applies && sl.entry.Filter != nil {
				if ok, why := sl.entry.Filter.Matches(subject); !ok {
					if filterAtEvent(why) {
						r.Reason = "filter " + why // listed: decided at each event
					} else {
						r.Applies, r.Reason = false, why
					}
				}
			}
			if r.Applies && sl.entry.When != "" {
				ok, err := subject.Eval(sl.entry.When)
				switch {
				case errors.Is(err, errAtEvent):
					r.Reason = "when: " + err.Error() // listed: decided at each event
				case err != nil:
					r.Applies, r.Reason = false, "when: "+err.Error()
				case !ok:
					r.Applies, r.Reason = false, "when is false"
				}
			}
			out = append(out, r)
		}
	}
	return out
}

// typeHooks is a type's declarations at every point: its `hooks:`.
func typeHooks(t api.ObjectType) api.Hooks {
	h := api.Hooks{}
	for p, es := range t.Hooks {
		h[p] = append(h[p], es...)
	}
	return h
}

// filterSubject is what an ObjectFilter is asked about for one object — shared
// by panels and hooks, so both answer "does this apply" the same way. Its
// `when:` calls no function: a hook's filter decides on every write, which must
// never wait on a remote service.
func (s *Service) filterSubject(obj api.Object, effective []api.CapabilityRef) api.FilterSubject {
	var env map[string]any
	return s.filterSubjectIn(obj, effective, objectExprs, func() map[string]any {
		if env == nil {
			env = s.objectActionEnv(obj)
		}
		return env
	})
}

// filterSubjectIn is filterSubject compiled by exprs over envOf — a panel's
// filter may call the capabilities' functions.
func (s *Service) filterSubjectIn(obj api.Object, effective []api.CapabilityRef, exprs *el.Cache, envOf func() map[string]any) api.FilterSubject {
	capIDs := make([]string, 0, len(effective))
	for _, c := range effective {
		capIDs = append(capIDs, c.ID)
	}
	return api.FilterSubject{
		Type:         obj.Type,
		Capabilities: capIDs,
		Eval:         func(expr string) (bool, error) { return exprs.EvalPredicate(expr, envOf()) },
	}.WithEdges(obj.ID, s.subjectEdges(obj))
}

// subjectEdges is the object's incident edges as a filter reads them: what
// points at it, from the index, and what it points at, from the object as it
// is being written — a before_create hook sees the edges the new object
// declares, a before_update the ones it is about to have.
func (s *Service) subjectEdges(obj api.Object) []api.Edge {
	out := obj.GraphEdges() // its own edges, a self-edge included, as written
	ix := s.index()
	if ix.ensureLoaded() != nil {
		return out
	}
	for _, e := range ix.edges(obj.ID, "", api.DirectionIn) {
		if e.From != obj.ID { // its own come from the object, not from the stored copy
			out = append(out, e)
		}
	}
	return out
}

// hookPoints is every declared point: CWM's own (the floor, in lifecycle
// order), then each other point a `hook-type` contribution declares, then each
// point an event type names and no hook-type does — a pack that declares an
// event type with its point needs no hook-type for it (api.EventHookTypes).
func (s *Service) hookPoints() []api.HookPoint {
	d := s.decl()
	out := append([]api.HookPoint(nil), api.HookPoints...)
	seen := map[api.HookPoint]bool{}
	for _, p := range out {
		seen[p] = true
	}
	for _, t := range d.HookTypes {
		if !seen[t.Name] {
			seen[t.Name] = true
			out = append(out, t.Name)
		}
	}
	for _, t := range api.EventHookTypes(d.EventTypes) {
		if !seen[t.Name] {
			seen[t.Name] = true
			out = append(out, t.Name)
		}
	}
	return out
}

// isHookPoint reports a declared point.
func (s *Service) isHookPoint(p api.HookPoint) bool {
	for _, q := range s.hookPoints() {
		if q == p {
			return true
		}
	}
	return false
}
