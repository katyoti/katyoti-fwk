// SPDX-License-Identifier: Apache-2.0

package api

// EVENT TYPES. An `event-type` contribution declares a kind of event a module
// emits — its name, its payload, whether other modules may subscribe to it
// (published) and the hook point an asynchronous subscription to it is
// declared at. CWM ships its own (object.created … object.split); another pack
// declares its own the same way (a code change, a timer), and an `async` hook
// entry is accepted at any point some declared event type names. A point an
// event type names is a point of its own (timing after, answer none): no
// hook-type is needed for it. A pack's event that is not an object write is
// recorded with EventRaise (event_raise.go).

// EventType is an `event-type` contribution.
type EventType struct {
	Name        string    `json:"name"                  yaml:"name"                  jsonschema:"The event's name (dotted kebab: object.created); unique"`
	Label       string    `json:"label,omitempty"       yaml:"label,omitempty"       jsonschema:"The event as a person reads it; empty = the name"`
	Description string    `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"What happened when it is emitted"`
	Payload     string    `json:"payload,omitempty"     yaml:"payload,omitempty"     jsonschema:"The data-type its payload conforms to (a data-type URI)"`
	Published   bool      `json:"published,omitempty"   yaml:"published,omitempty"   jsonschema:"True lets other modules subscribe to it; false keeps it the emitting module's own"`
	Point       HookPoint `json:"point,omitempty"       yaml:"point,omitempty"       jsonschema:"The hook point an async subscription to it is declared at; empty = declared but not subscribable yet"`
	// Provider is the pack that declared it, set when the catalog is read —
	// never authored, never on the wire; shown beside a raised event.
	Provider string `json:"-" yaml:"-"`
}

// BuiltinEventTypes are CWM's own event types — the floor of the declared set,
// and what the shipped `event-types/*.yaml` files state (a test holds the two
// together). The writes' events are subscribable at the on_* points, a
// transition and a refused move at their after-points (on_transitioned,
// on_transition_refused — points only these event types declare); a move, a
// split, an archive and a catch-up are declared and not subscribable yet.
var BuiltinEventTypes = []EventType{
	{Name: "object.created", Payload: objectEventPayloadType, Point: HookOnCreate},
	{Name: "object.updated", Payload: objectEventPayloadType, Point: HookOnUpdate},
	{Name: "object.deleted", Payload: objectEventPayloadType, Point: HookOnDelete},
	{Name: "object.transitioned", Payload: "system:/data-type/object-transitioned-event", Point: HookOnTransitioned},
	{Name: "object.transition-refused", Payload: "system:/data-type/object-transition-refused-event", Point: HookOnTransitionRefused},
	{Name: "object.moved"},
	{Name: "object.split"},
	{Name: "object.archived"},
	{Name: "project.caught-up", Payload: "system:/data-type/objects-caught-up-event"},
}

// The after-points of a transition. They are not CWM's hook-types (HookPoints):
// the event types above declare them (EventHookTypes), so they fire after the
// event and answer nothing. on_transition stays the synchronous gate before
// the move.
const (
	// HookOnTransitioned fires after a move was written, on the moved object;
	// its event's payload is the transitioned event's (from, to, by, …).
	HookOnTransitioned HookPoint = "on_transitioned"
	// HookOnTransitionRefused fires after a move its guard refused (never a
	// dry run, never a forced move), on the object, which is unchanged; its
	// event's payload says the guard, the reason and what each function
	// called answered.
	HookOnTransitionRefused HookPoint = "on_transition_refused"
)

// The actions a move's events carry (MatchedEvent.Action).
const (
	TransitionedEventAction      = "transitioned"
	TransitionRefusedEventAction = "transition-refused"
)

// ArchivedEventAction is the action of `object.<token>.archived`: an object
// ended (ObjectArchive), its file moved to its module's history folder.
const ArchivedEventAction = "archived"

// objectEventPayloadType is the payload of a write's event (ObjectEventPayload).
const objectEventPayloadType = "system:/data-type/object-event"

// IsBuiltinEventType reports one of CWM's own event types: raised by its
// writes, never by EventRaise.
func IsBuiltinEventType(name string) bool {
	for _, t := range BuiltinEventTypes {
		if t.Name == name {
			return true
		}
	}
	return false
}

// EventPoints is every point some event type names: where an async hook entry
// may be declared.
func EventPoints(types []EventType) map[HookPoint]bool {
	out := map[HookPoint]bool{}
	for _, t := range BuiltinEventTypes {
		if t.Point != "" {
			out[t.Point] = true
		}
	}
	for _, t := range types {
		if t.Point != "" {
			out[t.Point] = true
		}
	}
	return out
}

// EventHookTypes is the point each event type names, as a hook-type: it fires
// after the event (on the object the event concerns) and answers nothing. A
// pack declares the event type with its point once — no hook-type is needed
// for it; a hook-type of the same name that says another timing is a
// declaration finding (EventPointTimingsOf). CWM's own types first, then the
// declared ones, each point once.
func EventHookTypes(types []EventType) []HookType {
	var out []HookType
	seen := map[HookPoint]bool{}
	add := func(t EventType) {
		if t.Point == "" || seen[t.Point] {
			return
		}
		seen[t.Point] = true
		out = append(out, HookType{Name: t.Point, Timing: HookTimingAfter, Answer: HookAnswerNone,
			Description: "Fires on the event " + t.Name})
	}
	for _, t := range BuiltinEventTypes {
		add(t)
	}
	for _, t := range types {
		add(t)
	}
	return out
}
