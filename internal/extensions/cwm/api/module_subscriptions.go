// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// CROSS-MODULE SUBSCRIPTIONS. A module reacts to another module's events by
// declaring the subscription ON ITS OWN MODULE OBJECT: a `hooks:` entry at the
// point the event type names, `async`, with `on: {module, event}` — the
// emitting module and the event type — and the modules its reactions may
// write (`writes:`, default the subscriber's own). Any module may subscribe,
// to PUBLISHED event types only (EventType.Published); a module may subscribe
// to its own private types. CWM's own object.* types are private.
//
// In the emitter's objects' hook tables the subscription is the FARTHEST
// layer, `module:<subscriber>`, keyed like a policy's entry: no layer of the
// emitter replaces or removes it (`when: false` there removes nothing). Its
// id is `module:<subscriber>#<point>#<uri>` — one entry per point and uri on
// a module object. A reaction of it writing a module outside its `writes` is
// refused at the write path (ErrInvalidInput), staged writes included.

// ModuleSubscriptionSource is the hook-table source of a cross-module
// subscription declared by the module object of subscriber.
func ModuleSubscriptionSource(subscriber string) string { return "module:" + subscriber }

// ParseModuleSubscriptionID reads a cross-module subscription id
// (`module:<subscriber>#<point>#<uri>`) back; false when id is not one.
func ParseModuleSubscriptionID(id string) (subscriber string, point HookPoint, uri string, ok bool) {
	rest, found := strings.CutPrefix(id, "module:")
	if !found {
		return "", "", "", false
	}
	parts := strings.SplitN(rest, "#", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], HookPoint(parts[1]), parts[2], true
}

// ModuleSubscribersInput asks who subscribes to a module's events.
type ModuleSubscribersInput struct {
	Module string `json:"module"          jsonschema:"The emitting module; required"`
	Event  string `json:"event,omitempty" jsonschema:"One event type; empty = every type"`
}

// ModuleSubscriber is one module object's cross-module entry naming the
// module: who subscribes, to what, where, with which action, and whether it
// applies (a private type of another module, an undeclared type, a point that
// is not the type's or an entry that is not async do not — Reason says why).
type ModuleSubscriber struct {
	Subscriber   string    `json:"subscriber"          jsonschema:"The subscribing module"`
	Event        string    `json:"event"               jsonschema:"The event type subscribed to"`
	Point        HookPoint `json:"point"`
	URI          string    `json:"uri"                 jsonschema:"The action its reactions run"`
	Subscription string    `json:"subscription"        jsonschema:"module:<subscriber>#<point>#<uri>"`
	Writes       []string  `json:"writes"              jsonschema:"The modules its reactions may write"`
	Priority     string    `json:"priority,omitempty"`
	Applies      bool      `json:"applies"`
	Reason       string    `json:"reason,omitempty"`
}

// ModuleSubscribersOutput is every subscriber of the module, by subscriber
// then point then uri.
type ModuleSubscribersOutput struct {
	Subscribers []ModuleSubscriber `json:"subscribers"`
}
