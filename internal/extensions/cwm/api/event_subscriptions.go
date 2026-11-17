// SPDX-License-Identifier: Apache-2.0

package api

// THE SUBSCRIPTIONS TO AN EVENT TYPE. An event type is subscribed to at the
// point it names, by an async entry in any layer of a hook table: a module
// object's cross-module entry (`on:`), a `hook` contribution, a capability, a
// policy, an object type, an object's own `hooks:`. EventSubscriptions lists
// them all for one event type (or every declared one) WITHOUT an object: each
// declaration once, farthest layer first — module, hook, capability, policy,
// type, object — with who declared it and whether it applies.
//
// What is decided statically is decided here: a cross-module entry another
// module may not hold (a private type, a point that is not its type's), a
// policy that applies to no object. What is decided per object or per event
// is listed as applying, its Reason saying so: a `when` reading `event`
// ("evaluated at the event"), another `when` ("decided per object: when …"),
// an entry at a point several event types share ("fires for every event at
// …"). An object's own entry is resolved on its object. An event type that
// names no point is listed with no subscription ("not subscribable").

// EventSubscriptionsInput asks who subscribes to an event type.
type EventSubscriptionsInput struct {
	Event string `json:"event,omitempty" jsonschema:"One event type (object.created or code.changed); empty = every declared type"`
}

// EventTypeSubscriptions is one event type and every subscription to it.
type EventTypeSubscriptions struct {
	Event         string              `json:"event"              jsonschema:"The event type"`
	Point         HookPoint           `json:"point,omitempty"    jsonschema:"The point its subscriptions are declared at; empty = not subscribable"`
	Published     bool                `json:"published"          jsonschema:"True when other modules may subscribe to it"`
	Provider      string              `json:"provider,omitempty" jsonschema:"The pack that declared it; empty for CWM's own"`
	Payload       string              `json:"payload,omitempty"  jsonschema:"The data-type its payload conforms to"`
	Reason        string              `json:"reason,omitempty"   jsonschema:"Why it has no subscription (not subscribable)"`
	Subscriptions []EventSubscription `json:"subscriptions"      jsonschema:"Every async entry at its point: farthest layer first (module then hook then capability then policy then type then object)"`
}

// EventSubscription is one declared subscription to an event type.
type EventSubscription struct {
	Subscription string        `json:"subscription"        jsonschema:"Its id: <source>#<point>#<uri>"`
	Source       string        `json:"source"              jsonschema:"The layer that declares it: module:<subscriber> | hook:<provider>/<name> | capability:<name> | policy:<name> | type:<name> | object:<id>"`
	Provider     string        `json:"provider,omitempty"  jsonschema:"The binary that contributed it; empty for an object's own and a module's"`
	DeclaredOn   string        `json:"declared_on"         jsonschema:"What declares it: the subscriber module or the hook or capability or policy or type name or the object id"`
	URI          string        `json:"uri"                 jsonschema:"The action its reactions run"`
	When         string        `json:"when,omitempty"      jsonschema:"Its el predicate"`
	Filter       *ObjectFilter `json:"filter,omitempty"    jsonschema:"The objects it applies to"`
	Async        AsyncSpec     `json:"async"               jsonschema:"Its subscription spec with the defaults filled"`
	On           *EventSource  `json:"on,omitempty"        jsonschema:"A cross-module subscription's module and event type"`
	Writes       []string      `json:"writes,omitempty"    jsonschema:"A cross-module subscription's writable modules"`
	Applies      bool          `json:"applies"             jsonschema:"False when it can never match this event type"`
	Reason       string        `json:"reason,omitempty"    jsonschema:"Why it does not apply or what decides it per object or per event"`
}

// EventSubscriptionsOutput is every event type asked, CWM's own first then
// the declared ones in catalog order.
type EventSubscriptionsOutput struct {
	Events []EventTypeSubscriptions `json:"events"`
}
